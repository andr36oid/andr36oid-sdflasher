package layout

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"unicode/utf16"
)

const Sector int64 = 512
const Entries = 128
const EntrySize = 128

type Partition struct {
	Number int      `json:"number"`
	Name   string   `json:"name"`
	Start  uint64   `json:"start"`
	End    uint64   `json:"end"`
	Type   [16]byte `json:"type"`
	ID     [16]byte `json:"id"`
	Flags  uint64   `json:"flags"`
}

func (p Partition) Offset() int64 { return int64(p.Start) * Sector }
func (p Partition) Size() int64   { return int64(p.End-p.Start+1) * Sector }

type Table struct {
	GPT   bool        `json:"gpt"`
	ID    [16]byte    `json:"id"`
	Parts []Partition `json:"partitions"`
	Size  int64       `json:"size"`
}

func (t Table) Find(name string) (Partition, bool) {
	for _, p := range t.Parts {
		if p.Name == name {
			return p, true
		}
	}
	return Partition{}, false
}
func (t Table) Number(n int) (Partition, bool) {
	for _, p := range t.Parts {
		if p.Number == n {
			return p, true
		}
	}
	return Partition{}, false
}
func GUID() [16]byte {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	b[7] = (b[7] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return b
}

var LinuxType = [16]byte{0xaf, 0x3d, 0xc6, 0x0f, 0x83, 0x84, 0x72, 0x47, 0x8e, 0x79, 0x3d, 0x69, 0xd8, 0x47, 0x7d, 0xe4}

func Read(r io.ReaderAt, size int64) (Table, error) {
	t := Table{Size: size}
	if size < 68*Sector || size%Sector != 0 {
		return t, errors.New("unsupported disk size or sector size")
	}
	b := make([]byte, 1024)
	if _, e := r.ReadAt(b, 0); e != nil {
		return t, e
	}
	if b[510] != 0x55 || b[511] != 0xaa {
		return t, errors.New("no valid partition table")
	}
	if string(b[512:520]) != "EFI PART" {
		for i := 0; i < 4; i++ {
			e := b[446+i*16 : 462+i*16]
			kind := e[4]
			if kind == 0 {
				continue
			}
			if kind == 0xee {
				return t, errors.New("damaged GPT header")
			}
			if kind == 5 || kind == 15 || kind == 0x85 {
				return t, errors.New("extended MBR partitions are unsupported")
			}
			start := uint64(binary.LittleEndian.Uint32(e[8:]))
			count := uint64(binary.LittleEndian.Uint32(e[12:]))
			if count == 0 {
				return t, errors.New("empty MBR partition")
			}
			t.Parts = append(t.Parts, Partition{Number: i + 1, Start: start, End: start + count - 1, Type: LinuxType, ID: GUID()})
		}
		return t, t.Validate(false)
	}
	t.GPT = true
	h := b[512:]
	n := binary.LittleEndian.Uint32(h[12:])
	if n < 92 || n > 512 {
		return t, errors.New("invalid GPT header length")
	}
	sum := binary.LittleEndian.Uint32(h[16:])
	binary.LittleEndian.PutUint32(h[16:], 0)
	if crc32.ChecksumIEEE(h[:n]) != sum {
		return t, errors.New("GPT header checksum mismatch")
	}
	if binary.LittleEndian.Uint64(h[24:]) != 1 {
		return t, errors.New("invalid primary GPT location")
	}

	backupLBA := binary.LittleEndian.Uint64(h[32:])
	if backupLBA < 34 || backupLBA >= uint64(size/Sector) {
		return t, errors.New("backup GPT outside disk")
	}
	backup := make([]byte, 512)
	if _, e := r.ReadAt(backup, int64(backupLBA)*Sector); e != nil {
		return t, e
	}
	if string(backup[:8]) != "EFI PART" || binary.LittleEndian.Uint32(backup[12:]) != n {
		return t, errors.New("invalid backup GPT")
	}
	backupSum := binary.LittleEndian.Uint32(backup[16:])
	binary.LittleEndian.PutUint32(backup[16:], 0)
	if crc32.ChecksumIEEE(backup[:n]) != backupSum || binary.LittleEndian.Uint64(backup[24:]) != backupLBA || binary.LittleEndian.Uint64(backup[32:]) != 1 || !bytes.Equal(backup[40:72], h[40:72]) {
		return t, errors.New("primary and backup GPT disagree")
	}
	backupEntries := binary.LittleEndian.Uint64(backup[72:])
	if backupEntries != backupLBA-32 || !bytes.Equal(backup[80:92], h[80:92]) {
		return t, errors.New("invalid backup partition entries")
	}
	backupRaw := make([]byte, Entries*EntrySize)
	if _, e := r.ReadAt(backupRaw, int64(backupEntries)*Sector); e != nil {
		return t, e
	}
	if crc32.ChecksumIEEE(backupRaw) != binary.LittleEndian.Uint32(h[88:]) {
		return t, errors.New("backup GPT checksum mismatch")
	}
	copy(t.ID[:], h[56:72])
	lba := binary.LittleEndian.Uint64(h[72:])
	count := binary.LittleEndian.Uint32(h[80:])
	width := binary.LittleEndian.Uint32(h[84:])
	if lba != 2 || count != Entries || width != EntrySize {
		return t, errors.New("unsupported GPT entry layout")
	}
	raw := make([]byte, Entries*EntrySize)
	if _, e := r.ReadAt(raw, int64(lba)*Sector); e != nil {
		return t, e
	}
	if crc32.ChecksumIEEE(raw) != binary.LittleEndian.Uint32(h[88:]) {
		return t, errors.New("GPT partition checksum mismatch")
	}
	for i := 0; i < Entries; i++ {
		e := raw[i*EntrySize : (i+1)*EntrySize]
		if bytes.Equal(e[:16], make([]byte, 16)) {
			continue
		}
		p := Partition{Number: i + 1, Start: binary.LittleEndian.Uint64(e[32:]), End: binary.LittleEndian.Uint64(e[40:]), Flags: binary.LittleEndian.Uint64(e[48:])}
		copy(p.Type[:], e[:16])
		copy(p.ID[:], e[16:32])
		var s []uint16
		for j := 56; j < 128; j += 2 {
			v := binary.LittleEndian.Uint16(e[j:])
			if v == 0 {
				break
			}
			s = append(s, v)
		}
		p.Name = string(utf16.Decode(s))
		t.Parts = append(t.Parts, p)
	}
	return t, t.Validate(false)
}
func (t Table) Validate(forWrite bool) error {
	if len(t.Parts) == 0 {
		return errors.New("empty partition table")
	}
	last := uint64(t.Size/Sector) - 1
	seen := map[int]bool{}
	names := map[string]bool{}
	for i, p := range t.Parts {
		if p.Number < 1 || p.Number > Entries || seen[p.Number] {
			return errors.New("invalid or duplicate partition number")
		}
		seen[p.Number] = true
		if p.Name != "" && names[p.Name] {
			return errors.New("duplicate partition name")
		}
		names[p.Name] = true
		min := uint64(1)
		max := last
		if t.GPT || forWrite {
			min = 34
			max = last - 33
		}
		if p.Start < min || p.End < p.Start || p.End > max {
			return fmt.Errorf("partition %d does not leave room for the partition table", p.Number)
		}
		for _, q := range t.Parts[:i] {
			if p.Start <= q.End && q.Start <= p.End {
				return fmt.Errorf("partitions %d and %d overlap", p.Number, q.Number)
			}
		}
	}
	return nil
}

// Encode places the backup GPT at the physical end of the destination disk.
func (t Table) Encode() (head, tail []byte, err error) {
	if err = t.Validate(true); err != nil {
		return
	}
	raw := make([]byte, Entries*EntrySize)
	for _, p := range t.Parts {
		e := raw[(p.Number-1)*EntrySize : p.Number*EntrySize]
		copy(e, p.Type[:])
		copy(e[16:], p.ID[:])
		binary.LittleEndian.PutUint64(e[32:], p.Start)
		binary.LittleEndian.PutUint64(e[40:], p.End)
		binary.LittleEndian.PutUint64(e[48:], p.Flags)
		s := utf16.Encode([]rune(p.Name))
		if len(s) > 36 {
			err = errors.New("partition name too long")
			return
		}
		for j, v := range s {
			binary.LittleEndian.PutUint16(e[56+j*2:], v)
		}
	}
	last := uint64(t.Size/Sector) - 1
	id := t.ID
	if id == [16]byte{} {
		id = GUID()
	}
	header := func(current, backup, entries uint64) []byte {
		h := make([]byte, 512)
		copy(h, "EFI PART")
		binary.LittleEndian.PutUint32(h[8:], 0x10000)
		binary.LittleEndian.PutUint32(h[12:], 92)
		binary.LittleEndian.PutUint64(h[24:], current)
		binary.LittleEndian.PutUint64(h[32:], backup)
		binary.LittleEndian.PutUint64(h[40:], 34)
		binary.LittleEndian.PutUint64(h[48:], last-33)
		copy(h[56:], id[:])
		binary.LittleEndian.PutUint64(h[72:], entries)
		binary.LittleEndian.PutUint32(h[80:], Entries)
		binary.LittleEndian.PutUint32(h[84:], EntrySize)
		binary.LittleEndian.PutUint32(h[88:], crc32.ChecksumIEEE(raw))
		binary.LittleEndian.PutUint32(h[16:], crc32.ChecksumIEEE(h[:92]))
		return h
	}
	head = make([]byte, 34*512)
	head[510] = 0x55
	head[511] = 0xaa
	head[450] = 0xee
	head[447] = 0
	head[448] = 2
	head[449] = 0
	head[451] = 255
	head[452] = 255
	head[453] = 255
	binary.LittleEndian.PutUint32(head[454:], 1)
	sz := last
	if sz > 0xffffffff {
		sz = 0xffffffff
	}
	binary.LittleEndian.PutUint32(head[458:], uint32(sz))
	copy(head[512:], header(1, last, 2))
	copy(head[1024:], raw)
	tail = append(append([]byte{}, raw...), header(last, 1, last-32)...)
	return
}
