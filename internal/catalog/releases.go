package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const Repository = "andr36oid/releases"

type Asset struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	URL     string `json:"browser_download_url"`
	Digest  string `json:"digest"`
	Release string `json:"-"`
}
type release struct {
	Name   string  `json:"name"`
	Tag    string  `json:"tag_name"`
	Draft  bool    `json:"draft"`
	Pre    bool    `json:"prerelease"`
	Assets []Asset `json:"assets"`
}

func client() *http.Client { return &http.Client{Timeout: 30 * time.Minute} }
func List(ctx context.Context) ([]Asset, error) {
	var out []Asset
	for page := 1; page <= 10; page++ {
		req, e := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100&page=%d", Repository, page), nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "andr36oid-sdflasher")
		resp, e := client().Do(req)
		if e != nil {
			return nil, e
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub returned %s", resp.Status)
		}
		var rel []release
		e = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rel)
		resp.Body.Close()
		if e != nil {
			return nil, e
		}
		for _, r := range rel {
			if r.Draft {
				continue
			}
			for _, a := range r.Assets {
				n := strings.ToLower(a.Name)
				if strings.HasSuffix(n, ".img.zip") || strings.HasSuffix(n, ".img") {
					a.Release = r.Name
					if a.Release == "" {
						a.Release = r.Tag
					}
					if r.Pre {
						a.Release += " (pre-release)"
					}
					out = append(out, a)
				}
			}
		}
		if len(rel) < 100 {
			break
		}
	}
	return out, nil
}
func Download(ctx context.Context, a Asset, dir string, progress func(int64, int64)) (string, error) {
	u, e := url.Parse(a.URL)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || !strings.HasPrefix(u.Path, "/"+Repository+"/releases/download/") {
		return "", errors.New("invalid release download URL")
	}
	if a.Size <= 0 || a.Size > 32<<30 {
		return "", errors.New("invalid release size")
	}
	digest := strings.TrimPrefix(a.Digest, "sha256:")
	if len(digest) != 64 {
		return "", errors.New("this release has no SHA-256 digest; download it manually and select the local image")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	final := filepath.Join(dir, digest+"-"+filepath.Base(a.Name))
	req, e := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if e != nil {
		return "", e
	}
	resp, e := client().Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download returned %s", resp.Status)
	}
	f, e := os.CreateTemp(dir, "download-*")
	if e != nil {
		return "", e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	h := sha256.New()
	w := io.MultiWriter(f, h)
	buf := make([]byte, 1<<20)
	var n int64
	for {
		read, re := resp.Body.Read(buf)
		if read > 0 {
			n += int64(read)
			if n > a.Size {
				return "", errors.New("download exceeds expected size")
			}
			if _, e = w.Write(buf[:read]); e != nil {
				return "", e
			}
			if progress != nil {
				progress(n, a.Size)
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			return "", re
		}
	}
	if n != a.Size || hex.EncodeToString(h.Sum(nil)) != digest {
		return "", errors.New("download checksum mismatch")
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	f.Close()
	if e = os.Rename(tmp, final); e != nil {
		return "", e
	}
	return final, nil
}
