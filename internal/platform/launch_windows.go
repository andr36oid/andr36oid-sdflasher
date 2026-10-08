package platform

import (
	"os/exec"
	"strings"
)

func RunHelper(exe, job string) error {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	args := "job " + `"` + job + `"`
	c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$p=Start-Process -FilePath "+quote(exe)+" -ArgumentList "+quote(args)+" -Verb RunAs -PassThru -Wait; exit $p.ExitCode")
	hide(c)
	return c.Run()
}
