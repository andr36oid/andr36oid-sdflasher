package firmware

import "testing"

func TestTooOld(t *testing.T) {
	for name, want := range map[string]string{
		"lineage-18.1-20250902-2052-r36s-android.img.zip":     "2025-09-02",
		"lineage-18.1-202507181828-r36s-android.img.zip":      "2025-07-18",
		"lineage-18.1-20250712-r36s-android.zip":              "2025-07-12",
		`C:\Users\x\Downloads\lineage-18.1-20260831-1200.img`: "2026-08-31",
		"/home/x/lineage-18.1-20251109-0900-r46h-android.img": "2025-11-09",
		"lineage-18.1-20260901-0001-r36s-android.img.zip":     "",
		"lineage-18.1-20261011-1531-r36s-android.img.zip":     "",
		"my-card.img":                    "",
		"lineage-18.1-20250231-r36s.img": "",
		"backup-120250902.img":           "",
	} {
		got, old := TooOld(name)
		if got != want || old != (want != "") {
			t.Errorf("%s: got %q %v, want %q", name, got, old, want)
		}
	}
}
