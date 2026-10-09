// Package mobile exposes the shared flashing engine to the native Android app.
package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/andr36oid/andr36oid-sdflasher/internal/catalog"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"github.com/andr36oid/andr36oid-sdflasher/internal/flash"
	"github.com/andr36oid/andr36oid-sdflasher/internal/i18n"
)

type Disk interface {
	Capacity() int64
	Identity() string
	Read(offset, length int64) ([]byte, error)
	Write(offset int64, data []byte) error
	Flush() error
}
type Observer interface{ Progress(event string) }

type usbDisk struct {
	disk Disk
	size int64
}

func adapt(d Disk) (*usbDisk, error) {
	if d == nil {
		return nil, errors.New("connect a USB card reader")
	}
	size := d.Capacity()
	if size < 1<<30 || size%512 != 0 {
		return nil, errors.New("unsupported card capacity")
	}
	return &usbDisk{disk: d, size: size}, nil
}
func (d *usbDisk) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off > d.size || int64(len(p)) > d.size-off || off%512 != 0 || len(p)%512 != 0 {
		return 0, errors.New("invalid USB read range")
	}
	b, e := d.disk.Read(off, int64(len(p)))
	if e != nil {
		return 0, e
	}
	if len(b) != len(p) {
		return 0, io.ErrUnexpectedEOF
	}
	return copy(p, b), nil
}
func (d *usbDisk) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 || off > d.size || int64(len(p)) > d.size-off || off%512 != 0 || len(p)%512 != 0 {
		return 0, errors.New("invalid USB write range")
	}
	if e := d.disk.Write(off, p); e != nil {
		return 0, e
	}
	return len(p), nil
}
func (d *usbDisk) Sync() error     { return d.disk.Flush() }
func encode(v any) (string, error) { b, e := json.Marshal(v); return string(b), e }
func CheckAppUpdate(current string) (string, error) {
	update, err := catalog.CheckAppUpdate(context.Background(), current)
	if err != nil || update == nil {
		return "", err
	}
	return encode(update)
}
func emit(observer Observer, p flash.Progress) {
	if observer != nil {
		s, _ := encode(p)
		observer.Progress(s)
	}
}
func progress(observer Observer, phase string) func(int64, int64) {
	return func(n, total int64) { emit(observer, flash.Progress{Phase: phase, Done: n, Total: total}) }
}

type Session struct {
	mu    sync.Mutex
	dir   string
	image *firmware.Image
}

func NewSession(dir string) (*Session, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("an absolute application storage directory is required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	return &Session{dir: dir}, nil
}
func (s *Session) Load(path string, observer Observer) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.image = nil
	staged, e := firmware.Stage(context.Background(), path, s.dir, progress(observer, "extracting"))
	if e != nil {
		return "", e
	}
	im, e := firmware.Inspect(context.Background(), staged, progress(observer, "checking"))
	if e != nil {
		if staged != path {
			os.Remove(staged)
		}
		return "", e
	}
	s.image = im
	return encode(im)
}
func Releases() (string, error) {
	a, e := catalog.List(context.Background())
	if e != nil {
		return "", e
	}
	return encode(a)
}
func (s *Session) Download(asset string, observer Observer) (string, error) {
	var a catalog.Asset
	if e := json.Unmarshal([]byte(asset), &a); e != nil {
		return "", e
	}
	path, e := catalog.Download(context.Background(), a, s.dir, progress(observer, "downloading"))
	if e != nil {
		return "", e
	}
	return s.Load(path, observer)
}
func InspectDisk(d Disk) (string, error) {
	dev, e := adapt(d)
	if e != nil {
		return "", e
	}
	card, e := flash.InspectCard(dev, dev.size)
	if e != nil {
		return "", e
	}
	return encode(card)
}

type options struct {
	Mode           string `json:"mode"`
	Profile        string `json:"profile"`
	NoROMs         bool   `json:"no_roms"`
	Fingerprint    string `json:"fingerprint"`
	Identity       string `json:"identity"`
	Custom         string `json:"custom"`
	CustomAccepted bool   `json:"custom_accepted"`
}

func (s *Session) plan(d *usbDisk, input string) (*flash.Plan, options, error) {
	var opts options
	if e := json.Unmarshal([]byte(input), &opts); e != nil {
		return nil, opts, e
	}
	if s.image == nil {
		return nil, opts, errors.New("choose a Treble release first")
	}
	if opts.Identity == "" || opts.Identity != d.disk.Identity() {
		return nil, opts, errors.New("the selected card reader changed")
	}
	card, e := flash.InspectCard(d, d.size)
	if e != nil {
		return nil, opts, e
	}
	if opts.Fingerprint != card.Fingerprint {
		return nil, opts, errors.New("card changed after inspection; select it again")
	}
	if opts.Custom != "" && !opts.CustomAccepted {
		return nil, opts, errors.New(firmware.CustomWarning)
	}
	p, e := flash.Build(s.image, card, d.size, opts.Mode, opts.Profile, opts.NoROMs)
	return p, opts, e
}
func (s *Session) Plan(d Disk, input string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, e := adapt(d)
	if e != nil {
		return "", e
	}
	p, _, e := s.plan(dev, input)
	if e != nil {
		return "", e
	}
	return encode(p)
}

