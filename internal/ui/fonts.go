package ui

import (
	"embed"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed fonts/*.ttf
var fontFiles embed.FS

func uiTheme() fyne.Theme {
	normal, _ := fontFiles.ReadFile("fonts/SDFlasherUI-Regular.ttf")
	bold, _ := fontFiles.ReadFile("fonts/SDFlasherUI-Bold.ttf")
	return localizedTheme{Theme: theme.DefaultTheme(), normal: fyne.NewStaticResource("SDFlasherUI-Regular.ttf", normal), bold: fyne.NewStaticResource("SDFlasherUI-Bold.ttf", bold)}
}

type localizedTheme struct {
	fyne.Theme
	normal, bold fyne.Resource
}

func (t localizedTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace {
		return t.Theme.Font(style)
	}
	if style.Bold {
		return t.bold
	}
	return t.normal
}
