package firmware

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/fat32"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

const CustomWarning = "Custom DTBs must be built specifically for andr36oid. DTBs from other custom firmwares are unlikely to work."

func PrepareBoot(im *Image, profile string, noROMs, updating bool, custom, dir string) (string, error) {
	src, e := os.Open(im.Path)
	if e != nil {
		return "", e
	}
	defer src.Close()
	p, _ := im.Table.Find("BOOT")
	out, e := os.CreateTemp(dir, "boot-*.fat")
	if e != nil {
		return "", e
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(out.Name())
		}
	}()
	if _, e = io.Copy(out, io.NewSectionReader(src, p.Offset(), p.Size())); e != nil {
		return "", e
	}
	fs, e := fat32.Read(file.New(out, false), p.Size(), 0, 512)
	if e != nil {
		return "", e
	}
	var selected *Profile
	for i := range im.Profiles {
		if im.Profiles[i].ID == profile {
			selected = &im.Profiles[i]
		}
	}
	if selected == nil {
		return "", errors.New("profile missing from image")
	}
	write := func(name string, b []byte) error {
		f, e := fs.OpenFile(strings.TrimPrefix(name, "/"), os.O_CREATE|os.O_TRUNC|os.O_RDWR)
		if e != nil {
			return e
		}
		_, e = f.Write(b)
		ce := f.Close()
		if e != nil {
			return e
		}
		return ce
	}
	script, e := ReadFile(fs, "/boot.ini-android", 128<<10)
	if e != nil {
		return "", e
	}
	for _, n := range selected.BootFiles {
		if path.Base(n) == "boot.ini-android" {
			script, e = ReadFile(fs, n, 128<<10)
			if e != nil {
				return "", e
			}
		}
	}
	if !bytes.Contains(script, []byte("ramdisk.uimg")) {
		return "", errors.New("selected profile has no Treble boot script")
	}
	re := regexp.MustCompile(`(?m)^\s*load\s+mmc\s+\S+\s+\S+\s+([^\s]+\.dtb)\s*$`)
	m := re.FindSubmatch(script)
	if len(m) != 2 {
		return "", errors.New("cannot determine Android DTB destination")
	}
	target := string(m[1])
	if strings.Contains(target, "/") || strings.Contains(target, "..") {
		return "", errors.New("invalid DTB destination")
	}
	dtb, e := ReadFile(fs, selected.Kernel, 4<<20)
	if e != nil {
		return "", e
	}
	if custom != "" {
		st, e := os.Stat(custom)
		if e != nil {
			return "", e
		}
		if !st.Mode().IsRegular() || st.Size() > 4<<20 {
			return "", errors.New("custom DTB is too large or not a regular file")
		}
		dtb, e = os.ReadFile(custom)
		if e != nil {
			return "", e
		}
		if !validDTB(dtb) {
			return "", errors.New("custom file is not a valid flattened device tree")
		}
	}
	if e = write(target, dtb); e != nil {
		return "", e
	}
	for _, n := range selected.BootFiles {
		b, e := ReadFile(fs, n, 16<<20)
		if e != nil {
			return "", e
		}
		if e = write(path.Base(n), b); e != nil {
			return "", e
		}
		if strings.HasSuffix(n, ".dtb") {
			for _, alias := range []string{"rg351mp-kernel.dtb", "rg351p-kernel.dtb", "rg351v-kernel.dtb"} {
				if e = write(alias, b); e != nil {
					return "", e
				}
			}
		}
	}
	// The installation resizer formats userdata. It must never run on an update.
	if updating {
		if e = write("boot.ini", script); e != nil {
			return "", e
		}
	}
	if noROMs {
		if e = write(".noroms", nil); e != nil {
			return "", e
		}
	} else {
		if _, e = ReadFile(fs, "/.noroms", 64); e == nil {
			if e = fs.Remove(".noroms"); e != nil {
				return "", e
			}
		}
	}
	if e = write("andr36oid-profile.txt", []byte(profile+"\n")); e != nil {
		return "", e
	}
	if e = out.Sync(); e != nil {
		return "", e
	}
	check, e := ReadFile(fs, "/"+target, 4<<20)
	if e != nil || !bytes.Equal(check, dtb) {
		return "", fmt.Errorf("prepared DTB did not verify")
	}
	if updating {
		b, e := ReadFile(fs, "/boot.ini", 128<<10)
		if e != nil || !bytes.Equal(b, script) {
			return "", errors.New("prepared Android boot script did not verify")
		}
	}
	ok = true
	return out.Name(), nil
}
