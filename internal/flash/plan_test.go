package flash

import (
	"bytes"
	"context"
	"errors"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/layout"
	"os"
	"path/filepath"
	"testing"
)

func sample(t *testing.T) (*firmware.Image, Card, *os.File, string) {
	t.Helper()
	size := int64(8 << 20)
	path := filepath.Join(t.TempDir(), "source.img")
	raw := bytes.Repeat([]byte{0x5a}, int(size))
	os.WriteFile(path, raw, 0600)
	tab := layout.Table{GPT: true, ID: layout.GUID(), Size: size}
	names := []string{"idbloader", "uboot", "trust", "BOOT", "vendor", "metadata", "misc", "cache", "system"}
	nums := []int{3, 4, 5, 1, 8, 9, 10, 11, 2}
	start := uint64(64)
	for i, n := range names {
		tab.Parts = append(tab.Parts, layout.Partition{Number: nums[i], Name: n, Start: start, End: start + 255, Type: layout.LinuxType, ID: layout.GUID()})
		start += 256
	}
	im := &firmware.Image{Path: path, Size: size, Table: tab, Profiles: []firmware.Profile{{ID: "Panels/Panel4"}}}
	sf, _ := os.Open(path)
	im.SHA256, _ = firmware.Hash(context.Background(), sf, size, nil)
	sf.Close()
	target, e := os.Create(filepath.Join(t.TempDir(), "card.img"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { target.Close() })
	target.Truncate(16 << 20)
	fill := bytes.Repeat([]byte{0xa7}, 16<<20)
	target.WriteAt(fill, 0)
	old := tab
	old.Parts = append([]layout.Partition{}, tab.Parts...)
	old.Size = 16 << 20
	old.Parts = append(old.Parts, layout.Partition{Number: 6, Name: "userdata", Start: 8192, End: 16383, Type: layout.LinuxType, ID: layout.GUID()}, layout.Partition{Number: 7, Name: "EASYROMS", Start: 16384, End: 30000, Type: layout.LinuxType, ID: layout.GUID()})
	finger, _ := Snapshot(target, old.Size)
	card := Card{Installed: true, Treble: true, Table: old, Fingerprint: finger}
	boot := filepath.Join(t.TempDir(), "boot.fat")
	os.WriteFile(boot, bytes.Repeat([]byte{0x3c}, 256*512), 0600)
	return im, card, target, boot
}
func TestUpdatePreservesUserDataGamesAndMetadata(t *testing.T) {
	im, card, dst, boot := sample(t)
	p, e := Build(im, card, card.Table.Size, "update", "Panels/Panel4", false)
	if e != nil {
		t.Fatal(e)
	}
	before := map[string][]byte{}
	for _, r := range p.Protected {
		b := make([]byte, r.Length)
		dst.ReadAt(b, r.Offset)
		before[r.Name] = b
	}
	if e = Execute(context.Background(), dst, im, p, boot, t.TempDir(), nil); e != nil {
		t.Fatal(e)
	}
	for _, r := range p.Protected {
		b := make([]byte, r.Length)
		dst.ReadAt(b, r.Offset)
		if !bytes.Equal(b, before[r.Name]) {
			t.Fatalf("changed protected %s", r.Name)
		}
	}
	tab, e := layout.Read(dst, p.Size)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := tab.Find("userdata")
	old, _ := card.Table.Find("userdata")
	if u != old {
		t.Fatal("userdata geometry or identity changed")
	}
}

type faulty struct {
	*os.File
	writes int
	failAt int
}

func (f *faulty) WriteAt(b []byte, off int64) (int, error) {
	f.writes++
	if f.writes == f.failAt {
		return 0, errors.New("simulated disconnect")
	}
	return f.File.WriteAt(b, off)
}
func TestInterruptedUpdateRestoresOriginal(t *testing.T) {
	im, card, dst, boot := sample(t)
	p, e := Build(im, card, card.Table.Size, "update", "Panels/Panel4", false)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := firmware.Hash(context.Background(), dst, p.Size, nil)
	bad := &faulty{File: dst, failAt: 3}
	if e = Execute(context.Background(), bad, im, p, boot, t.TempDir(), nil); e == nil {
		t.Fatal("failure not reported")
	}
	after, _ := firmware.Hash(context.Background(), dst, p.Size, nil)
	if before != after {
		t.Fatal("restore did not recover exact original card")
	}
}
func TestPreflightRejectsChangedImageAndCardWithoutWrites(t *testing.T) {
	for _, kind := range []string{"image", "card", "overlap"} {
		t.Run(kind, func(t *testing.T) {
			im, card, dst, boot := sample(t)
			p, _ := Build(im, card, card.Table.Size, "update", "Panels/Panel4", false)
			switch kind {
			case "image":
				f, _ := os.OpenFile(im.Path, os.O_RDWR, 0)
				f.WriteAt([]byte{1}, 1000)
				f.Close()
			case "card":
				dst.WriteAt([]byte{1}, 0)
			case "overlap":
				p.Writes[0].Offset = p.Protected[0].Offset
			}
			bad := &faulty{File: dst, failAt: 999}
			if e := Execute(context.Background(), bad, im, p, boot, t.TempDir(), nil); e == nil {
				t.Fatal("accepted unsafe preflight")
			}
			if bad.writes != 0 {
				t.Fatal("preflight wrote to card")
			}
		})
	}
}
func TestMigrationProtectsMBRDataAndRefusesNoTailRoom(t *testing.T) {
	im, card, _, _ := sample(t)
	card.Treble = false
	card.Table.GPT = false
	var old []layout.Partition
	for _, p := range card.Table.Parts {
		if p.Name == "BOOT" || p.Name == "system" || p.Name == "userdata" {
			old = append(old, p)
		}
	}
	card.Table.Parts = old
	if _, e := Build(im, card, card.Table.Size, "update", "Panels/Panel4", false); e != nil {
		t.Fatal(e)
	}
	for i := range card.Table.Parts {
		if card.Table.Parts[i].Name == "userdata" {
			card.Table.Parts[i].End = uint64(card.Table.Size/512) - 1
		}
	}
	if _, e := Build(im, card, card.Table.Size, "update", "Panels/Panel4", false); e == nil {
		t.Fatal("accepted GPT backup over userdata")
	}
}
func TestWriteGuardRejectsWritesOutsidePlan(t *testing.T) {
	_, _, dst, _ := sample(t)
	p := &Plan{Size: 16 << 20, Writes: []Write{{Range: Range{"system", 1024, 1024}}}, Protected: []Range{{"userdata", 2048, 4096}}}
	g := &guardedDevice{Device: dst, plan: p}
	for _, off := range []int64{-1, 0, 2048, 16 << 20} {
		if _, e := g.WriteAt(make([]byte, 512), off); e == nil {
			t.Fatalf("allowed write at %d", off)
		}
	}
	if _, e := g.WriteAt(make([]byte, 512), 1024); e != nil {
		t.Fatal(e)
	}
}
