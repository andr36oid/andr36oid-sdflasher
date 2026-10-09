package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const flasherRepository = "andr36oid/andr36oid-sdflasher"

type AppUpdate struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	APK     string `json:"apk"`
}

func CheckAppUpdate(ctx context.Context, current string) (*AppUpdate, error) {
	return checkAppUpdate(ctx, http.DefaultClient, "https://api.github.com/repos/"+flasherRepository+"/releases/latest", current)
}

func checkAppUpdate(ctx context.Context, client *http.Client, endpoint, current string) (*AppUpdate, error) {
	current = "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(current) || strings.HasPrefix(current, "v0.0.0-") {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "andr36oid-sdflasher")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check returned %s", resp.Status)
	}
	var r release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, err
	}
	if r.Draft || r.Pre || !semver.IsValid(r.Tag) || semver.Compare(r.Tag, current) <= 0 {
		return nil, nil
	}
	base := "https://github.com/" + flasherRepository + "/releases/"
	u := &AppUpdate{Version: r.Tag, URL: base + "tag/" + url.PathEscape(r.Tag)}
	name := "andr36oid-sdflasher-" + r.Tag + "-android-universal.apk"
	for _, a := range r.Assets {
		if a.Name == name && a.Size > 0 && a.URL == base+"download/"+url.PathEscape(r.Tag)+"/"+name {
			u.APK = a.URL
			break
		}
	}
	return u, nil
}
