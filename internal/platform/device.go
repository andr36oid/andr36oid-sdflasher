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
func Same(a, b Drive) bool {
	return a.Path == b.Path && a.Name == b.Name && a.Serial == b.Serial && a.Size == b.Size && a.SectorSize == b.SectorSize
}
func Revalidate(want Drive) (Drive, error) {
	ds, e := List()
	if e != nil {
		return Drive{}, e
	}
	for _, d := range ds {
		if Same(d, want) {
			if d.SectorSize != 512 {
				return d, errors.New("only 512-byte logical sectors are supported")
			}
			return d, nil
		}
	}
	return Drive{}, errors.New("the selected card was removed, changed, or is no longer safe to open")
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
