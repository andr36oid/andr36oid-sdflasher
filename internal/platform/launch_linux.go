package platform

import (
	"os"
	"os/exec"
)

func hostCommand(name string, args ...string) *exec.Cmd {
	if os.Getenv("FLATPAK_ID") != "" {
		return exec.Command("flatpak-spawn", append([]string{"--host", name}, args...)...)
	}
	return exec.Command(name, args...)
}
func RunHelper(exe, job string) error {
	if os.Geteuid() == 0 {
		return exec.Command(exe, "job", job).Run()
	}
	return hostCommand("pkexec", exe, "job", job).Run()
}
