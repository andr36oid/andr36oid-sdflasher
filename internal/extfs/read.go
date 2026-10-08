// Package extfs provides bounded, read-only access to release-image ext4 filesystems.
package extfs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"path"
	"strings"
)

var le = binary.LittleEndian

type FS struct {
	r                                   io.ReaderAt
	offset, size                        int64
	block                               uint64
	inodesPerGroup, inodeSize, descSize uint64
	blocks                              uint64
	is64                                bool
}

func Open(r io.ReaderAt, offset, size int64) (*FS, error) {
	s := make([]byte, 1024)
	if _, e := r.ReadAt(s, offset+1024); e != nil {
		return nil, e
	}
	if le.Uint16(s[56:]) != 0xef53 {
		return nil, errors.New("ext4 signature missing")
	}
	shift := le.Uint32(s[24:])
	if shift > 6 {
		return nil, errors.New("invalid ext4 block size")
	}
	incompat := le.Uint32(s[96:])
	if incompat&0x10000 != 0 || incompat&0x8000 != 0 {
		return nil, errors.New("encrypted or inline ext4 is unsupported")
	}
	if incompat&4 != 0 {
		return nil, errors.New("filesystem journal needs recovery")
	}
	blocks := uint64(le.Uint32(s[4:]))
	is64 := incompat&0x80 != 0
	if is64 {
		blocks |= uint64(le.Uint32(s[336:])) << 32
	}
	block := uint64(1024) << shift
	if blocks == 0 || blocks > uint64(size)/block {
		return nil, errors.New("filesystem exceeds partition")
	}
	ino := uint64(le.Uint16(s[88:]))
	ipg := uint64(le.Uint32(s[40:]))
	if ino < 128 || ino > block || ipg == 0 {
		return nil, errors.New("invalid inode geometry")
	}
	ds := uint64(32)
	if is64 {
		ds = uint64(le.Uint16(s[254:]))
		if ds < 64 || ds > block {
			return nil, errors.New("invalid descriptor size")
		}
	}
	if le.Uint32(s[100:])&0x400 != 0 {
		if s[373] != 1 {
			return nil, errors.New("unsupported ext4 checksum")
		}
		if ^crc32.Checksum(s[:1020], crc32.MakeTable(crc32.Castagnoli)) != le.Uint32(s[1020:]) {
			return nil, errors.New("ext4 superblock checksum mismatch")
		}
	}
	return &FS{r, offset, size, block, ipg, ino, ds, blocks, is64}, nil
}
func (f *FS) read(off, n uint64) ([]byte, error) {
	if off > uint64(f.size) || n > uint64(f.size)-off || n > 64<<20 {
		return nil, errors.New("filesystem read outside partition")
	}
	b := make([]byte, n)
	_, e := f.r.ReadAt(b, f.offset+int64(off))
	return b, e
}
func (f *FS) inode(n uint32) ([]byte, error) {
	if n == 0 {
		return nil, errors.New("invalid inode")
	}
	g := uint64(n-1) / f.inodesPerGroup
	idx := uint64(n-1) % f.inodesPerGroup
	base := f.block
	if f.block == 1024 {
		base = 2048
	}
	d, e := f.read(base+g*f.descSize, f.descSize)
	if e != nil {
		return nil, e
	}
	table := uint64(le.Uint32(d[8:]))
	if f.is64 {
		table |= uint64(le.Uint32(d[40:])) << 32
	}
	if table >= f.blocks {
		return nil, errors.New("inode table outside filesystem")
	}
	return f.read(table*f.block+idx*f.inodeSize, f.inodeSize)
}

type extent struct {
	logical, physical, count uint64
	zero                     bool
}

