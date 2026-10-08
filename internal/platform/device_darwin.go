package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func diskInfo(args ...string) (map[string]any, error) {
	b, e := exec.Command("/usr/sbin/diskutil", args...).Output()
	if e != nil {
		return nil, e
	}
	cmd := exec.Command("/usr/bin/plutil", "-convert", "json", "-o", "-", "--", "-")
	cmd.Stdin = strings.NewReader(string(b))
	b, e = cmd.Output()
	if e != nil {
		return nil, e
	}
	var v map[string]any
	e = json.Unmarshal(b, &v)
	return v, e
}
func List() ([]Drive, error) {
	v, e := diskInfo("list", "-plist", "external", "physical")
	if e != nil {
		return nil, e
	}

	protected := map[string]bool{}
	root, e := diskInfo("info", "-plist", "/")
	if e != nil {
		return nil, e
	}
	rootID, _ := root["ParentWholeDisk"].(string)
	protected[rootID] = true
	apfs, e := diskInfo("apfs", "list", "-plist")
	if e != nil {
		return nil, fmt.Errorf("cannot identify the system disk: %w", e)
	}
	containers, _ := apfs["Containers"].([]any)
	base := regexp.MustCompile(`^disk[0-9]+`)
	for _, entry := range containers {
		c, _ := entry.(map[string]any)
		ref, _ := c["ContainerReference"].(string)
		system := ref == rootID
		volumes, _ := c["Volumes"].([]any)
		for _, v := range volumes {
			vm, _ := v.(map[string]any)
			mp, _ := vm["MountPoint"].(string)
			if mp == "/" || strings.HasPrefix(mp, "/System/Volumes/") {
				system = true
			}
		}
		if system {
			stores, _ := c["PhysicalStores"].([]any)
			for _, store := range stores {
				sm, _ := store.(map[string]any)
				id, _ := sm["DeviceIdentifier"].(string)
				protected[base.FindString(id)] = true
			}
		}
	}
	var out []Drive
	disks, _ := v["AllDisksAndPartitions"].([]any)
	for _, entry := range disks {
		m, _ := entry.(map[string]any)
		id, _ := m["DeviceIdentifier"].(string)
		if id == "" || protected[id] {
			continue
		}
		inf, e := diskInfo("info", "-plist", id)
		if e != nil {
			continue
		}
		internal, _ := inf["Internal"].(bool)
		writable, _ := inf["Writable"].(bool)
		whole, _ := inf["Whole"].(bool)
		if internal || !writable || !whole {
			continue
		}
		size, _ := inf["TotalSize"].(float64)
		sec, _ := inf["DeviceBlockSize"].(float64)
		name, _ := inf["MediaName"].(string)
		serial, _ := inf["DiskUUID"].(string)
		out = append(out, Drive{Path: "/dev/" + id, Name: name, Serial: serial, Size: int64(size), SectorSize: int64(sec)})
	}
	return out, nil
}
func Open(d Drive, write bool) (*Handle, error) {
	d, e := Revalidate(d)
	if e != nil {
		return nil, e
	}
	if write {
		if b, e := exec.Command("/usr/sbin/diskutil", "unmountDisk", d.Path).CombinedOutput(); e != nil {
			return nil, fmt.Errorf("cannot unmount card: %s", b)
		}
	}
	flag := os.O_RDONLY
	if write {
		flag = os.O_RDWR
	}
	f, e := os.OpenFile(strings.Replace(d.Path, "/dev/disk", "/dev/rdisk", 1), flag, 0)
	if e != nil {
		return nil, e
	}
	return &Handle{File: f, Cleanup: func() {
		if write {
			exec.Command("/usr/sbin/diskutil", "eject", d.Path).Run()
		}
	}}, nil
}
