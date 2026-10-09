package ui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed icon.png
var iconData []byte

var appIcon = fyne.NewStaticResource("andr36oid-sdflasher.png", iconData)
