//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"log"
	"os"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// Run from the repository root after changing the shared SVG artwork.
func main() {
	source, err := os.ReadFile("packaging/io.github.andr36oid.sdflasher.svg")
	if err != nil {
		log.Fatal(err)
	}
	var contents bytes.Buffer
	for _, entry := range []struct {
		kind string
		size int
	}{
		{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128},
		{"ic08", 256}, {"ic09", 512}, {"ic10", 1024},
		{"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512},
	} {
		icon, err := oksvg.ReadIconStream(bytes.NewReader(source), oksvg.StrictErrorMode)
		if err != nil {
			log.Fatal(err)
		}
		img := image.NewNRGBA(image.Rect(0, 0, entry.size, entry.size))
		icon.SetTarget(0, 0, float64(entry.size), float64(entry.size))
		icon.Draw(rasterx.NewDasher(entry.size, entry.size, rasterx.NewScannerGV(entry.size, entry.size, img, img.Bounds())), 1)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			log.Fatal(err)
		}
		contents.WriteString(entry.kind)
		binary.Write(&contents, binary.BigEndian, uint32(encoded.Len()+8))
		contents.Write(encoded.Bytes())
		if entry.kind == "ic09" {
			if err := os.WriteFile("internal/ui/icon.png", encoded.Bytes(), 0644); err != nil {
				log.Fatal(err)
			}
		}
	}
	var icns bytes.Buffer
	icns.WriteString("icns")
	binary.Write(&icns, binary.BigEndian, uint32(contents.Len()+8))
	icns.Write(contents.Bytes())
	if err := os.WriteFile("packaging/andr36oid-sdflasher.icns", icns.Bytes(), 0644); err != nil {
		log.Fatal(err)
	}
}
