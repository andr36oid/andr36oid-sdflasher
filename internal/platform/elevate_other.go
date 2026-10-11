//go:build !windows

package platform

// ElevateAtStart only does something on Windows; elsewhere each card job asks for
// administrator rights itself.
func ElevateAtStart() bool { return false }
