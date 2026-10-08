package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/flash"
	"github.com/andr36oid/andr36oid-sdflasher/internal/platform"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type Request struct {
	Recovery        string         `json:"recovery,omitempty"`
	Action          string         `json:"action"`
	Drive           platform.Drive `json:"drive"`
	Image           string         `json:"image,omitempty"`
	ImageHash       string         `json:"image_hash,omitempty"`
	CardFingerprint string         `json:"card_fingerprint,omitempty"`
	Mode            string         `json:"mode,omitempty"`
	Profile         string         `json:"profile,omitempty"`
	NoROMs          bool           `json:"no_roms,omitempty"`
	CustomDTB       string         `json:"custom_dtb,omitempty"`
	CustomAccepted  bool           `json:"custom_accepted,omitempty"`
}
type Result struct {
	Done     bool            `json:"done"`
	Error    string          `json:"error,omitempty"`
	Card     *flash.Card     `json:"card,omitempty"`
	Progress *flash.Progress `json:"progress,omitempty"`
	Backup   string          `json:"backup,omitempty"`
}

func CacheDir() (string, error) {
	d, e := os.UserCacheDir()
	if e != nil {
		return "", e
	}
	p := filepath.Join(d, "andr36oid-sdflasher")
	return p, os.MkdirAll(p, 0700)
}
func Worker(job string) error {
	st, e := os.Lstat(job)
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || st.Size() > 1<<20 {
		return errors.New("invalid job file")
	}
	f, e := os.Open(job)
	if e != nil {
		return e
	}
	var req Request
	e = json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&req)
	f.Close()
	if e != nil {
		return e
	}
	dir := filepath.Dir(job)
	status := filepath.Join(dir, "status.json")
	emit := func(r Result) {
		b, _ := json.Marshal(r)
		tmp := status + ".tmp"
		if os.WriteFile(tmp, b, 0644) == nil {
			os.Rename(tmp, status)
		}
	}
	var result Result
	defer func() { result.Done = true; emit(result) }()
	work := func() error {
		d, e := platform.Revalidate(req.Drive)
		if e != nil {
			return e
		}
		if req.Action == "inspect" {
			h, e := platform.Open(d, false)
			if e != nil {
				return e
			}
			defer h.Close()
			card, e := flash.InspectCard(h.File, d.Size)
			result.Card = &card
			return e
		}

		if req.Action == "restore" {
			original, e := os.Open(filepath.Join(filepath.Dir(req.Recovery), "request.json"))
			if e != nil {
				return errors.New("original update request is missing")
			}
			var previous Request
			e = json.NewDecoder(io.LimitReader(original, 1<<20)).Decode(&previous)
			original.Close()
			if e != nil {
				return e
			}
			if !platform.Same(d, previous.Drive) || previous.Mode != "update" {
				return errors.New("this recovery backup belongs to a different card")
			}
			h, e := platform.Open(d, true)
			if e != nil {
				return e
			}
			defer h.Close()
			emit(Result{Progress: &flash.Progress{Phase: "restoring", Name: "Verifying and restoring the recovery backup"}})
			return flash.RestoreBackup(context.Background(), h.File, req.Recovery, d.Size)
		}
		if req.Action != "write" {
			return errors.New("unknown job action")
		}
		if req.CustomDTB != "" && !req.CustomAccepted {
			return errors.New("custom DTB warning has not been accepted")
		}
		im, e := firmware.Inspect(context.Background(), req.Image, func(n, total int64) {
			emit(Result{Progress: &flash.Progress{Phase: "preflight", Name: "Validating release", Done: n, Total: total}})
		})
		if e != nil {
			return e
		}
		if im.SHA256 != req.ImageHash {
			return errors.New("image changed after selection")
		}
		h, e := platform.Open(d, true)
		if e != nil {
			return e
		}
		defer h.Close()
		card, e := flash.InspectCard(h.File, d.Size)
		if e != nil {
			return e
		}
		if card.Fingerprint != req.CardFingerprint {
			return errors.New("card changed after inspection; select it again")
		}
		p, e := flash.Build(im, card, d.Size, req.Mode, req.Profile, req.NoROMs)
		if e != nil {
			return e
		}
		boot, e := firmware.PrepareBoot(im, req.Profile, p.NoROMs, req.Mode == "update", req.CustomDTB, dir)
		if e != nil {
			return e
		}
		defer os.Remove(boot)
		backup := filepath.Join(dir, "recovery")
		if req.Mode == "update" {
			result.Backup = backup
		}
		return flash.Execute(context.Background(), h.File, im, p, boot, backup, func(p flash.Progress) { emit(Result{Progress: &p, Backup: result.Backup}) })
	}
	if e := work(); e != nil {
		result.Error = e.Error()
	}
	return nil
}
func helper(dir string) (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	name := "andr36oid-sdflasher-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	src := filepath.Join(filepath.Dir(exe), name)
	if _, e = os.Stat(src); e != nil {
		return "", fmt.Errorf("disk helper missing: keep both application files together: %w", e)
	}
	if os.Getenv("FLATPAK_ID") == "" && os.Getenv("APPIMAGE") == "" {
		return src, nil
	}
	in, e := os.Open(src)
	if e != nil {
		return "", e
	}
	defer in.Close()
	dst := filepath.Join(dir, name)
	out, e := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if e != nil {
		return "", e
	}
	_, e = io.Copy(out, in)
	out.Close()
	return dst, e
}
func Run(req Request, onProgress func(flash.Progress)) (Result, error) {
	cache, e := CacheDir()
	if e != nil {
		return Result{}, e
	}
	dir, e := os.MkdirTemp(cache, "operation-")
	if e != nil {
		return Result{}, e
	}
	// Keep the recovery directory owned by the desktop user so its manifest
	// remains selectable after the privileged helper has saved the backup.
	if req.Action == "write" && req.Mode == "update" {
		if e = os.Mkdir(filepath.Join(dir, "recovery"), 0700); e != nil {
			return Result{}, e
		}
	}
	exe, e := helper(dir)
	if e != nil {
		return Result{}, e
	}
	b, e := json.Marshal(req)
	if e != nil {
		return Result{}, e
	}
	job := filepath.Join(dir, "request.json")
	if e = os.WriteFile(job, b, 0600); e != nil {
		return Result{}, e
	}
	ch := make(chan error, 1)
	go func() { ch <- platform.RunHelper(exe, job) }()
	timer := time.NewTicker(150 * time.Millisecond)
	defer timer.Stop()
	var latest Result
	poll := func() {
		b, e := os.ReadFile(filepath.Join(dir, "status.json"))
		if e == nil {
			var r Result
			if json.Unmarshal(b, &r) == nil {
				latest = r
				if r.Progress != nil && onProgress != nil {
					onProgress(*r.Progress)
				}
			}
		}
	}
	for {
		select {
		case <-timer.C:
			poll()
		case e := <-ch:
			poll()
			if e != nil {
				return latest, fmt.Errorf("administrator access was denied or the helper stopped: %w", e)
			}
			if !latest.Done {
				return latest, fmt.Errorf("disk helper ended without a result; recovery files, if present, are in %s", dir)
			}
			if latest.Error != "" {
				return latest, errors.New(latest.Error)
			}
			return latest, nil
		}
	}
}
