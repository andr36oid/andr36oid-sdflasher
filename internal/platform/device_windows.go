package platform

import (
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"strings"
	"unsafe"
)

func List() ([]Drive, error) {
	script := `$ErrorActionPreference='Stop'; $d=@(Get-Disk | Where-Object { -not $_.IsBoot -and -not $_.IsSystem -and -not $_.IsReadOnly -and $_.BusType -in @('USB','SD','MMC') } | ForEach-Object { $v=@(Get-Partition -DiskNumber $_.Number -ErrorAction SilentlyContinue | ForEach-Object { $_.AccessPaths | Where-Object { $_.StartsWith('\\?\Volume{') } | ForEach-Object { $_.TrimEnd('\') } }); @{ path=('\\.\PhysicalDrive'+$_.Number); name=$_.FriendlyName; serial=$_.SerialNumber; size=[long]$_.Size; sector_size=[long]$_.LogicalSectorSize; volumes=$v } }); ConvertTo-Json -InputObject $d -Compress -Depth 4`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	hide(cmd)
	b, e := cmd.Output()
	if e != nil {
		return nil, e
	}
	var ds []Drive
	e = json.Unmarshal(b, &ds)
	for i := range ds {
		ds[i].Serial = strings.TrimSpace(ds[i].Serial)
	}
	return ds, e
}
func Open(d Drive, write bool) (*Handle, error) {
	d, e := Revalidate(d)
	if e != nil {
		return nil, e
	}
	var volumes []windows.Handle
	cleanup := func() {
		for _, h := range volumes {
			windows.CloseHandle(h)
		}
	}
	if write {
		for _, v := range d.Volumes {
			p, e := windows.UTF16PtrFromString(v)
			if e != nil {
				cleanup()
				return nil, e
			}
			h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
			if e != nil {
				cleanup()
				return nil, e
			}
			volumes = append(volumes, h)
			var n uint32
			for _, op := range []uint32{0x90018, 0x90020} {
				if e = windows.DeviceIoControl(h, op, nil, 0, nil, 0, &n, nil); e != nil {
					cleanup()
					return nil, fmt.Errorf("cannot lock/unmount %s: %w", v, e)
				}
			}
		}
	}
	p, e := windows.UTF16PtrFromString(d.Path)
	if e != nil {
		cleanup()
		return nil, e
	}
	access := uint32(windows.GENERIC_READ)
	if write {
		access |= windows.GENERIC_WRITE
	}
	h, e := windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_WRITE_THROUGH, 0)
	if e != nil {
		cleanup()
		return nil, e
	}
	var n uint32
	var size int64
	if e = windows.DeviceIoControl(h, 0x7405c, nil, 0, (*byte)(unsafe.Pointer(&size)), 8, &n, nil); e != nil || size != d.Size {
		windows.CloseHandle(h)
		cleanup()
		return nil, fmt.Errorf("card capacity changed")
	}
	f := os.NewFile(uintptr(h), d.Path)
	return &Handle{File: f, Cleanup: cleanup}, nil
}
