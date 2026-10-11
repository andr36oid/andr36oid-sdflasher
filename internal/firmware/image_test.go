package firmware

import (
	"archive/zip"
	"context"
	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/fat32"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseImage(t *testing.T) {
	p := os.Getenv("SDFLASHER_TEST_IMAGE")
	if p == "" {
		t.Skip("set SDFLASHER_TEST_IMAGE for release-image integration")
	}
	im, e := Inspect(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(im.Profiles) < 8 {
		t.Fatal("missing panel profiles")
	}
	for _, p := range im.Profiles {
		if p.ID != "Devices/R46H" {
			continue
		}
		charge := 0
		for _, n := range p.BootFiles {
			if filepath.Base(n) == "battery_0.bmp" || filepath.Base(n) == "battery_fail.bmp" {
				charge++
			}
		}
		if charge != 2 {
			t.Fatal("R46H charge screens not copied", p.BootFiles)
		}
	}
	for _, p := range im.Profiles {
		if p.ID == "Panels/Panel4" {
			for _, update := range []bool{false, true} {
				boot, e := PrepareBoot(im, p.ID, true, update, "", t.TempDir())
				if e != nil {
					t.Fatal(e)
				}
				st, e := os.Stat(boot)
				if e != nil || st.Size() != 256<<20 {
					t.Fatal("invalid prepared boot")
				}
			}
			return
		}
	}
	t.Fatal("Panel 4 missing")
}
func TestLegacyImageRejected(t *testing.T) {
	p := os.Getenv("SDFLASHER_TEST_LEGACY")
	if p == "" {
		t.Skip("set SDFLASHER_TEST_LEGACY")
	}
	if _, e := Inspect(context.Background(), p, nil); e == nil {
		t.Fatal("accepted non-Treble image")
	}
}
func TestArchiveRejectsAmbiguousAndDamagedImages(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "two.zip")
	f, _ := os.Create(p)
	w := zip.NewWriter(f)
	for _, n := range []string{"a.img", "b.img"} {
		x, _ := w.Create(n)
		x.Write([]byte("not a disk"))
	}
	w.Close()
	f.Close()
	if _, e := Stage(context.Background(), p, dir, nil); e == nil {
		t.Fatal("accepted multiple images")
	}
	p = filepath.Join(dir, "broken.zip")
	f, _ = os.Create(p)
	w = zip.NewWriter(f)
	x, _ := w.CreateHeader(&zip.FileHeader{Name: "disk.img", Method: zip.Store})
	x.Write([]byte("image payload"))
	w.Close()
	f.Close()
	b, _ := os.ReadFile(p)
	b[38] ^= 1
	os.WriteFile(p, b, 0600)
	if _, e := Stage(context.Background(), p, dir, nil); e == nil {
		t.Fatal("accepted corrupt ZIP")
	}
}
func TestUserdataSizeFile(t *testing.T) {
	p := os.Getenv("SDFLASHER_TEST_IMAGE")
	if p == "" {
		t.Skip("set SDFLASHER_TEST_IMAGE for release-image integration")
	}
	im, e := Inspect(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e)
	}
	read := func(gib int, noROMs, update bool) (string, bool) {
		boot, e := PrepareBoot(im, "Panels/Panel4", noROMs, update, "", t.TempDir(), BootOptions{UserdataGiB: gib})
		if e != nil {
			t.Fatal(e)
		}
		f, e := os.Open(boot)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		st, _ := f.Stat()
		fs, e := fat32.Read(file.New(f, true), st.Size(), 0, 512)
		if e != nil {
			t.Fatal(e)
		}
		b, e := ReadFile(fs, "/userdata-size.txt", 64)
		return string(b), e == nil
	}
	if s, ok := read(32, false, false); !ok || s != "32\n" {
		t.Errorf("install with 32 GiB wrote %q %v", s, ok)
	}
	for _, c := range []struct {
		gib            int
		noROMs, update bool
	}{{0, false, false}, {32, true, false}} {
		if s, ok := read(c.gib, c.noROMs, c.update); ok {
			t.Errorf("%+v wrote %q", c, s)
		}
	}
}
