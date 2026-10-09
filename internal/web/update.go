package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ReleasesAPI answers with the latest published release. Tests point it at a local server.
var ReleasesAPI = "https://api.github.com/repos/smbdev/mho-loot-filter/releases/latest"

// releasesPage prefixes every page the app may open in the browser.
const releasesPage = "https://github.com/smbdev/mho-loot-filter/releases/"

type release struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

func latestRelease(ctx context.Context, version string) (release, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ReleasesAPI, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MHOLootFilter/"+version) // GitHub refuses requests without one
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.Tag == "" {
		return release{}, errors.New("GitHub sent an answer the filter does not understand")
	}
	if !strings.HasPrefix(r.URL, releasesPage) {
		r.URL = releasesPage + "latest"
	}
	return r, nil
}

// newer reports whether version a ("v2.10") is later than b ("2.9"), comparing each number in turn.
func newer(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}
