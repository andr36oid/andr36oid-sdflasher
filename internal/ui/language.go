package ui

import (
	"embed"
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
	"github.com/andr36oid/andr36oid-sdflasher/internal/i18n"
)

//go:embed flags/*.svg
var flags embed.FS

func preferredLocale(a fyne.App) string {
	saved := a.Preferences().String("language")
	if saved == "" {
		saved = string(lang.SystemLocale())
	}
	return i18n.Resolve(saved)
}
func (s *screen) languageName() string {
	for _, l := range i18n.Languages {
		if l.Code == s.locale {
			return l.Name
		}
	}
	return "English"
}
func (s *screen) chooseLanguage() {
	if s.busy {
		return
	}
	change := func(code string) {
		fyne.CurrentApp().Preferences().SetString("language", code)
		s.locale = preferredLocale(fyne.CurrentApp())
		s.localizeDialogs()
		if s.started {
			s.build()
		} else {
			s.showCardAdvice()
		}
	}
	automatic := fyne.NewMenuItem(s.t("System language"), func() { change("") })
	automatic.Checked = fyne.CurrentApp().Preferences().String("language") == ""
	items := []*fyne.MenuItem{automatic, fyne.NewMenuItemSeparator()}
	for _, l := range i18n.Languages {
		code := l.Code
		item := fyne.NewMenuItem(l.Name, func() { change(code) })
		data, _ := flags.ReadFile("flags/" + code + ".svg")
		item.Icon = fyne.NewStaticResource(code+".svg", data)
		item.Checked = fyne.CurrentApp().Preferences().String("language") == code
		items = append(items, item)
	}
	menu := widget.NewPopUpMenu(fyne.NewMenu(s.t("Language"), items...), s.win.Canvas())
	menu.ShowAtPosition(fyne.CurrentApp().Driver().AbsolutePositionForObject(s.language).Add(fyne.NewPos(0, s.language.Size().Height)))
}
func (s *screen) localizeDialogs() {
	// Fyne file dialogs use the system locale. Replace their common messages
	// through its public translation API when the app language changes.
	messages := map[string]string{}
	for _, key := range []string{"Advanced", "Cancel", "Confirm", "Copy", "Create Folder", "Cut", "Enter filename", "Error", "Favourites", "File", "Folder", "New Folder", "No", "OK", "Open", "Paste", "Quit", "Redo", "Save", "Select all", "Show Hidden Files", "Undo", "Yes", "file.name", "file.parent"} {
		messages[key] = s.t(key)
	}
	data, _ := json.Marshal(messages)
	_ = lang.AddTranslationsForLocale(data, lang.SystemLocale())
}
func (s *screen) info(title, message string, parent fyne.Window) {
	dialog.ShowCustom(title, s.t("OK"), wrapped(message), parent)
}
func (s *screen) confirmDialog(title, message string, callback func(bool), parent fyne.Window) {
	dialog.ShowCustomConfirm(title, s.t("Continue"), s.t("Cancel"), wrapped(message), callback, parent)
}
func (s *screen) showError(err error) {
	s.info(s.t("Operation could not finish"), s.t("Check the selected image and card, then try again.")+"\n\n"+s.t("Technical details")+":\n"+err.Error(), s.win)
}
