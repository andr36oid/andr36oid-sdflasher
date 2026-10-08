package firmware

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/diskfs/go-diskfs/backend"
)

type sectorStorage struct {
	backend.Storage
	data  []byte
	calls int
	err   error
}

func (s *sectorStorage) ReadAt(p []byte, off int64) (int, error) {
	s.calls++
	if off%512 != 0 || len(p)%512 != 0 {
		return 0, errors.New("unaligned raw device read")
	}
	if s.err != nil {
		return 0, s.err
	}
	return bytes.NewReader(s.data).ReadAt(p, off)
}

func TestBootReadsUseCompleteSectorsWithinPartition(t *testing.T) {
	raw := make([]byte, 4096)
	for i := range raw {
		raw[i] = byte(i % 251)
	}
	dev := &sectorStorage{data: raw}
	s := bootStorage{Storage: dev, start: 512, size: 2048}
	for _, r := range [][2]int{{512, 512}, {513, 7}, {1001, 799}, {2550, 10}} {
		b := make([]byte, r[1])
		n, e := s.ReadAt(b, int64(r[0]))
		if e != nil || n != len(b) || !bytes.Equal(b, raw[r[0]:r[0]+r[1]]) {
			t.Fatalf("read %v: n=%d, error=%v", r, n, e)
		}
	}
	before := dev.calls
	for _, off := range []int64{-1, 0, 2559, 4096} {
		if _, e := s.ReadAt(make([]byte, 2), off); !errors.Is(e, io.EOF) {
			t.Fatalf("accepted out-of-partition read at %d", off)
		}
	}
	if dev.calls != before {
		t.Fatal("out-of-partition read reached the device")
	}
	dev.err = errors.New("media error")
	if _, e := s.ReadAt(make([]byte, 1), 513); !errors.Is(e, dev.err) {
		t.Fatal("media error was lost")
	}
}
