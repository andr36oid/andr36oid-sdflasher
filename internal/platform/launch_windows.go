package platform

import (
	"os/exec"
	"strings"
)

func RunHelper(exe, job string) error {
	// Already admin (ElevateAtStart): start the helper directly, no second prompt
	if elevated() {
		c := exec.Command(exe, "job", job)
		hide(c)
		return c.Run()
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	args := "job " + `"` + job + `"`
	c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$p=Start-Process -FilePath "+quote(exe)+" -ArgumentList "+quote(args)+" -Verb RunAs -WindowStyle Hidden -PassThru -Wait; exit $p.ExitCode")
	hide(c)
	return c.Run()
}
