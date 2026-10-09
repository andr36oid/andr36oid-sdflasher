package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/andr36oid/andr36oid-sdflasher/internal/dtbconv"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/jobs"
	"os"
	"path/filepath"
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
	} else if len(os.Args) == 4 && os.Args[1] == "convert-dtb" {
		err = convertDTB(os.Args[2], os.Args[3])
	} else {
		err = fmt.Errorf("usage: andr36oid-sdflasher-helper inspect IMAGE | job REQUEST | convert-dtb STOCK.dtb OUTDIR")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// convertDTB turns an ArkOS/dArkOS or stock EmuELEC DTB into andr36oid BOOT files.
func convertDTB(in, dir string) error {
	b, e := os.ReadFile(in)
	if e != nil {
		return e
	}
	r, e := dtbconv.Convert(b, dtbconv.EmbeddedBases())
	if e != nil {
		return e
	}
	if e = os.MkdirAll(dir, 0o755); e != nil {
		return e
	}
	files := map[string][]byte{"rk3326-r36s-android.dtb": r.Kernel, "conversion-report.txt": []byte(r.Report())}
	for _, n := range []string{"rg351mp-kernel.dtb", "rg351v-kernel.dtb", "rg351p-kernel.dtb"} {
		files[n] = r.UBoot
	}
	for n, data := range files {
		if e = os.WriteFile(filepath.Join(dir, n), data, 0o644); e != nil {
			return e
		}
	}
	fmt.Print(r.Report())
	return nil
}
