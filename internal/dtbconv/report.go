package dtbconv

import (
	"fmt"
	"strings"
)

// Report is a plain-text summary of a conversion, for people.
func (r *Result) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "andr36oid DTB conversion\n\nInput model: %s\nBoard family: %s\nScreen: %dx%d\n\n", r.Model, r.Family.Label(), r.Width, r.Height)
	b.WriteString("What was done:\n")
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\nPlease check:\n")
		for _, n := range r.Warnings {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	b.WriteString("\nNever copied from the input:\n")
	for _, n := range Skipped {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	b.WriteString("\nThis DTB was made by a program and has not been tested on your console.\nKeep the files in andr36oid-dtb-backup to go back.\n")
	return b.String()
}
