package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/engine"
)

func TestNewerComparesNumbers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"v2.7", "2.6", true}, {"v2.10", "2.9", true}, {"v2.6", "2.6", false}, {"v2.5", "2.6", false}, {"v3", "2.9", true}, {"v2.6.1", "2.6", true}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestUpdateCheckAndOpen(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "no agent", 403)
			return
		}
		w.Write([]byte(`{"tag_name":"v2.7","html_url":"https://github.com/smbdev/mho-loot-filter/releases/tag/v2.7"}`))
	}))
	defer gh.Close()
	old := ReleasesAPI
	ReleasesAPI = gh.URL
	defer func() { ReleasesAPI = old }()

	d, _ := db.Load()
	var opened string
	h := New(d, &engine.Engine{DB: d, GameDir: t.TempDir(), DataDir: t.TempDir()}, Options{Version: "2.6", OpenURL: func(u string) error { opened = u; return nil }})
	var u struct {
		Latest string `json:"latest"`
		Newer  bool   `json:"newer"`
		URL    string `json:"url"`
	}
	get(t, h, "/api/update", &u)
	if !u.Newer || u.Latest != "2.7" || !strings.HasSuffix(u.URL, "/tag/v2.7") {
		t.Fatalf("got %+v", u)
	}
	post := func(body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/open-release", strings.NewReader(body))
		req.Host = "127.0.0.1:4000"
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if post(`{"url":"`+u.URL+`"}`) != 200 || opened != u.URL {
		t.Fatal("release page not opened")
	}
	if post(`{"url":"https://evil.example/"}`) != 400 {
		t.Fatal("only the release page may be opened")
	}
}
