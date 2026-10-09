package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/flash"
)

func TestAutomaticProfileSelection(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	for _, tc := range []struct {
		name, selected, detected, want string
		panel4                         bool
	}{
		{"new card", "", "", "Panels/Panel4", true},
		{"manual choice", "Panels/Panel3", "", "Panels/Panel3", true},
		{"installed panel", "Panels/Panel4", "Panels/Panel3", "Panels/Panel3", true},
		{"unsupported installed panel", "Panels/Panel4", "Devices/Other", "", true},
		{"image without the default", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := a.NewWindow("test")
			defer w.Close()
			profiles := []firmware.Profile{{ID: "Panels/Panel3", Name: "R36S — Panel 3", Console: "R36S"}}
			if tc.panel4 {
				profiles = append(profiles, firmware.Profile{ID: "Panels/Panel4", Name: "R36S — Panel 4", Console: "R36S"})
			}
			s := &screen{win: w, locale: "en", profile: tc.selected, im: &firmware.Image{Profiles: profiles}, card: &flash.Card{Profile: tc.detected}}
			s.build()
			s.autoProfile()
			if s.profile != tc.want {
				t.Fatalf("selected %q, want %q", s.profile, tc.want)
			}
			if tc.want == "" && s.profileSelect.Selected != "" {
				t.Fatal("unsupported profile still shown as selected")
			}
		})
	}
}
