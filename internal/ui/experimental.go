package ui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
)

// where problems with experimental device trees go, and only there
const cloneTracker = "https://github.com/andr36oid/clone_issue_tracker"

// confirmExperimental stands between choosing an experimental profile and using it:
// a direct warning, a checkbox and an "I agree" button that only works once the box
// is ticked. Asked once per profile while the app runs. done gets whether the
// user agreed.
func (s *screen) confirmExperimental(p firmware.Profile, done func(bool)) {
	if s.experimentalOK[p.ID] {
		done(true)
		return
	}
	name := widget.NewLabelWithStyle(p.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	body := wrapped(s.t("Nobody on the andr36oid team has tested this device tree on real hardware. It can leave the screen black or the buttons dead, and in the worst case put a wrong voltage on part of the board.\n\nSupport for experimental device trees is very limited. If something doesn't work:\n• Report it only in the clone issue tracker: %s\n• Attach all the boot files from a system that works on your console, for example from the SD card it came with: every .dtb file and the other files at the top of its first drive.\n• Answer every question in the issue form.\n\nReports without those files will be closed.", cloneTracker))
	body.Importance = widget.DangerImportance
	link, _ := url.Parse(cloneTracker)
	check := widget.NewCheck(s.t("I understand that this may not work, that support is very limited, and that I must attach my console's working boot files to any report."), nil)
	agree := widget.NewButton(s.t("I agree"), nil)
	agree.Importance = widget.HighImportance
	agree.Disable()
	cancel := widget.NewButton(s.t("Cancel"), nil)
	content := container.NewVScroll(container.NewVBox(name, body, widget.NewHyperlink(cloneTracker, link), widget.NewSeparator(), check))
	d := dialog.NewCustomWithoutButtons(s.t("Experimental device tree"), content, s.win)
	check.OnChanged = func(v bool) {
		if v {
			agree.Enable()
		} else {
			agree.Disable()
		}
	}
	agree.OnTapped = func() {
		if s.experimentalOK == nil {
			s.experimentalOK = map[string]bool{}
		}
		s.experimentalOK[p.ID] = true
		d.Hide()
		done(true)
	}
	cancel.OnTapped = func() {
		d.Hide()
		done(false)
	}
	d.SetButtons([]fyne.CanvasObject{cancel, agree})
	d.Resize(fyne.NewSize(640, 600))
	d.Show()
}
