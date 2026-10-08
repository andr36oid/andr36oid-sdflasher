package layout

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func fixture() Table {
	return Table{GPT: true, ID: GUID(), Size: 16 << 20, Parts: []Partition{{Number: 1, Name: "BOOT", Start: 64, End: 1023, Type: LinuxType, ID: GUID()}, {Number: 6, Name: "userdata", Start: 2048, End: 30000, Type: LinuxType, ID: GUID()}}}
}
func TestRoundTripAndCorruption(t *testing.T) {
	tab := fixture()
	head, tail, e := tab.Encode()
	if e != nil {
		t.Fatal(e)
	}
	buf := make([]byte, tab.Size)
	copy(buf, head)
	copy(buf[int64(len(buf))-int64(len(tail)):], tail)
	got, e := Read(bytes.NewReader(buf), tab.Size)
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Parts) != 2 || got.Parts[1] != tab.Parts[1] || got.ID != tab.ID {
		t.Fatal("partition identity changed")
	}
	for _, off := range []int{512 + 56, 1024 + 32} {
		bad := append([]byte{}, buf...)
		bad[off] ^= 1
		if _, e := Read(bytes.NewReader(bad), tab.Size); e == nil {
			t.Fatalf("accepted corrupt byte %d", off)
		}
	}
}
func TestRejectOverlapAndTailCollision(t *testing.T) {
	tab := fixture()
	tab.Parts[1].Start = 1000
	if e := tab.Validate(true); e == nil {
		t.Fatal("accepted overlapping partitions")
	}
	tab = fixture()
	tab.Parts[1].End = uint64(tab.Size/512) - 1
	if _, _, e := tab.Encode(); e == nil {
		t.Fatal("accepted backup GPT over userdata")
	}
}
func TestMBR(t *testing.T) {
	b := make([]byte, 1024)
	b[510] = 0x55
	b[511] = 0xaa
	b[450] = 0x83
	binary.LittleEndian.PutUint32(b[454:], 2048)
	binary.LittleEndian.PutUint32(b[458:], 4096)
	tab, e := Read(bytes.NewReader(b), 16<<20)
	if e != nil {
		t.Fatal(e)
	}
	if tab.GPT || tab.Parts[0].Start != 2048 || tab.Parts[0].End != 6143 {
		t.Fatal(tab)
	}
	b[450] = 0x0f
	if _, e = Read(bytes.NewReader(b), 16<<20); e == nil {
		t.Fatal("accepted extended partition")
	}
}
func FuzzRead(f *testing.F) {
	f.Add(make([]byte, 1024))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		Read(bytes.NewReader(b), 16<<20)
	})
}
