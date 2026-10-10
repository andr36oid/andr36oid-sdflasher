package flash

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type timedDevice struct {
	*os.File
	read, write time.Duration
	writes      []Range
}

func (d *timedDevice) ReadAt(b []byte, off int64) (int, error) {
	time.Sleep(d.read)
	return d.File.ReadAt(b, off)
}
func (d *timedDevice) WriteAt(b []byte, off int64) (int, error) {
	time.Sleep(d.write)
	d.writes = append(d.writes, Range{"", off, int64(len(b))})
	return d.File.WriteAt(b, off)
}

func TestCardSpeed(t *testing.T) {
	const size = int64(1 << 30)
	p := &Plan{Size: size, Writes: []Write{
		{Range: Range{"Release image", 1 << 20, 300 << 20}},
		{Range: Range{"Device setup", 400 << 20, 128 << 20}, Boot: true},
		{Range: Range{"Partition table", 0, 34 * 512}, Data: make([]byte, 34*512)},
	}}
	if start, length := unreadRange(p); start != 528<<20 || length != size-34*512-(528<<20) {
		t.Fatalf("unread range %d+%d", start, length)
	}
	for _, c := range []struct {
		name        string
		read, write time.Duration
		mbps        float64
		slow        bool
	}{
		{"fast", 0, 0, 40, false},
		{"slow random reads", 8 * time.Millisecond, 0, 40, true},
		{"slow random writes", 0, 40 * time.Millisecond, 40, true},
		{"slow sequential", 0, 0, 5, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, _ := os.Create(filepath.Join(t.TempDir(), "card"))
			defer f.Close()
			f.Truncate(size)
			d := &timedDevice{File: f, read: c.read, write: c.write}
			s, e := measureLatency(context.Background(), d, p)
			if e != nil {
				t.Fatal(e)
			}
			for _, w := range d.writes {
				if w.Offset < 1<<20 || w.End() > 301<<20 {
					t.Fatalf("speed test wrote outside the release image range: %+v", w)
				}
			}
			if len(d.writes) != speedWrites {
				t.Fatalf("%d test writes", len(d.writes))
			}
			s.WriteMBps = c.mbps
			s.judge()
			if s.Slow != c.slow {
				t.Fatalf("%+v: slow %v, want %v", s, s.Slow, c.slow)
			}
		})
	}
}
