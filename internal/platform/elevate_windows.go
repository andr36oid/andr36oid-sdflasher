package platform

import (
	"os"

	"golang.org/x/sys/windows"
)

// startedElevated marks the copy ElevateAtStart started, so it never tries again:
// with UAC turned off for a standard user, "runas" starts the app without admin
// rights and it would otherwise keep restarting itself.
const startedElevated = "--started-elevated"

// ElevateAtStart starts the app again as administrator, the one UAC prompt, so card
// jobs run without asking each time. It returns true when the admin copy started and
// this one should exit. If the user says no, the app carries on without admin
// rights and asks for every card job as before.
func ElevateAtStart() bool {
	for _, a := range os.Args[1:] {
		if a == startedElevated {
			return false
		}
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		return false
	}
	exe, e := os.Executable()
	if e != nil {
		return false
	}
	dir, _ := os.Getwd()
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	args, _ := windows.UTF16PtrFromString(startedElevated)
	cwd, _ := windows.UTF16PtrFromString(dir)
	return windows.ShellExecute(0, verb, file, args, cwd, windows.SW_NORMAL) == nil
}

// elevated reports whether this process already has admin rights.
func elevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
