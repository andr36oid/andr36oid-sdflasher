package extfs

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadExtentsAndSymlinks(t *testing.T) {
	mk, e := exec.LookPath("mke2fs")
	if e != nil {
		if _, err := os.Stat("/usr/sbin/mke2fs"); err != nil {
			t.Skip("mke2fs not installed")
		}
		mk = "/usr/sbin/mke2fs"
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	os.MkdirAll(filepath.Join(root, "system"), 0700)
	payload := bytes.Repeat([]byte("system content\n"), 4096)
	os.WriteFile(filepath.Join(root, "system", "build.prop"), payload, 0600)
	os.Symlink("system/build.prop", filepath.Join(root, "version"))
	image := filepath.Join(dir, "fs.img")
	f, e := os.Create(image)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	f.Truncate(32 << 20)
	if b, e := exec.Command(mk, "-q", "-F", "-t", "ext4", "-d", root, image).CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
	fs, e := Open(f, 0, 32<<20)
	if e != nil {
		t.Fatal(e)
	}
	got, e := fs.ReadFile("/version", 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("filesystem content changed")
	}
	if _, e = fs.ReadFile("/version", 16); e == nil {
		t.Fatal("ignored read limit")
	}
	f.WriteAt([]byte{0}, 1024+56)
	if _, e = Open(f, 0, 32<<20); e == nil {
		t.Fatal("accepted corrupt filesystem")
	}
}
func FuzzOpen(f *testing.F) {
	f.Add(make([]byte, 2048))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		Open(bytes.NewReader(b), 0, int64(len(b)))
	})
}
