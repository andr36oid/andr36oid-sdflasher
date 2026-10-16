package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Eject locks and dismounts every volume of the card, as Open does, then asks the
// reader to eject the card (IOCTL_STORAGE_EJECT_MEDIA). A volume that is still in
// use (a window showing it, a file open on it) can't be locked.
func Eject(d Drive) error {
	d, e := Revalidate(d)
	if e != nil {
		return e
	}
	var held []windows.Handle
	defer func() {
		for _, h := range held {
			windows.CloseHandle(h)
		}
	}()
	var n uint32
	for _, v := range d.Volumes {
		p, e := windows.UTF16PtrFromString(v)
		if e != nil {
			return e
		}
		h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if e != nil {
			return fmt.Errorf("cannot open %s: %w", v, e)
		}
		held = append(held, h)
		for _, op := range []uint32{0x90018, 0x90020} { // FSCTL_LOCK_VOLUME, FSCTL_DISMOUNT_VOLUME
			if e = windows.DeviceIoControl(h, op, nil, 0, nil, 0, &n, nil); e != nil {
				return fmt.Errorf("cannot lock/unmount %s (still in use?): %w", v, e)
			}
		}
	}
	p, e := windows.UTF16PtrFromString(d.Path)
	if e != nil {
		return e
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if e != nil {
		return fmt.Errorf("cannot open %s: %w", d.Path, e)
	}
	defer windows.CloseHandle(h)
	if e = windows.DeviceIoControl(h, 0x2D4808, nil, 0, nil, 0, &n, nil); e != nil { // IOCTL_STORAGE_EJECT_MEDIA
		return fmt.Errorf("cannot eject %s: %w", d.Path, e)
	}
	return nil
}
