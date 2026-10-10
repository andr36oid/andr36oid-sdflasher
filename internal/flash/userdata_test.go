package flash

import "testing"

const gb = int64(1000 * 1000 * 1000)

// Same cases as tests/plan_userdata_test.sh next to the resizer's linuxrc.
func TestUserdataLayout(t *testing.T) {
	half := func(size int64) int64 {
		last := size/512 - 34
		end := (5701009+(last-5701009+1)/2)/2048*2048 - 1
		return (end - 5701009 + 1) * 512
	}
	whole := func(g int64) int64 { return ((5701009+g*2097152)/2048*2048 - 5701009) * 512 }
	all := int64(-1)
	cases := []struct {
		size   int64
		want   int
		noROMs bool
		exp    int64
	}{
		{20113457152, 0, false, all},
		{24 * gb, 0, false, whole(16)},
		{32 * gb, 0, false, whole(16)},
		{16 * gb, 0, false, all},
		{8 * gb, 0, false, all},
		{64 * gb, 0, false, half(64 * gb)},
		{512 * gb, 0, false, whole(64)},
		{64 * gb, 16, true, all},
		{64 * gb, 16, false, whole(16)},
		{64 * gb, 56, false, all},
		{32 * gb, 100, false, all},
		{64 * gb, 1, false, whole(16)},
		{64 * gb, 8, false, whole(16)},
		{16 * gb, 8, false, all},
	}
	for _, c := range cases {
		u, r := UserdataLayout(c.size, c.want, c.noROMs)
		free := (c.size/512 - 34 - 5701009 + 1) * 512
		if c.exp == all {
			if u != free || r != 0 {
				t.Errorf("%d %d %v: got %d/%d, want all %d", c.size, c.want, c.noROMs, u, r, free)
			}
			continue
		}
		if u != c.exp || u+r != free || r < 1<<30 {
			t.Errorf("%d %d %v: got %d/%d, want %d", c.size, c.want, c.noROMs, u, r, c.exp)
		}
	}
}

func TestMaxUserdataGiB(t *testing.T) {
	for _, size := range []int64{8 * gb, 16 * gb, 32 * gb, 128 * gb} {
		m := MaxUserdataGiB(size)
		if m == 0 {
			// 16 GiB for Android and 1 GiB for games need more than 16 GB
			if size > 20*gb {
				t.Errorf("%d: no games partition possible", size)
			}
			continue
		}
		if _, r := UserdataLayout(size, m, false); r == 0 {
			t.Errorf("%d: max %d leaves no games partition", size, m)
		}
		if _, r := UserdataLayout(size, m+1, false); r != 0 {
			t.Errorf("%d: max %d is not the largest", size, m)
		}
	}
}
