package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/jobs"
	"os"
)

func main() {
	var err error
	if len(os.Args) == 3 {
		switch os.Args[1] {
		case "inspect":
			var im *firmware.Image
			im, err = firmware.Inspect(context.Background(), os.Args[2], nil)
			if err == nil {
				json.NewEncoder(os.Stdout).Encode(im)
			}
		case "job":
			err = jobs.Worker(os.Args[2])
		default:
			err = fmt.Errorf("unknown command")
		}
	} else {
		err = fmt.Errorf("usage: andr36oid-sdflasher-helper inspect IMAGE | job REQUEST")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
