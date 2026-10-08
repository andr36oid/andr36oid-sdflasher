package ui

import (
	"github.com/andr36oid/andr36oid-sdflasher/internal/flash"
	"testing"
)

func TestMapNeverPaintsPreservedRegionsAsWritten(t *testing.T) {
	m := newDiskMap()
	p := &flash.Plan{Size: 100000, Protected: []flash.Range{{Name: "userdata", Offset: 50000, Length: 50000}}}
	m.plan(p)
	m.update(flash.Progress{Phase: "writing", Offset: 0, Length: 100000})
	m.update(flash.Progress{Phase: "verifying", Offset: 0, Length: 100000})
	for i := len(m.state) / 2; i < len(m.state); i++ {
		if m.state[i] != 2 {
			t.Fatal("preserved region displayed as written")
		}
	}
}
