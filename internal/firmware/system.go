package firmware

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const GSIWarning = "A different system image (for example a GSI) replaces the andr36oid system. It must be an ARM64 GSI for Android 11 or newer that fits the 2.2 GiB system partition; most newer GSIs are larger. andr36oid features, updates and support do not apply to it."

const sparseMagic = 0xed26ff3a

// PrepareSystem turns a raw or Android sparse system image into a file exactly the size of
// the image's system partition, padded with zeros, ready to be written in its place.
func PrepareSystem(ctx context.Context, im *Image, src, dir string) (string, error) {
	part, ok := im.Table.Find("system")
	if !ok {
		return "", errors.New("this release has no system partition")
	}
	in, e := os.Open(src)
	if e != nil {
		return "", e
	}
	defer in.Close()
	st, e := in.Stat()
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("the system image is not a regular file")
	}
	head := make([]byte, 2048)
	n, _ := io.ReadFull(in, head)
	head = head[:n]
	if kind := compressed(head); kind != "" {
		return "", fmt.Errorf("the system image is %s compressed; extract the .img file first", kind)
	}
	out, e := os.CreateTemp(dir, "system-*.img")
	if e != nil {
		return "", e
	}
	ok = false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(out.Name())
		}
	}()
	if len(head) >= 4 && binary.LittleEndian.Uint32(head) == sparseMagic {
		e = unsparse(ctx, in, out, part.Size())
	} else {
		if st.Size() > part.Size() {
			return "", tooBig(st.Size(), part.Size())
		}
		_, e = io.Copy(out, io.NewSectionReader(in, 0, st.Size()))
	}
	if e != nil {
		return "", e
	}
	if e = out.Truncate(part.Size()); e != nil {
		return "", e
	}
	fs := make([]byte, 2048)
	if _, e = out.ReadAt(fs, 0); e != nil {
		return "", e
	}
	if !systemFS(fs) {
		return "", errors.New("the system image has no ext4 or EROFS filesystem")
	}
	if e = out.Sync(); e != nil {
		return "", e
	}
	ok = true
	return out.Name(), nil
}

func tooBig(size, room int64) error {
	return fmt.Errorf("the system image is too large: %.2f GiB, the system partition holds %.2f GiB", float64(size)/(1<<30), float64(room)/(1<<30))
}

func compressed(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xfd, '7', 'z', 'X', 'Z', 0}):
		return "xz"
	case bytes.HasPrefix(b, []byte{0x1f, 0x8b}):
		return "gzip"
	case bytes.HasPrefix(b, []byte("PK\x03\x04")):
		return "zip"
	case bytes.HasPrefix(b, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return "zstd"
	case bytes.HasPrefix(b, []byte("7z\xbc\xaf\x27\x1c")):
		return "7z"
	}
	return ""
}

func systemFS(b []byte) bool {
	return (b[1080] == 0x53 && b[1081] == 0xef) || bytes.Equal(b[1024:1028], []byte{0xe2, 0xe1, 0xf5, 0xe0})
}

// unsparse expands an Android sparse image into out. Skipped blocks stay zero.
func unsparse(ctx context.Context, in io.ReadSeeker, out io.WriterAt, room int64) error {
	if _, e := in.Seek(0, io.SeekStart); e != nil {
		return e
	}
	var h struct {
		Magic                       uint32
		Major, Minor                uint16
		HeaderSize, ChunkHeaderSize uint16
		BlockSize                   uint32
		Blocks, Chunks              uint32
		Checksum                    uint32
	}
	if e := binary.Read(in, binary.LittleEndian, &h); e != nil {
		return fmt.Errorf("damaged sparse image: %w", e)
	}
	if h.Major != 1 || h.HeaderSize < 28 || h.ChunkHeaderSize < 12 || h.BlockSize == 0 || h.BlockSize%4 != 0 {
		return errors.New("unsupported sparse image")
	}
	if total := int64(h.Blocks) * int64(h.BlockSize); total > room {
		return tooBig(total, room)
	}
	if _, e := in.Seek(int64(h.HeaderSize), io.SeekStart); e != nil {
		return e
	}
	bs := int64(h.BlockSize)
	var block int64
	buf := make([]byte, 4<<20)
	for i := uint32(0); i < h.Chunks; i++ {
		if e := ctx.Err(); e != nil {
			return e
		}
		var c struct {
			Type     uint16
			Reserved uint16
			Blocks   uint32
			Size     uint32
		}
		if e := binary.Read(in, binary.LittleEndian, &c); e != nil {
			return fmt.Errorf("damaged sparse image: %w", e)
		}
		if _, e := in.Seek(int64(h.ChunkHeaderSize)-12, io.SeekCurrent); e != nil {
			return e
		}
		body := int64(c.Size) - int64(h.ChunkHeaderSize)
		length := int64(c.Blocks) * bs
		if block+int64(c.Blocks) > int64(h.Blocks) {
			return errors.New("damaged sparse image: chunk past the end")
		}
		off := block * bs
		switch c.Type {
		case 0xcac1: // raw
			if body != length {
				return errors.New("damaged sparse image: bad raw chunk")
			}
			for done := int64(0); done < length; {
				n := int64(len(buf))
				if length-done < n {
					n = length - done
				}
				if _, e := io.ReadFull(in, buf[:n]); e != nil {
					return fmt.Errorf("damaged sparse image: %w", e)
				}
				if _, e := out.WriteAt(buf[:n], off+done); e != nil {
					return e
				}
				done += n
			}
		case 0xcac2: // fill
			if body != 4 {
				return errors.New("damaged sparse image: bad fill chunk")
			}
			var v [4]byte
			if _, e := io.ReadFull(in, v[:]); e != nil {
				return e
			}
			if v != [4]byte{} {
				pat := bytes.Repeat(v[:], len(buf)/4)
				for done := int64(0); done < length; {
					n := int64(len(pat))
					if length-done < n {
						n = length - done
					}
					if _, e := out.WriteAt(pat[:n], off+done); e != nil {
						return e
					}
					done += n
				}
			}
		case 0xcac3: // don't care
			if body != 0 {
				return errors.New("damaged sparse image: bad skip chunk")
			}
		case 0xcac4: // checksum
			if _, e := in.Seek(body, io.SeekCurrent); e != nil {
				return e
			}
		default:
			return fmt.Errorf("damaged sparse image: unknown chunk type %#x", c.Type)
		}
		block += int64(c.Blocks)
	}
	return nil
}
