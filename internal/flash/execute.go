package flash

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/andr36oid/andr36oid-sdflasher/internal/firmware"
	"io"
	"os"
	"path/filepath"
)

type Progress struct {
	Phase  string `json:"phase"`
	Name   string `json:"name"`
	Done   int64  `json:"done"`
	Total  int64  `json:"total"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	Plan   *Plan  `json:"plan,omitempty"`
}
type Device interface {
	io.ReaderAt
	io.WriterAt
	Sync() error
}
type backupEntry struct {
	Range
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}
type Backup struct {
	Size      int64         `json:"size"`
	Entries   []backupEntry `json:"entries"`
	Protected []Range       `json:"protected"`
}

func copyRange(ctx context.Context, dst io.WriterAt, src io.ReaderAt, doff, soff, n int64, cb func(int64)) error {
	buf := make([]byte, 4<<20)
	for off := int64(0); off < n; {
		if e := ctx.Err(); e != nil {
			return e
		}
		sz := int64(len(buf))
		if n-off < sz {
			sz = n - off
		}
		if _, e := src.ReadAt(buf[:sz], soff+off); e != nil {
			return e
		}
		w, e := dst.WriteAt(buf[:sz], doff+off)
		if e != nil {
			return e
		}
		if int64(w) != sz {
			return io.ErrShortWrite
		}
		off += sz
		if cb != nil {
			cb(off)
		}
	}
	return nil
}
func verify(ctx context.Context, dst, src io.ReaderAt, doff, soff, n int64, cb func(int64)) error {
	a := make([]byte, 4<<20)
	b := make([]byte, len(a))
	for off := int64(0); off < n; {
		if e := ctx.Err(); e != nil {
			return e
		}
		sz := int64(len(a))
		if n-off < sz {
			sz = n - off
		}
		if _, e := dst.ReadAt(a[:sz], doff+off); e != nil {
			return e
		}
		if _, e := src.ReadAt(b[:sz], soff+off); e != nil {
			return e
		}
		if !bytes.Equal(a[:sz], b[:sz]) {
			return fmt.Errorf("read-back mismatch at byte %d", doff+off)
		}
		off += sz
		if cb != nil {
			cb(off)
		}
	}
	return nil
}
func Execute(ctx context.Context, dst Device, im *firmware.Image, p *Plan, bootPath, backupDir string, notify func(Progress)) error {
	if e := p.Validate(); e != nil {
		return e
	}
	dst = &guardedDevice{Device: dst, plan: p}
	src, e := os.Open(im.Path)
	if e != nil {
		return e
	}
	defer src.Close()
	boot, e := os.Open(bootPath)
	if e != nil {
		return e
	}
	defer boot.Close()
	bst, e := boot.Stat()
	if e != nil {
		return e
	}
	for _, w := range p.Writes {
		if w.Data != nil {
			if int64(len(w.Data)) != w.Length {
				return errors.New("invalid partition table payload")
			}
			continue
		}
		available := im.Size
		off := w.SourceOffset
		if w.Boot {
			available = bst.Size()
			off = 0
		}
		if off < 0 || off > available || w.Length > available-off {
			return errors.New("source range outside prepared image")
		}
	}

	if notify == nil {
		notify = func(Progress) {}
	}
	notify(Progress{Phase: "preflight", Name: "Checking source image", Plan: p})
	sum, e := firmware.Hash(ctx, src, im.Size, nil)
	if e != nil {
		return e
	}
	if sum != im.SHA256 {
		return errors.New("source image changed after inspection")
	}
	snap, e := Snapshot(dst, p.Size)
	if e != nil {
		return e
	}
	if snap != p.OldFingerprint {
		return errors.New("card contents changed since preflight; inspect it again")
	}
	// Pre-read every destination range. Media read failures abort before the first write.
	for _, w := range p.Writes {
		if _, e = firmware.Hash(ctx, io.NewSectionReader(dst, w.Offset, w.Length), w.Length, nil); e != nil {
			return fmt.Errorf("card read check failed: %w", e)
		}
	}
	var backup *Backup
	if p.Mode == "update" {
		backup, e = saveBackup(ctx, dst, p, backupDir, notify)
		if e != nil {
			return fmt.Errorf("recovery backup failed; no card bytes were written: %w", e)
		}
	}
	sourceFor := func(w Write) (io.ReaderAt, int64) {
		if w.Data != nil {
			return bytes.NewReader(w.Data), 0
		}
		if w.Boot {
			return boot, 0
		}
		return src, w.SourceOffset
	}
	var total int64
	for _, w := range p.Writes {
		total += w.Length
	}
	var done int64
	fail := func(cause error) error {
		if backup != nil {
			notify(Progress{Phase: "restoring", Name: "Restoring the previous installation"})
			if e := restore(context.Background(), dst, backupDir, backup); e != nil {
				return fmt.Errorf("%v; automatic restore failed: %v. Keep the card out of the console. Recovery backup: %s", cause, e, backupDir)
			}
			return fmt.Errorf("%v; the previous installation was restored and verified", cause)
		}
		return fmt.Errorf("%v; installation is incomplete, run Install again before using the card", cause)
	}
	for _, w := range p.Writes {
		r, off := sourceFor(w)
		if e = copyRange(ctx, dst, r, w.Offset, off, w.Length, func(n int64) {
			notify(Progress{Phase: "writing", Name: w.Name, Done: done + n, Total: total, Offset: w.Offset, Length: n})
		}); e != nil {
			return fail(e)
		}
		if e = dst.Sync(); e != nil {
			return fail(e)
		}
		if e = verify(ctx, dst, r, w.Offset, off, w.Length, func(n int64) {
			notify(Progress{Phase: "verifying", Name: w.Name, Done: done + n, Total: total, Offset: w.Offset, Length: n})
		}); e != nil {
			return fail(e)
		}
		done += w.Length
	}
	// Fresh installs write a prepared BOOT after the source payload; updates already included it.
	if p.Mode == "install" {
		part, _ := im.Table.Find("BOOT")
		if e = copyRange(ctx, dst, boot, part.Offset(), 0, part.Size(), func(n int64) {
			notify(Progress{Phase: "writing", Name: "Device setup", Done: n, Total: part.Size(), Offset: part.Offset(), Length: n})
		}); e != nil {
			return fail(e)
		}
		if e = dst.Sync(); e != nil {
			return fail(e)
		}
		if e = verify(ctx, dst, boot, part.Offset(), 0, part.Size(), nil); e != nil {
			return fail(e)
		}
		notify(Progress{Phase: "verifying", Name: "Device setup", Done: part.Size(), Total: part.Size(), Offset: part.Offset(), Length: part.Size()})
	}
	if sum, e := firmware.Hash(ctx, src, im.Size, nil); e != nil || sum != im.SHA256 {
		return fail(errors.New("source image changed during the operation"))
	}
	notify(Progress{Phase: "complete", Name: "Written and verified", Done: total, Total: total})
	return nil
}
func saveBackup(ctx context.Context, dev io.ReaderAt, p *Plan, dir string, notify func(Progress)) (*Backup, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	b := &Backup{Size: p.Size, Protected: p.Protected}
	for i, w := range p.Writes {
		name := fmt.Sprintf("region-%03d.bin", i)
		f, e := os.OpenFile(filepath.Join(dir, name), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		e = copyRange(ctx, f, dev, 0, w.Offset, w.Length, func(n int64) { notify(Progress{Phase: "backup", Name: w.Name, Done: n, Total: w.Length}) })
		if e == nil {
			e = f.Sync()
		}
		if e == nil {
			e = verify(ctx, f, dev, 0, w.Offset, w.Length, nil)
		}
		var sum string
		if e == nil {
			sum, e = firmware.Hash(ctx, f, w.Length, nil)
		}
		f.Close()
		if e != nil {
			return nil, e
		}
		b.Entries = append(b.Entries, backupEntry{w.Range, name, sum})
	}
	j, e := json.MarshalIndent(b, "", "  ")
	if e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "recovery.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return nil, e
	}
	_, e = f.Write(j)
	if e == nil {
		e = f.Sync()
	}
	f.Close()
	return b, e
}
func restore(ctx context.Context, dst Device, dir string, b *Backup) error {
	// Check the complete backup before restoring any part of it.
	for _, ent := range b.Entries {
		f, e := os.Open(filepath.Join(dir, ent.File))
		if e != nil {
			return e
		}
		sum, e := firmware.Hash(ctx, f, ent.Length, nil)
		f.Close()
		if e != nil {
			return e
		}
		if sum != ent.SHA256 {
			return errors.New("recovery backup checksum mismatch")
		}
		for _, r := range b.Protected {
			if ent.Offset < r.End() && r.Offset < ent.End() {
				return errors.New("backup overlaps protected data")
			}
		}
	}
	for _, ent := range b.Entries {
		f, e := os.Open(filepath.Join(dir, ent.File))
		if e != nil {
			return e
		}
		e = copyRange(ctx, dst, f, ent.Offset, 0, ent.Length, nil)
		if e == nil {
			e = dst.Sync()
		}
		if e == nil {
			e = verify(ctx, dst, f, ent.Offset, 0, ent.Length, nil)
		}
		f.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
func Digest(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }

// The final write boundary is enforced independently of the operation loop.
type guardedDevice struct {
	Device
	plan *Plan
}

func (g *guardedDevice) WriteAt(b []byte, off int64) (int, error) {
	n := int64(len(b))
	if off < 0 || off > g.plan.Size || n > g.plan.Size-off {
		return 0, errors.New("write outside card")
	}
	for _, r := range g.plan.Protected {
		if off < r.End() && r.Offset < off+n {
			return 0, fmt.Errorf("blocked write into %s", r.Name)
		}
	}
	for _, w := range g.plan.Writes {
		if off >= w.Offset && off+n <= w.End() {
			return g.Device.WriteAt(b, off)
		}
	}
	return 0, errors.New("write outside the approved plan")
}

// RestoreBackup verifies the saved regions completely before opening a write path.
func RestoreBackup(ctx context.Context, dst Device, dir string, size int64) error {
	f, e := os.Open(filepath.Join(dir, "recovery.json"))
	if e != nil {
		return e
	}
	defer f.Close()
	var b Backup
	if e = json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&b); e != nil {
		return e
	}
	if b.Size != size || len(b.Entries) == 0 || len(b.Entries) > 32 {
		return errors.New("backup does not match the destination")
	}
	p := &Plan{Size: size, Protected: b.Protected}
	for i, ent := range b.Entries {
		if ent.File != fmt.Sprintf("region-%03d.bin", i) {
			return errors.New("invalid recovery file name")
		}
		p.Writes = append(p.Writes, Write{Range: ent.Range})
	}
	if e = p.Validate(); e != nil {
		return e
	}
	return restore(ctx, &guardedDevice{Device: dst, plan: p}, dir, &b)
}