func (f *FS) extents(b []byte, depth int, seen map[uint64]bool) ([]extent, error) {
	if depth > 5 || len(b) < 12 || le.Uint16(b) != 0xf30a {
		return nil, errors.New("invalid extent tree")
	}
	n := int(le.Uint16(b[2:]))
	level := le.Uint16(b[6:])
	if level > 5 || 12+n*12 > len(b) {
		return nil, errors.New("invalid extent count")
	}
	var out []extent
	for i := 0; i < n; i++ {
		e := b[12+i*12:]
		if level == 0 {
			length := uint64(le.Uint16(e[4:]))
			zero := length > 32768
			if zero {
				length -= 32768
			}
			p := uint64(le.Uint16(e[6:]))<<32 | uint64(le.Uint32(e[8:]))
			if length == 0 || p+length > f.blocks {
				return nil, errors.New("extent outside filesystem")
			}
			out = append(out, extent{uint64(le.Uint32(e)), p, length, zero})
		} else {
			p := uint64(le.Uint32(e[4:])) | uint64(le.Uint16(e[8:]))<<32
			if p >= f.blocks || seen[p] {
				return nil, errors.New("invalid extent index")
			}
			seen[p] = true
			child, e := f.read(p*f.block, f.block)
			if e != nil {
				return nil, e
			}
			if le.Uint16(child[6:])+1 != level {
				return nil, errors.New("extent depth mismatch")
			}
			sub, e := f.extents(child, depth+1, seen)
			if e != nil {
				return nil, e
			}
			out = append(out, sub...)
		}
	}
	return out, nil
}
func (f *FS) data(in []byte, limit uint64) ([]byte, error) {
	size := uint64(le.Uint32(in[4:]))
	mode := le.Uint16(in) & 0xf000
	if mode == 0x8000 {
		size |= uint64(le.Uint32(in[108:])) << 32
	}
	if size > limit {
		return nil, errors.New("file exceeds inspection limit")
	}
	if mode == 0xa000 && size <= 60 {
		return append([]byte{}, in[40:40+size]...), nil
	}
	b := make([]byte, size)
	if size == 0 {
		return b, nil
	}
	var ex []extent
	if le.Uint32(in[32:])&0x80000 != 0 {
		var e error
		ex, e = f.extents(in[40:100], 0, map[uint64]bool{})
		if e != nil {
			return nil, e
		}
	} else {
		if size > 12*f.block {
			return nil, errors.New("legacy indirect ext4 file is unsupported")
		}
		for i := uint64(0); i < 12; i++ {
			p := uint64(le.Uint32(in[40+i*4:]))
			if p != 0 {
				ex = append(ex, extent{i, p, 1, false})
			}
		}
	}
	var last uint64
	for i, e := range ex {
		start := e.logical * f.block
		n := e.count * f.block
		if i > 0 && start < last {
			return nil, errors.New("overlapping file extents")
		}
		last = start + n
		if start >= size {
			continue
		}
		if n > size-start {
			n = size - start
		}
		if !e.zero {
			d, err := f.read(e.physical*f.block, n)
			if err != nil {
				return nil, err
			}
			copy(b[start:], d)
		}
	}
	return b, nil
}
func (f *FS) ReadFile(name string, limit int64) ([]byte, error) {
	return f.resolve(path.Clean("/"+name), limit, 0)
}
func (f *FS) resolve(name string, limit int64, links int) ([]byte, error) {
	if links > 8 {
		return nil, errors.New("too many symlinks")
	}
	node := uint32(2)
	parts := strings.Split(strings.Trim(name, "/"), "/")
	for i, part := range parts {
		in, e := f.inode(node)
		if e != nil {
			return nil, e
		}
		if le.Uint16(in)&0xf000 != 0x4000 {
			return nil, errors.New("path is not a directory")
		}
		b, e := f.data(in, 16<<20)
		if e != nil {
			return nil, e
		}
		found := uint32(0)
		for pos := 0; pos+8 <= len(b); {
			rec := int(le.Uint16(b[pos+4:]))
			n := int(b[pos+6])
			if rec < 8 || pos+rec > len(b) || n > rec-8 {
				return nil, errors.New("damaged directory entry")
			}
			if string(b[pos+8:pos+8+n]) == part {
				found = le.Uint32(b[pos:])
				break
			}
			pos += rec
		}
		if found == 0 {
			return nil, fmt.Errorf("file %s not found", name)
		}
		node = found
		in, e = f.inode(node)
		if e != nil {
			return nil, e
		}
		if le.Uint16(in)&0xf000 == 0xa000 {
			target, e := f.data(in, 4096)
			if e != nil {
				return nil, e
			}
			next := string(target)
			if !strings.HasPrefix(next, "/") {
				next = path.Join("/", strings.Join(parts[:i], "/"), next)
			}
			next = path.Join(next, strings.Join(parts[i+1:], "/"))
			return f.resolve(next, limit, links+1)
		}
	}
	in, e := f.inode(node)
	if e != nil {
		return nil, e
	}
	return f.data(in, uint64(limit))
}

func (f *FS) RootNames() ([]string, error) {
	in, e := f.inode(2)
	if e != nil {
		return nil, e
	}
	b, e := f.data(in, 16<<20)
	if e != nil {
		return nil, e
	}
	var out []string
	for pos := 0; pos+8 <= len(b); {
		rec := int(le.Uint16(b[pos+4:]))
		n := int(b[pos+6])
		if rec < 8 || pos+rec > len(b) || n > rec-8 {
			return nil, errors.New("damaged root directory")
		}
		if le.Uint32(b[pos:]) != 0 {
			out = append(out, string(b[pos+8:pos+8+n]))
		}
		pos += rec
	}
	return out, nil
}
