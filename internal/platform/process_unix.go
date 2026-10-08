//go:build !windows

package platform

import "os/exec"

func hide(c *exec.Cmd) {}
