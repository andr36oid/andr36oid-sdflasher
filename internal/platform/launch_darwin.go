package platform

import (
	"os/exec"
	"strconv"
	"strings"
)

func RunHelper(exe, job string) error {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	cmd := quote(exe) + " job " + quote(job)
	return exec.Command("/usr/bin/osascript", "-e", "do shell script "+strconv.Quote(cmd)+" with administrator privileges").Run()
}
