package flash

// The first-boot resizer on the card (Image-resizing, linuxrc in
// device/gameconsole/common) creates userdata and EASYROMS. UserdataLayout
// repeats its rule so the flasher can show what will happen; keep the two in step.

const (
	gib            = int64(1) << 30
	userdataStart  = int64(5701009) * 512
	MinUserdataGiB = 4
	minROMs        = 1 * gib
)

// UserdataLayout returns the sizes in bytes the resizer will give userdata
// and EASYROMS on a card of size bytes. wantGiB 0 means automatic (half of the
// free space, 8 to 64 GiB). EASYROMS is 0 when it is not made: noROMs, or less
// than 1 GiB would be left.
func UserdataLayout(size int64, wantGiB int, noROMs bool) (userdata, roms int64) {
	last := size/512 - 34
	free := (last - userdataStart/512 + 1) * 512
	if free <= 0 {
		return 0, 0
	}
	if noROMs {
		return free, 0
	}
	want := int64(wantGiB) * gib
	if wantGiB <= 0 {
		want = free / 2 / 512 * 512
		want = max(want, 8*gib)
		want = min(want, 64*gib)
	} else {
		want = max(want, MinUserdataGiB*gib)
	}
	end := (userdataStart/512+want/512)/2048*2048 - 1
	if last-end < minROMs/512 {
		return free, 0
	}
	userdata = (end - userdataStart/512 + 1) * 512
	return userdata, free - userdata
}

// MaxUserdataGiB is the largest whole-GiB userdata that still leaves a games
// partition; 0 when the card is too small for one.
func MaxUserdataGiB(size int64) int {
	for g := int((size - userdataStart) / gib); g >= MinUserdataGiB; g-- {
		if _, roms := UserdataLayout(size, g, false); roms > 0 {
			return g
		}
	}
	return 0
}
