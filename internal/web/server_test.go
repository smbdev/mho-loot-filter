package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/engine"
)

func srv(t *testing.T) http.Handler {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	return New(d, &engine.Engine{DB: d, GameDir: t.TempDir(), DataDir: t.TempDir(), GameRunning: func() bool { return false }}, Options{})
}

func get(t *testing.T, h http.Handler, url string, v any) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", url, nil)
	req.Host = "127.0.0.1:4000"
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%s: %d %s", url, rec.Code, rec.Body)
	}
	json.Unmarshal(rec.Body.Bytes(), v)
}

func TestSearchStripsMarkupAndCase(t *testing.T) {
	h := srv(t)
	var res []map[string]any
	get(t, h, "/api/search?q=relic%20of%20ATLANTIS", &res)
	if len(res) != 1 || res[0]["name"] != "Relic of Atlantis" || res[0]["sharedWith"].(float64) != 0 {
		t.Fatalf("got %v", res)
	}
	get(t, h, "/api/search?q=captain%20america%27s", &res)
	if len(res) == 0 {
		t.Fatal("apostrophe search found nothing")
	}
	for _, r := range res {
		if strings.Contains(r["name"].(string), "#") {
			t.Fatalf("markup leaked: %v", r["name"])
		}
	}
}

func TestSharedTypeListsOtherItems(t *testing.T) {
	h := srv(t)
	var res []map[string]any
	get(t, h, "/api/search?q=Gem%20of%20the%20Kursed", &res)
	var key string
	for _, r := range res {
		if r["name"] == "Gem of the Kursed" {
			key = r["type"].(string)
		}
	}
	var ty map[string]any
	get(t, h, "/api/type?key="+key, &ty)
	if len(ty["items"].([]any)) < 10 {
		t.Fatalf("expected the shared items, got %v", ty)
	}
}

func TestFilterRoundTrip(t *testing.T) {
	h := srv(t)
	rec := httptest.NewRecorder()
	put := httptest.NewRequest("PUT", "/api/filter", strings.NewReader(`{"items":{"t|Relic of Atlantis":{"hide":true}},"looks":{"x":{"Glow":true}},"rarities":{"Common":true}}`))
	put.Host = "127.0.0.1:4000"
	h.ServeHTTP(rec, put)
	if rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	var f engine.Filter
	get(t, h, "/api/filter", &f)
	if !f.Looks["x"].Glow || !f.Rarities["Common"] || !f.Items["t|Relic of Atlantis"].Hide {
		t.Fatalf("filter not saved: %+v", f)
	}
}

func TestRejectsOtherHostsAndOrigins(t *testing.T) {
	h := srv(t)
	cases := []struct {
		method, host, origin string
		want                 int
	}{
		{"GET", "evil.example", "", http.StatusForbidden},                       // DNS rebinding
		{"POST", "127.0.0.1:4000", "http://evil.example", http.StatusForbidden}, // cross-site form or fetch
		{"PUT", "127.0.0.1:4000", "http://evil.example", http.StatusForbidden},
		{"GET", "127.0.0.1:4000", "", http.StatusOK},
		{"PUT", "localhost:4000", "http://localhost:4000", http.StatusOK},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, "/api/filter", strings.NewReader(`{"types":{},"rarities":{}}`))
		req.Host = c.host
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s host=%s origin=%s: got %d, want %d", c.method, c.host, c.origin, rec.Code, c.want)
		}
	}
}

func send(t *testing.T, h http.Handler, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Host = "127.0.0.1:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGroupsRoute(t *testing.T) {
	var groups []map[string]any
	get(t, srv(t), "/api/groups", &groups)
	for _, g := range groups {
		if g["id"] == "relics" && g["count"].(float64) > 10 {
			return
		}
	}
	t.Fatalf("relics group missing or empty: %v", groups)
}

func TestSearchGivesItemKeysAndSharing(t *testing.T) {
	var res []map[string]any
	get(t, srv(t), "/api/search?q=Uru-Forged%20Battle%20Axe", &res)
	if len(res) != 1 || res[0]["key"] != "marvelitem_uruforgedbase|Uru-Forged Battle Axe" || res[0]["sharedWith"].(float64) != 22 ||
		fmt.Sprint(res[0]["groups"]) != "[uru]" {
		t.Fatalf("got %v", res)
	}
	var it map[string]any
	get(t, srv(t), "/api/item?key="+url.QueryEscape(res[0]["key"].(string)), &it)
	if it["name"] != "Uru-Forged Battle Axe" {
		t.Fatalf("item route: %v", it)
	}
}

