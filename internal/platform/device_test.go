package platform

import (
	"strings"
	"testing"
)

func TestSame(t *testing.T) {
	card := Drive{Path: `\\.\PhysicalDrive2`, Name: "Generic STORAGE DEVICE", Serial: "000000000820", Size: 31914983424, SectorSize: 512}
	noSerial, noName := card, card
	noSerial.Serial = ""
	noName.Name = ""
	for _, d := range []Drive{card, noSerial, noName} {
		if !Same(d, card) || !Same(card, d) {
			t.Fatal("same card not matched", d)
		}
	}
	other := card
	other.Path = `\\.\PhysicalDrive3`
	bigger := card
	bigger.Size *= 2
	swapped := card
	swapped.Serial = "000000000821"
	renamed := card
	renamed.Name = "Other Reader"
	for _, d := range []Drive{other, bigger, swapped, renamed} {
		if Same(d, card) {
			t.Fatal("different card matched", d)
		}
	}
	if why := difference(swapped, card); !strings.Contains(why, "000000000821") || !strings.Contains(why, "000000000820") {
		t.Fatal("difference doesn't show both serials:", why)
	}
}
