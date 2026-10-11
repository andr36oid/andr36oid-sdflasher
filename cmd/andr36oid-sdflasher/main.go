package main

import (
	"github.com/andr36oid/andr36oid-sdflasher/internal/platform"
	"github.com/andr36oid/andr36oid-sdflasher/internal/ui"
)

func main() {
	// Windows: ask for administrator rights once, here, instead of for every card job
	if platform.ElevateAtStart() {
		return
	}
	ui.Run()
}