type anchor struct {
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	SHA256 string `json:"sha256"`
}
type record struct {
	Anchors   []anchor `json:"anchors,omitempty"`
	Directory string   `json:"directory"`
	Identity  string   `json:"identity"`
	Size      int64    `json:"size"`
	Created   string   `json:"created"`
	Version   string   `json:"version"`
	Profile   string   `json:"profile"`
}

func (s *Session) Write(d Disk, input string, observer Observer) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, e := adapt(d)
	if e != nil {
		return "", e
	}
	p, opts, e := s.plan(dev, input)
	if e != nil {
		return "", e
	}
	dir, e := os.MkdirTemp(s.dir, "operation-")
	if e != nil {
		return "", e
	}
	rec := record{Directory: dir, Identity: d.Identity(), Size: dev.size, Created: time.Now().UTC().Format(time.RFC3339), Version: s.image.Version, Profile: opts.Profile}
	// Sample both ends of each preserved partition, including its filesystem
	// identity. Reusing the same USB reader is not enough to identify a card.
	for _, region := range p.Protected {
		n := min(region.Length, 64<<10)
		for _, off := range []int64{region.Offset, region.Offset + region.Length - n} {
			hash, err := firmware.Hash(context.Background(), io.NewSectionReader(dev, off, n), n, nil)
			if err != nil {
				return "", err
			}
			rec.Anchors = append(rec.Anchors, anchor{Offset: off, Length: n, SHA256: hash})
		}
	}
	raw, _ := json.Marshal(rec)
	metadata, e := os.OpenFile(filepath.Join(dir, "operation.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	_, e = metadata.Write(raw)
	if e == nil {
		e = metadata.Sync()
	}
	closeErr := metadata.Close()
	if e != nil {
		return "", e
	}
	if closeErr != nil {
		return "", closeErr
	}
	directory, e := os.Open(dir)
	if e != nil {
		return "", e
	}
	e = directory.Sync()
	directory.Close()
	if e != nil {
		return "", e
	}
	boot, e := firmware.PrepareBoot(s.image, opts.Profile, p.NoROMs, opts.Mode == "update", opts.Custom, dir)
	if e != nil {
		return "", e
	}
	defer os.Remove(boot)
	backup := filepath.Join(dir, "recovery")
	if e = flash.Execute(context.Background(), dev, s.image, p, boot, backup, func(p flash.Progress) { emit(observer, p) }); e != nil {
		return "", fmt.Errorf("%w\nRecovery: %s", e, dir)
	}
	return encode(rec)
}
func (s *Session) Recoveries() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dirs, e := filepath.Glob(filepath.Join(s.dir, "operation-*"))
	if e != nil {
		return "", e
	}
	result := []record{}
	for _, dir := range dirs {
		if _, e := os.Stat(filepath.Join(dir, "recovery", "recovery.json")); e != nil {
			continue
		}
		b, e := os.ReadFile(filepath.Join(dir, "operation.json"))
		if e != nil {
			continue
		}
		var r record
		if json.Unmarshal(b, &r) == nil {
			r.Directory = dir
			result = append(result, r)
		}
	}
	return encode(result)
}
func (s *Session) Restore(d Disk, dir string, observer Observer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if filepath.Dir(filepath.Clean(dir)) != s.dir {
		return errors.New("choose a saved recovery backup")
	}
	dev, e := adapt(d)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(dir, "operation.json"))
	if e != nil {
		return e
	}
	var r record
	if e = json.Unmarshal(b, &r); e != nil {
		return e
	}
	if r.Identity != d.Identity() || r.Size != dev.size {
		return errors.New("this backup belongs to a different card reader or card size")
	}
	if len(r.Anchors) == 0 {
		return errors.New("recovery backup has no saved card identity")
	}
	for _, sample := range r.Anchors {
		if sample.Offset < 0 || sample.Length <= 0 || sample.Length > 64<<10 || sample.Offset > dev.size-sample.Length {
			return errors.New("invalid saved card identity")
		}
		hash, err := firmware.Hash(context.Background(), io.NewSectionReader(dev, sample.Offset, sample.Length), sample.Length, nil)
		if err != nil {
			return err
		}
		if hash != sample.SHA256 {
			return errors.New("the preserved partitions do not match this recovery backup; select the original card")
		}
	}
	emit(observer, flash.Progress{Phase: "restoring"})
	return flash.RestoreBackup(context.Background(), dev, filepath.Join(dir, "recovery"), dev.size)
}
func CustomWarning() string { return firmware.CustomWarning }

func Translate(locale, key string) string { return i18n.Text(locale, key) }
func ResolveLocale(locale string) string  { return i18n.Resolve(locale) }
