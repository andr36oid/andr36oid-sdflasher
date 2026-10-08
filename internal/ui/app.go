package ui

import (
	"context"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/andr36oid/andr36oid-sdflasher/internal/catalog"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/flash"
	"github.com/andr36oid/andr36oid-sdflasher/internal/jobs"
	"github.com/andr36oid/andr36oid-sdflasher/internal/platform"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var Version = "development"

type screen struct {
	win                                 fyne.Window
	home                                fyne.CanvasObject
	im                                  *firmware.Image
	drive                               *platform.Drive
	card                                *flash.Card
	profile                             string
	custom                              string
	customAccepted                      bool
	busy                                bool
	releaseLabel, cardLabel, help       *widget.Label
	console, profileSelect, driveSelect *widget.Select
	mode                                *widget.RadioGroup
	noROMs                              *widget.Check
	action, local, online, refresh      *widget.Button
	customLabel                         *widget.Label
	progress                            *widget.ProgressBar
	cancel                              context.CancelFunc
}

func Run() {
	a := app.NewWithID("io.github.andr36oid.sdflasher")
	w := a.NewWindow("andr36oid SD Flasher")
	s := &screen{win: w}
	w.Resize(fyne.NewSize(820, 760))
	w.SetCloseIntercept(func() {
		if s.busy {
			dialog.ShowInformation("Operation in progress", "Wait for the operation to finish before closing. Keep the card connected.", w)
			return
		}
		w.Close()
	})
	s.build()
	w.ShowAndRun()
}
func wrapped(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}
func (s *screen) build() {
	s.releaseLabel = wrapped("Choose a release to see its supported consoles and panels.")
	s.cardLabel = wrapped("Insert a microSD card, then select it below.")
	s.help = wrapped("For a new installation, everything on the selected card will be erased. An update preserves apps, settings, saves, and the games partition.")
	s.local = widget.NewButton("Choose image from disk…", s.chooseImage)
	s.online = widget.NewButton("Download a release…", s.releases)
	s.refresh = widget.NewButton("Refresh cards", s.listDrives)
	s.console = widget.NewSelect(nil, func(name string) {
		s.profile = ""
		var opts []string
		if s.im != nil {
			for _, p := range s.im.Profiles {
				if p.Console == name {
					label := p.Name
					if p.Experimental {
						label += " (experimental)"
					}
					opts = append(opts, label)
				}
			}
		}
		s.profileSelect.SetOptions(opts)
		s.profileSelect.ClearSelected()
		if len(opts) == 1 {
			s.profileSelect.SetSelected(opts[0])
		}
		s.ready()
	})
	s.console.PlaceHolder = "Choose your console / board family"
	s.profileSelect = widget.NewSelect(nil, func(label string) {
		if s.im != nil {
			for _, p := range s.im.Profiles {
				name := p.Name
				if p.Experimental {
					name += " (experimental)"
				}
				if name == label {
					s.profile = p.ID
				}
			}
		}
		s.ready()
	})
	s.profileSelect.PlaceHolder = "Choose your board and screen"
	s.driveSelect = widget.NewSelect(nil, nil)
	s.driveSelect.PlaceHolder = "Select a microSD card"
	s.mode = widget.NewRadioGroup([]string{"Install a new card (erase)", "Update an existing installation"}, func(v string) {
		if strings.HasPrefix(v, "Update") {
			s.noROMs.Disable()
			if s.card != nil {
				s.noROMs.SetChecked(s.card.NoROMs)
			}
		} else {
			s.noROMs.Enable()
		}
		s.ready()
	})
	s.noROMs = widget.NewCheck("Use all available storage for Android", nil)
	s.mode.SetSelected("Install a new card (erase)")
	// Set the callback after the storage control exists.
	s.mode.OnChanged = func(v string) {
		if strings.HasPrefix(v, "Update") {
			s.noROMs.Disable()
			if s.card != nil {
				s.noROMs.SetChecked(s.card.NoROMs)
			}
		} else {
			s.noROMs.Enable()
		}
		s.ready()
	}
	s.customLabel = wrapped("Use the DTB shipped with the selected profile.")
	custom := widget.NewButton("Choose custom Android DTB…", s.chooseCustom)
	clear := widget.NewButton("Use shipped DTB", func() {
		s.custom = ""
		s.customAccepted = false
		s.customLabel.SetText("Use the DTB shipped with the selected profile.")
	})
	advanced := widget.NewAccordion(widget.NewAccordionItem("Advanced: custom DTB", container.NewVBox(wrapped(firmware.CustomWarning), s.customLabel, container.NewHBox(custom, clear))))
	s.action = widget.NewButton("Continue", s.confirm)
	s.action.Importance = widget.HighImportance
	s.action.Disable()
	s.progress = widget.NewProgressBar()
	s.progress.Hide()
	body := container.NewVBox(widget.NewLabelWithStyle("Set up or update your andr36oid card", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), wrapped("Choose your release and console. We’ll prepare the card and check the result."), widget.NewSeparator(), widget.NewLabelWithStyle("1. Release", 0, fyne.TextStyle{Bold: true}), container.NewHBox(s.online, s.local), s.releaseLabel, s.progress, widget.NewLabelWithStyle("2. Console and screen", 0, fyne.TextStyle{Bold: true}), s.console, s.profileSelect, widget.NewButton("I don’t know my board or panel", func() {
		dialog.ShowInformation("Finding your board and panel", "An existing andr36oid card may identify its selected profile. For a new card, use your console’s board markings and the panel information from its supplier. Similar-looking R36S consoles can have different wiring. Choose the board family first; a panel number alone is not enough. If your console is missing here, this release does not ship its profile.", s.win)
	}), advanced, widget.NewLabelWithStyle("3. microSD card", 0, fyne.TextStyle{Bold: true}), container.NewBorder(nil, nil, nil, s.refresh, s.driveSelect), s.cardLabel, s.mode, s.noROMs, wrapped("Unchecked: reserve 16 GiB for Android and use the remaining space for a games partition readable on your computer. Updates keep the existing arrangement."))
	s.home = container.NewBorder(nil, container.NewVBox(s.help, s.action, container.NewHBox(widget.NewButton("Restore an interrupted update…", s.recover), widget.NewLabel("GPLv3 · "+Version))), nil, nil, container.NewVScroll(body))
	s.win.SetContent(s.home)
	s.listDrives()
}
func (s *screen) ready() {
	if s.action == nil {
		return
	}
	if !s.busy && s.im != nil && s.drive != nil && s.card != nil && s.profile != "" {
		if s.mode.Selected == "Update an existing installation" && !s.card.Installed {
			s.action.Disable()
			return
		}
		s.action.Enable()
	} else {
		s.action.Disable()
	}
}
func (s *screen) setBusy(v bool) {
	s.busy = v
	for _, b := range []*widget.Button{s.local, s.online, s.refresh} {
		if v {
			b.Disable()
		} else {
			b.Enable()
		}
	}
	for _, x := range []*widget.Select{s.console, s.profileSelect, s.driveSelect} {
		if v {
			x.Disable()
		} else {
			x.Enable()
		}
	}
	if v {
		s.mode.Disable()
		s.noROMs.Disable()
	} else {
		s.mode.Enable()
		if s.mode.Selected != "Update an existing installation" {
			s.noROMs.Enable()
		}
	}
	s.ready()
}
func (s *screen) chooseImage() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, e error) {
		if e != nil {
			dialog.ShowError(e, s.win)
			return
		}
		if r == nil {
			return
		}
		p := r.URI().Path()
		r.Close()
		s.loadImage(p)
	}, s.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".img", ".zip"}))
	d.Show()
}
func (s *screen) loadImage(p string) {
	s.setBusy(true)
	s.im = nil
	s.profile = ""
	s.releaseLabel.SetText("Checking the release image…")
	s.progress.SetValue(0)
	s.progress.Show()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		dir, e := jobs.CacheDir()
		var im *firmware.Image
		var staged string
		if e == nil {
			staged, e = firmware.Stage(ctx, p, dir, s.progressFn())
		}
		if e == nil {
			im, e = firmware.Inspect(ctx, staged, s.progressFn())
		}
		if e != nil && staged != "" && staged != p {
			os.Remove(staged)
		}
		fyne.Do(func() {
			s.progress.Hide()
			s.setBusy(false)
			if e != nil {
				s.releaseLabel.SetText("Image rejected. Choose an andr36oid Treble release.")
				dialog.ShowError(e, s.win)
				return
			}
			s.im = im
			s.releaseLabel.SetText(im.Version + " · " + filepath.Base(p) + fmt.Sprintf(" · %d device profiles", len(im.Profiles)))
			families := map[string]bool{}
			var opts []string
			for _, p := range im.Profiles {
				if !families[p.Console] {
					families[p.Console] = true
					opts = append(opts, p.Console)
				}
			}
			sort.Strings(opts)
			s.console.SetOptions(opts)
			s.console.ClearSelected()
			s.autoProfile()
			s.ready()
		})
	}()
}
func (s *screen) progressFn() func(int64, int64) {
	return func(n, total int64) {
		fyne.Do(func() {
			if total > 0 {
				s.progress.SetValue(float64(n) / float64(total))
			}
		})
	}
}
func (s *screen) autoProfile() {
	if s.im == nil || s.card == nil {
		return
	}
	for _, p := range s.im.Profiles {
		if p.ID == s.card.Profile {
			s.console.SetSelected(p.Console)
			label := p.Name
			if p.Experimental {
				label += " (experimental)"
			}
			s.profileSelect.SetSelected(label)
			return
		}
	}
}
func (s *screen) listDrives() {
	s.refresh.Disable()
	go func() {
		ds, e := platform.List()
		fyne.Do(func() {
			s.refresh.Enable()
			if e != nil {
				s.cardLabel.SetText("Could not list cards: " + e.Error())
				return
			}
			var opts []string
			for _, d := range ds {
				opts = append(opts, d.Label())
			}
			s.driveSelect.OnChanged = nil
			s.driveSelect.SetOptions(opts)
			s.driveSelect.ClearSelected()
			s.drive = nil
			s.card = nil
			s.ready()
			s.driveSelect.OnChanged = func(label string) {
				for _, d := range ds {
					if d.Label() == label {
						s.inspectDrive(d)
						break
					}
				}
			}
			if len(ds) == 0 {
				s.cardLabel.SetText("No removable cards found. Insert a card and refresh.")
			}
		})
	}()
}
func (s *screen) inspectDrive(d platform.Drive) {
	s.setBusy(true)
	s.drive = &d
	s.card = nil
	s.cardLabel.SetText("Inspecting the card. Your computer may request administrator access.")
	go func() {
		result, e := jobs.Run(jobs.Request{Action: "inspect", Drive: d}, nil)
		fyne.Do(func() {
			s.setBusy(false)
			if e != nil {
				s.cardLabel.SetText("Card inspection failed.")
				dialog.ShowError(e, s.win)
				return
			}
			s.card = result.Card
			if s.card == nil {
				return
			}
			if s.card.Installed {
				kind := "Legacy andr36oid — will migrate to Treble"
				if s.card.Treble {
					kind = "Treble installation"
				}
				s.cardLabel.SetText(kind + ". Apps, settings, saves, and games will be preserved.")
				s.mode.SetSelected("Update an existing installation")
				s.noROMs.SetChecked(s.card.NoROMs)
				s.autoProfile()
			} else {
				s.cardLabel.SetText("New installation required: " + s.card.Reason)
				s.mode.SetSelected("Install a new card (erase)")
			}
			s.ready()
		})
	}()
}
func (s *screen) chooseCustom() {
	if s.busy {
		return
	}
	dialog.ShowConfirm("Custom Android DTB", firmware.CustomWarning+"\n\nOnly the Android DTB is replaced. The selected shipped profile still supplies the bootloader display files. Continue only if this DTB was made for your console and this release.", func(ok bool) {
		if !ok {
			return
		}
		d := dialog.NewFileOpen(func(r fyne.URIReadCloser, e error) {
			if e != nil {
				dialog.ShowError(e, s.win)
				return
			}
			if r == nil {
				return
			}
			s.custom = r.URI().Path()
			r.Close()
			s.customAccepted = true
			s.customLabel.SetText(filepath.Base(s.custom) + " — custom Android DTB")
		}, s.win)
		d.SetFilter(storage.NewExtensionFileFilter([]string{".dtb"}))
		d.Show()
	}, s.win)
}
func (s *screen) releases() {
	s.setBusy(true)
	s.releaseLabel.SetText("Checking GitHub releases…")
	go func() {
		assets, e := catalog.List(context.Background())
		fyne.Do(func() {
			s.setBusy(false)
			if e != nil {
				dialog.ShowError(e, s.win)
				return
			}
			if len(assets) == 0 {
				dialog.ShowInformation("No image downloads available", "No disk images are attached to andr36oid/releases yet. You can choose an image from disk.", s.win)
				return
			}
			var opts []string
			for _, a := range assets {
				opts = append(opts, a.Release+" · "+a.Name)
			}
			pick := widget.NewSelect(opts, nil)
			pick.PlaceHolder = "Choose a release image"
			dialog.ShowCustomConfirm("Download a release", "Download", "Cancel", container.NewVBox(wrapped("Only Treble images can be installed. Each download is checked before the card can be written."), pick), func(ok bool) {
				if !ok {
					return
				}
				i := pick.SelectedIndex()
				if i < 0 {
					return
				}
				s.download(assets[i])
			}, s.win)
		})
	}()
}
func (s *screen) download(a catalog.Asset) {
	s.setBusy(true)
	s.progress.Show()
	s.progress.SetValue(0)
	s.releaseLabel.SetText("Downloading " + a.Name + "…")
	go func() {
		dir, e := jobs.CacheDir()
		var p string
		if e == nil {
			p, e = catalog.Download(context.Background(), a, dir, s.progressFn())
		}
		fyne.Do(func() {
			s.progress.Hide()
			s.setBusy(false)
			if e != nil {
				dialog.ShowError(e, s.win)
				return
			}
			s.loadImage(p)
		})
	}()
}
func (s *screen) confirm() {
	if s.im == nil || s.card == nil || s.drive == nil {
		return
	}
	mode := "install"
	if s.mode.Selected == "Update an existing installation" {
		mode = "update"
	}
	p, e := flash.Build(s.im, *s.card, s.drive.Size, mode, s.profile, s.noROMs.Checked)
	if e != nil {
		dialog.ShowError(e, s.win)
		return
	}
	msg := "Erase and install on " + s.drive.Label() + "?\n\nEverything currently on this card will be erased."
	title := "Erase this card?"
	if mode == "update" {
		title = "Update this card?"
		msg = "Update " + s.drive.Label() + "?\n\nBOOT, system, and vendor will be updated together. The dedicated cache will be cleared. Userdata and games stay in place. A recovery backup will be saved before writing."
	}
	if s.custom != "" {
		msg += "\n\n" + firmware.CustomWarning
	}
	dialog.ShowConfirm(title, msg, func(ok bool) {
		if ok {
			s.write(p)
		}
	}, s.win)
}
func (s *screen) write(p *flash.Plan) {
	s.setBusy(true)
	m := newDiskMap()
	m.plan(p)
	phase := wrapped("Preparing to write. Keep the card connected.")
	bar := widget.NewProgressBar()
	details := wrapped("Each block summarizes a region of the card. Preserved regions are never written.")
	back := widget.NewButton("Back to setup", func() {
		s.win.SetContent(s.home)
		s.card = nil
		s.drive = nil
		s.driveSelect.ClearSelected()
		s.setBusy(false)
		s.listDrives()
	})
	back.Disable()
	s.win.SetContent(container.NewPadded(container.NewVBox(widget.NewLabelWithStyle("Writing your andr36oid card", 0, fyne.TextStyle{Bold: true}), widget.NewLabel(s.drive.Label()), m.img, legend(), phase, bar, details, back)))
	req := jobs.Request{Action: "write", Drive: *s.drive, Image: s.im.Path, ImageHash: s.im.SHA256, CardFingerprint: s.card.Fingerprint, Mode: p.Mode, Profile: s.profile, NoROMs: p.NoROMs, CustomDTB: s.custom, CustomAccepted: s.customAccepted}
	go func() {
		result, e := jobs.Run(req, func(p flash.Progress) {
			fyne.Do(func() {
				m.update(p)
				phase.SetText(strings.ToUpper(p.Phase[:1]) + p.Phase[1:] + ": " + p.Name)
				if p.Total > 0 {
					bar.SetValue(float64(p.Done) / float64(p.Total))
				}
			})
		})
		fyne.Do(func() {
			s.busy = false
			back.Enable()
			if e != nil {
				phase.SetText("Operation stopped: " + e.Error())
				if result.Backup != "" {
					details.SetText("Recovery files: " + result.Backup)
				}
				return
			}
			bar.SetValue(1)
			phase.SetText("Your card has been written and verified.")
			if p.Mode == "install" {
				details.SetText("Safely eject the card and insert it into the console’s system card slot. The first boot prepares storage and restarts the console. Keep it powered on until Android setup appears. If the screen stays blank, check the chosen board and panel profile.")
			} else {
				details.SetText("Safely eject the card and start your console. Apps, settings, saves, and games were preserved. Recovery backup: " + result.Backup)
			}
		})
	}()
}

