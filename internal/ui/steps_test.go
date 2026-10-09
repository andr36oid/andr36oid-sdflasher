package ui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/flash"
	"github.com/andr36oid/andr36oid-sdflasher/internal/i18n"
	"github.com/andr36oid/andr36oid-sdflasher/internal/platform"
)

// Every step page renders in every language. With SDFLASHER_SCREENSHOTS set,
// each page is saved as <language>-<step>.png for a visual check.
func TestEveryStepRenders(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(uiTheme())
	w := a.NewWindow("test")
	defer w.Close()
	w.Resize(fyne.NewSize(820, 760))
	drive := platform.Drive{Path: "/dev/sdb", Size: 64 << 30, Name: "Generic SD reader"}
	s := &screen{win: w, locale: "en", im: &firmware.Image{Version: "lineage-18.1-20261009", Profiles: []firmware.Profile{{ID: "Panels/Panel4", Name: "R36S — Panel 4", Console: "R36S"}, {ID: "Experimental/EE-G80-Panel8", Name: "G80C / G80CA / G80D — Panel 8", Console: "G80", Experimental: true}}}, profile: "Experimental/EE-G80-Panel8", drive: &drive, drives: []platform.Drive{drive}, card: &flash.Card{Reason: "no andr36oid partitions"}, custom: "/home/u/rk3326-r36s-linux.dtb", customAccepted: true, convert: true}
	dir := os.Getenv("SDFLASHER_SCREENSHOTS")
	for _, l := range i18n.Languages {
		s.locale = l.Code
		for step := 0; step < stepCount; step++ {
			s.step = step
			s.build()
			for i, p := range s.pages {
				if p.Visible() != (i == step) {
					t.Fatalf("%s step %d: page %d visible=%v", l.Code, step, i, p.Visible())
				}
			}
			if s.action.Disabled() {
				t.Fatalf("%s: write button disabled with everything chosen", l.Code)
			}
			if dir != "" {
				os.MkdirAll(dir, 0o755)
				f, e := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%d.png", l.Code, step)))
				if e != nil {
					t.Fatal(e)
				}
				png.Encode(f, w.Canvas().Capture())
				f.Close()
			}
		}
	}
}

func TestStepsNeedWhatComesBefore(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	w := a.NewWindow("test")
	defer w.Close()
	s := &screen{win: w, locale: "en"}
	s.build()
	if !s.next.Disabled() {
		t.Fatal("next enabled without a release")
	}
	for i := stepCard; i < stepCount; i++ {
		if !s.stepButtons[i].Disabled() {
			t.Fatalf("step %d reachable without a release", i)
		}
	}
	s.showStep(stepWrite)
	s.im = &firmware.Image{Profiles: []firmware.Profile{{ID: "Panels/Panel4", Name: "R36S — Panel 4", Console: "R36S"}}}
	s.populateImage()
	s.autoProfile()
	s.ready()
	if s.next.Disabled() || !s.stepButtons[stepConsole].Disabled() {
		t.Fatal("release chosen: only the card step should open up")
	}
	s.drive = &platform.Drive{Path: "x", Size: 8 << 30}
	s.card = &flash.Card{Installed: false}
	s.mode.SetSelected(s.t("Update an existing installation"))
	if s.action.Disabled() == false || !s.stepButtons[stepWrite].Disabled() {
		t.Fatal("update of a card without andr36oid must not be writable")
	}
	s.mode.SetSelected(s.t("Install a new card (erase)"))
	if s.action.Disabled() || s.stepButtons[stepWrite].Disabled() {
		t.Fatal("new install with everything chosen must be writable")
	}
}