func TestSettingsRoutes(t *testing.T) {
	d, _ := db.Load()
	game := t.TempDir()
	for _, rel := range []string{"UnrealEngine3/MarvelGame/CookedPCConsole/MarvelGame.upk", "UnrealEngine3/Binaries/Win64/MarvelHeroesOmega.exe"} {
		p := filepath.Join(game, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	saved := ""
	e := &engine.Engine{DB: d, GameDir: t.TempDir(), DataDir: t.TempDir()}
	h := New(d, e, Options{
		Searching:  func() bool { return true },
		Detect:     func() []string { return []string{game} },
		SaveFolder: func(dir string) error { saved = dir; return nil },
	})
	var s map[string]any
	get(t, h, "/api/settings", &s)
	if s["gameFound"] != false || len(s["detected"].([]any)) != 1 || s["searching"] != true {
		t.Fatalf("settings: %v", s)
	}
	if rec := send(t, h, "PUT", "/api/settings", `{"gameDir":"C:\\nowhere"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad folder accepted: %d", rec.Code)
	}
	body, _ := json.Marshal(map[string]string{"gameDir": game})
	if rec := send(t, h, "PUT", "/api/settings", string(body)); rec.Code != 200 || e.Dir() != game || saved != game {
		t.Fatalf("good folder: %d dir=%s saved=%s", rec.Code, e.Dir(), saved)
	}
	if rec := send(t, h, "POST", "/api/pick-folder", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("pick-folder without a dialog: %d", rec.Code)
	}
	h = New(d, e, Options{PickFolder: func() (string, error) { return `D:\Games\MHO`, nil }})
	var picked map[string]string
	rec := send(t, h, "POST", "/api/pick-folder", "")
	json.Unmarshal(rec.Body.Bytes(), &picked)
	if picked["path"] != `D:\Games\MHO` {
		t.Fatalf("pick-folder: %s", rec.Body)
	}
}

func TestGroupsSayWhenNamesNeedHiding(t *testing.T) {
	var groups []map[string]any
	get(t, srv(t), "/api/groups", &groups)
	want := map[string]bool{"cosmic-artifacts": false, "uniques": true, "relics": true}
	for _, g := range groups {
		if w, ok := want[g["id"].(string)]; ok && g["namesAlone"] != w {
			t.Errorf("%s: namesAlone = %v, want %v", g["id"], g["namesAlone"], w)
		}
	}
}

func TestSettingsAcceptsCopiedPathWithQuotes(t *testing.T) {
	d, _ := db.Load()
	game := t.TempDir()
	for _, rel := range []string{"UnrealEngine3/MarvelGame/CookedPCConsole/MarvelGame.upk", "UnrealEngine3/Binaries/Win64/MarvelHeroesOmega.exe"} {
		p := filepath.Join(game, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	e := &engine.Engine{DB: d, GameDir: t.TempDir(), DataDir: t.TempDir()}
	h := New(d, e, Options{SaveFolder: func(string) error { return errors.New("disk full") }})
	body, _ := json.Marshal(map[string]string{"gameDir": ` "` + game + `" `})
	before := e.Dir()
	if rec := send(t, h, "PUT", "/api/settings", string(body)); rec.Code == 200 || e.Dir() != before {
		t.Fatalf("a folder that could not be saved must not be switched to: %d %s", rec.Code, e.Dir())
	}
	h = New(d, e, Options{SaveFolder: func(string) error { return nil }})
	if rec := send(t, h, "PUT", "/api/settings", string(body)); rec.Code != 200 || e.Dir() != game {
		t.Fatalf("quoted path from Explorer should work: %d %s", rec.Code, rec.Body)
	}
}

func TestGroupMembersRoute(t *testing.T) {
	var items []map[string]any
	get(t, srv(t), "/api/group?id=relics", &items)
	if len(items) != 14 || items[0]["key"] == "" {
		t.Fatalf("relics group should list its 14 items: %d %v", len(items), items)
	}
	rec := send(t, srv(t), "GET", "/api/group?id=nope", "")
	if rec.Code != 404 {
		t.Fatalf("unknown group: %d", rec.Code)
	}
}

func TestSoundUploadAndReset(t *testing.T) {
	h := srv(t)
	send := func(method, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/sound", strings.NewReader(body))
		req.Host = "127.0.0.1:4000"
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	var s struct {
		Name   string `json:"name"`
		Custom bool   `json:"custom"`
		Wav    []byte `json:"wav"`
	}
	get(t, h, "/api/sound", &s)
	if s.Custom || string(s.Wav[:4]) != "RIFF" {
		t.Fatalf("built-in sound expected: %+v", s.Name)
	}
	// two samples, base64 of 00 10 00 f0
	if code := send("PUT", `{"name":"ding.wav","samples":"ABAA8A=="}`); code != 200 {
		t.Fatalf("upload: %d", code)
	}
	get(t, h, "/api/sound", &s)
	if !s.Custom || s.Name != "ding.wav" || len(s.Wav) != 44+4 {
		t.Fatalf("custom sound not served: %q %d", s.Name, len(s.Wav))
	}
	if code := send("PUT", `{"name":"x","samples":""}`); code != 400 {
		t.Fatalf("empty sound accepted: %d", code)
	}
	if code := send("DELETE", ""); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	get(t, h, "/api/sound", &s)
	if s.Custom {
		t.Fatal("built-in sound not restored")
	}
}

func TestGroupsAreAlphabetical(t *testing.T) {
	var groups []struct {
		Label string `json:"label"`
	}
	get(t, srv(t), "/api/groups", &groups)
	for i := 1; i < len(groups); i++ {
		if strings.ToLower(groups[i-1].Label) > strings.ToLower(groups[i].Label) {
			t.Fatalf("%q listed before %q", groups[i-1].Label, groups[i].Label)
		}
	}
	if len(groups) < 2 {
		t.Fatal("no groups")
	}
}

func TestHeroUniquesAreListedAndServed(t *testing.T) {
	h := srv(t)
	var heroes []struct {
		ID    string   `json:"id"`
		Label string   `json:"label"`
		Keys  []string `json:"keys"`
	}
	get(t, h, "/api/heroes", &heroes)
	if len(heroes) < 60 {
		t.Fatalf("only %d heroes", len(heroes))
	}
	var jean struct {
		ID   string
		Keys []string
	}
	for _, x := range heroes {
		if x.Label == "Jean Grey" {
			jean.ID, jean.Keys = x.ID, x.Keys
		}
	}
	var members []struct {
		Key string `json:"key"`
	}
	get(t, h, "/api/group?id="+url.QueryEscape(jean.ID), &members)
	if len(members) != len(jean.Keys) || len(members) < 5 {
		t.Fatalf("Jean Grey: %d keys, %d members", len(jean.Keys), len(members))
	}
}
