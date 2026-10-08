package firmware

import (
	"archive/zip"
	"context"
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
