package catalog

import (
	"testing"
	"time"
)

func TestUsableSkipsOldBuilds(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
	got := usable([]release{
		{Tag: "20250902", Published: day("2025-09-02"), Assets: []Asset{{Name: "lineage-18.1-20250902-2052-r36s-android.img.zip"}}},
		{Tag: "rc1.1", Published: day("2026-10-11"), Assets: []Asset{
			{Name: "lineage-18.1-20261011-1531-r36s-android.img.zip"},
			{Name: "lineage-18.1-20251109-0900-r46h-android.img.zip"},
			{Name: "notes.txt"},
		}},
		{Tag: "draft", Draft: true, Published: day("2026-10-12"), Assets: []Asset{{Name: "lineage-18.1-20261012-0900-r36s-android.img.zip"}}},
	})
	if len(got) != 1 || got[0].Name != "lineage-18.1-20261011-1531-r36s-android.img.zip" || got[0].Release != "rc1.1" {
		t.Fatalf("got %+v", got)
	}
}

func TestUsablePrefersXZ(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }
	got := usable([]release{
		{Tag: "rc3", Published: day("2026-10-18"), Assets: []Asset{
			{Name: "andr36oid-20261018-1200-rk3326.img.zip"},
			{Name: "andr36oid-20261018-1200-rk3326.img.xz"},
			{Name: "andr36oid-20261018-1200-rk3326.img.xz.sha256"},
		}},
		{Tag: "rc2", Published: day("2026-10-17"), Assets: []Asset{{Name: "andr36oid-20261017-0354-rk3326.img.zip"}}},
	})
	if len(got) != 2 || got[0].Name != "andr36oid-20261018-1200-rk3326.img.xz" || got[1].Name != "andr36oid-20261017-0354-rk3326.img.zip" {
		t.Fatalf("got %+v", got)
	}
}
