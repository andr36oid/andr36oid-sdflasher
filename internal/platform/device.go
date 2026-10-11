package platform

import (
	"errors"
	"fmt"
	"os"
)

type Drive struct {
	Path       string   `json:"path"`
	Name       string   `json:"name"`
	Serial     string   `json:"serial"`
	Size       int64    `json:"size"`
	SectorSize int64    `json:"sector_size"`
	Volumes    []string `json:"volumes,omitempty"`
}

func (d Drive) Label() string {
	return fmt.Sprintf("%s · %.1f GB · %s", d.Name, float64(d.Size)/1e9, d.Path)
}

// Same reports whether two listings describe the same card.
func Same(a, b Drive) bool {
	return difference(a, b) == ""
}

// difference names the first thing that tells two listings apart, "" if nothing
// does. The disk, size and sector size must match. The name and serial only count
// when both listings have one: Windows can show a card reader's serial to the admin
// helper but not to the app that picked the card, and the same card then never
// matched after the admin prompt.
func difference(a, b Drive) string {
	switch {
	case a.Path != b.Path:
		return "disk"
	case a.Size != b.Size:
		return fmt.Sprintf("size %d, was %d", a.Size, b.Size)
	case a.SectorSize != b.SectorSize:
		return fmt.Sprintf("sector size %d, was %d", a.SectorSize, b.SectorSize)
	case a.Name != "" && b.Name != "" && a.Name != b.Name:
		return fmt.Sprintf("name %q, was %q", a.Name, b.Name)
	case a.Serial != "" && b.Serial != "" && a.Serial != b.Serial:
		return fmt.Sprintf("serial %q, was %q", a.Serial, b.Serial)
	}
	return ""
}

func Revalidate(want Drive) (Drive, error) {
	ds, e := List()
	if e != nil {
		return Drive{}, e
	}
	why := ""
	for _, d := range ds {
		if Same(d, want) {
			if d.SectorSize != 512 {
				return d, errors.New("only 512-byte logical sectors are supported")
			}
			return d, nil
		}
		if d.Path == want.Path {
			why = " (" + d.Path + " now has " + difference(d, want) + ")"
		}
	}
	return Drive{}, errors.New("the selected card was removed, changed, or is no longer safe to open" + why)
}

type Handle struct {
	File    *os.File
	Cleanup func()
}

func (h *Handle) Close() {
	h.File.Close()
	if h.Cleanup != nil {
		h.Cleanup()
	}
}
