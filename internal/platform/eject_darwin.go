package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// Eject unmounts the card's volumes and ejects it, as Finder does.
func Eject(d Drive) error {
	d, e := Revalidate(d)
	if e != nil {
		return e
	}
	if b, e := exec.Command("/usr/sbin/diskutil", "eject", d.Path).CombinedOutput(); e != nil {
		return fmt.Errorf("cannot eject %s: %s", d.Path, strings.TrimSpace(string(b)))
	}
	return nil
}
