package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// Eject unmounts the card's volumes and powers the reader's card down
// (udisksctl, as the desktop's own eject does), or runs eject without udisks.
func Eject(d Drive) error {
	d, e := Revalidate(d)
	if e != nil {
		return e
	}
	if _, e := exec.LookPath("udisksctl"); e != nil {
		if b, e := exec.Command("eject", d.Path).CombinedOutput(); e != nil {
			return fmt.Errorf("eject %s: %s", d.Path, strings.TrimSpace(string(b)))
		}
		return nil
	}
	for _, v := range d.Volumes {
		if b, e := exec.Command("udisksctl", "unmount", "--no-user-interaction", "-b", v).CombinedOutput(); e != nil {
			return fmt.Errorf("cannot unmount %s: %s", v, strings.TrimSpace(string(b)))
		}
	}
	if b, e := exec.Command("udisksctl", "power-off", "--no-user-interaction", "-b", d.Path).CombinedOutput(); e != nil {
		return fmt.Errorf("cannot eject %s: %s", d.Path, strings.TrimSpace(string(b)))
	}
	return nil
}
