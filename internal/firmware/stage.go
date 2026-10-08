package firmware

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxImageSize int64 = 32 << 30

// Stage expands exactly one image. ZIP CRC verification completes before the file is accepted.
func Stage(ctx context.Context, src, dir string, progress func(int64, int64)) (string, error) {
	if strings.EqualFold(filepath.Ext(src), ".img") {
		return src, nil
	}
	if !strings.EqualFold(filepath.Ext(src), ".zip") {
		return "", errors.New("choose an .img or .img.zip release image")
	}
	z, e := zip.OpenReader(src)
	if e != nil {
		return "", e
	}
	defer z.Close()
	var entry *zip.File
	for _, f := range z.File {
		if strings.EqualFold(filepath.Ext(f.Name), ".img") {
			if entry != nil {
				return "", errors.New("archive contains multiple images")
			}
			entry = f
		}
	}
	if entry == nil {
		return "", errors.New("archive contains no disk image; recovery OTA packages are not supported")
	}
	if entry.UncompressedSize64 > uint64(MaxImageSize) {
		return "", errors.New("image is too large")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	dst, e := os.CreateTemp(dir, "image-*.img")
	if e != nil {
		return "", e
	}
	name := dst.Name()
	ok := false
	defer func() {
		dst.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	r, e := entry.Open()
	if e != nil {
		return "", e
	}
	defer r.Close()
	buf := make([]byte, 4<<20)
	var written int64
	for {
		if e = ctx.Err(); e != nil {
			return "", e
		}
		n, re := r.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > MaxImageSize {
				return "", errors.New("expanded image exceeds limit")
			}
			if _, e = dst.Write(buf[:n]); e != nil {
				return "", e
			}
			if progress != nil {
				progress(written, int64(entry.UncompressedSize64))
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return "", fmt.Errorf("damaged archive: %w", re)
		}
	}
	if e = dst.Sync(); e != nil {
		return "", e
	}
	if written != int64(entry.UncompressedSize64) {
		return "", errors.New("truncated archive")
	}
	ok = true
	return name, nil
}
