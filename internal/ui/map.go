package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/andr36oid/andr36oid-sdflasher/internal/flash"
	"github.com/andr36oid/andr36oid-sdflasher/internal/i18n"
	"image"
	"image/color"
	"image/draw"
	"math"
)

const mapCols = 72
const mapRows = 28

var mapColors = []color.RGBA{{240, 240, 240, 255}, {170, 170, 170, 255}, {0, 210, 210, 255}, {0, 0, 180, 255}, {250, 20, 20, 255}, {0, 110, 240, 255}}

type diskMap struct {
	img   *canvas.Image
	state []byte
	size  int64
}

func newDiskMap() *diskMap {
	m := &diskMap{state: make([]byte, mapCols*mapRows)}
	m.img = canvas.NewImageFromImage(m.render())
	m.img.SetMinSize(fyne.NewSize(576, 224))
	m.img.FillMode = canvas.ImageFillStretch
	return m
}
func (m *diskMap) render() image.Image {
	im := image.NewRGBA(image.Rect(0, 0, mapCols*8, mapRows*9))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.RGBA{20, 20, 20, 255}), image.Point{}, draw.Src)
	for i, s := range m.state {
		x := (i % mapCols) * 8
		y := (i / mapCols) * 9
		c := mapColors[s]
		draw.Draw(im, image.Rect(x+1, y+1, x+7, y+8), image.NewUniform(c), image.Point{}, draw.Src)
		if s == 3 {
			for yy := y + 2; yy < y+8; yy += 2 {
				for xx := x + 2; xx < x+7; xx += 2 {
					im.Set(xx, yy, color.RGBA{190, 190, 255, 255})
				}
			}
		}
	}
	return im
}
func (m *diskMap) mark(off, length int64, state byte) {
	if m.size <= 0 || length <= 0 {
		return
	}
	start := int(float64(off) / float64(m.size) * float64(len(m.state)))
	end := int(math.Ceil(float64(off+length) / float64(m.size) * float64(len(m.state))))
	if start < 0 {
		start = 0
	}
	if end > len(m.state) {
		end = len(m.state)
	}
	for i := start; i < end; i++ {
		if m.state[i] != 2 {
			m.state[i] = state
		}
	}
}
func (m *diskMap) plan(p *flash.Plan) {
	m.size = p.Size
	for i := range m.state {
		m.state[i] = 0
	}
	for _, w := range p.Writes {
		m.mark(w.Offset, w.Length, 1)
	}
	for _, r := range p.Protected {
		m.mark(r.Offset, r.Length, 2)
	}
	m.refresh()
}
func (m *diskMap) update(p flash.Progress) {
	if p.Plan != nil {
		m.plan(p.Plan)
	}
	switch p.Phase {
	case "writing":
		m.mark(p.Offset, p.Length, 5)
		block := m.size / int64(len(m.state))
		if block < 1 {
			block = 1
		}
		m.mark(p.Offset+p.Length-block, block, 4)
	case "verifying":
		m.mark(p.Offset, p.Length, 3)
	}
	m.refresh()
}
func (m *diskMap) refresh() { m.img.Image = m.render(); m.img.Refresh() }
func legend(locale string) *fyne.Container {
	items := []fyne.CanvasObject{}
	for _, x := range []struct {
		state int
		name  string
	}{{1, "Pending"}, {4, "Writing"}, {5, "Written"}, {3, "Verified"}, {2, "Preserved"}} {
		r := canvas.NewRectangle(mapColors[x.state])
		items = append(items, container.NewHBox(container.NewGridWrap(fyne.NewSize(12, 12), r), widget.NewLabel(i18n.Text(locale, x.name))))
	}
	return container.NewHBox(items...)
}
