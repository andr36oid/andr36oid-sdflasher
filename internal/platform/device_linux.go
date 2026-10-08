package platform

import (
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"strings"
	"unsafe"
)

type block struct {
	Name, Path, Type, Model, Serial, Tran string
	Size                                  int64
	RM                                    bool
	RO                                    bool
	Sector                                int64     `json:"log-sec"`
	Mounts                                []*string `json:"mountpoints"`
	Children                              []block
}

func blocks() ([]block, error) {
	b, e := hostCommand("lsblk", "--bytes", "--json", "--output", "NAME,PATH,TYPE,SIZE,MODEL,SERIAL,RM,TRAN,MOUNTPOINTS,LOG-SEC,RO").Output()
	if e != nil {
		return nil, e
	}
	var v struct{ Blockdevices []block }
	e = json.Unmarshal(b, &v)
	return v.Blockdevices, e
}
func critical(b block) bool {
	for _, m := range b.Mounts {
		if m == nil {
			continue
		}
		s := *m
		if s == "/" || s == "/home" || s == "/usr" || s == "/var" || s == "/boot" || strings.HasPrefix(s, "/boot/") || s == "[SWAP]" {
			return true
		}
	}
	for _, c := range b.Children {
		if critical(c) {
			return true
		}
	}
	return false
}
func List() ([]Drive, error) {
	bs, e := blocks()
	if e != nil {
		return nil, e
	}
	var out []Drive
	for _, b := range bs {
		if b.Type != "disk" || b.RO || (!b.RM && b.Tran != "usb") || critical(b) || b.Size < 1<<30 {
			continue
		}
		d := Drive{Path: b.Path, Name: strings.TrimSpace(b.Model), Serial: strings.TrimSpace(b.Serial), Size: b.Size, SectorSize: b.Sector}
		if d.Name == "" {
			d.Name = b.Name
		}
		var add func(block)
		add = func(x block) {
			for _, m := range x.Mounts {
				if m != nil && *m != "" {
					d.Volumes = append(d.Volumes, x.Path)
					break
				}
			}
			for _, c := range x.Children {
				add(c)
			}
		}
		add(b)
		out = append(out, d)
	}
	return out, nil
}
func Open(d Drive, write bool) (*Handle, error) {
	d, e := Revalidate(d)
	if e != nil {
		return nil, e
	}
	if write {
		for _, v := range d.Volumes {
			if b, e := exec.Command("umount", "--", v).CombinedOutput(); e != nil {
				return nil, fmt.Errorf("cannot unmount %s: %s", v, b)
			}
		}
	}
	flag := os.O_RDONLY
	if write {
		flag = os.O_RDWR | unix.O_EXCL
	}
	f, e := os.OpenFile(d.Path, flag, 0)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	var size uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		e = errno
	}
	if e != nil || int64(size) != d.Size {
		f.Close()
		return nil, fmt.Errorf("card capacity changed")
	}
	return &Handle{File: f, Cleanup: func() {
		if write {
			exec.Command("blockdev", "--rereadpt", d.Path).Run()
		}
	}}, nil
}