func (s *screen) recover() {
	if s.busy {
		return
	}
	if s.drive == nil {
		dialog.ShowInformation("Choose a card", "Select the card that was being updated first.", s.win)
		return
	}
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, e error) {
		if e != nil {
			dialog.ShowError(e, s.win)
			return
		}
		if r == nil {
			return
		}
		file := r.URI().Path()
		r.Close()
		if filepath.Base(file) != "recovery.json" {
			dialog.ShowError(fmt.Errorf("choose the recovery.json file saved by the interrupted update"), s.win)
			return
		}
		drive := *s.drive
		dialog.ShowConfirm("Restore previous installation", "Restore the saved operating system on "+drive.Label()+"? Userdata and games remain protected.", func(ok bool) {
			if !ok {
				return
			}
			s.setBusy(true)
			s.cardLabel.SetText("Verifying and restoring the recovery backup…")
			go func() {
				_, e := jobs.Run(jobs.Request{Action: "restore", Drive: drive, Recovery: filepath.Dir(file)}, nil)
				fyne.Do(func() {
					s.setBusy(false)
					if e != nil {
						dialog.ShowError(e, s.win)
					} else {
						dialog.ShowInformation("Restored", "The previous installation was restored and verified.", s.win)
					}
					s.listDrives()
				})
			}()
		}, s.win)
	}, s.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	d.Show()
}
