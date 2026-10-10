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

	"encoding/base64"
	"mholootfilter/internal/db"
	"mholootfilter/internal/engine"
	"mholootfilter/internal/launch"
	"mholootfilter/internal/patch"
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
	sum := 0
	for _, x := range heroes[1:] {
		sum += len(x.Keys)
	}
	get(t, h, "/api/group?id="+url.QueryEscape(heroes[0].ID), &members)
	if heroes[0].Label != "All heroes" || len(heroes[0].Keys) != sum || len(members) != sum {
		t.Fatalf("All heroes: %q with %d keys and %d members, want %d", heroes[0].Label, len(heroes[0].Keys), len(members), sum)
	}
}

func TestIconsComeFromTheGameFolder(t *testing.T) {
	d, _ := db.Load()
	game := t.TempDir()
	cooked := filepath.Join(game, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	os.MkdirAll(cooked, 0o755)
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ICO__MarvelUIIcons_SF.upk"))
	if err != nil {
		t.Skip("testdata/ICO__MarvelUIIcons_SF.upk not found: run python tools/make_fixtures.py first")
	}
	os.WriteFile(filepath.Join(cooked, "ICO__MarvelUIIcons_SF.upk"), b, 0o644)
	h := New(d, &engine.Engine{DB: d, GameDir: game, DataDir: t.TempDir()}, Options{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/icons", strings.NewReader(`{"keys":["marvelitem_insignia_avengers|Insignia of Thor","nope|Nothing"]}`))
	req.Host = "127.0.0.1:4000"
	h.ServeHTTP(rec, req)
	var got map[string]string
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || !strings.HasPrefix(got["marvelitem_insignia_avengers|Insignia of Thor"], "data:image/png;base64,") {
		t.Fatalf("got %d icons: %v", len(got), rec.Body.String()[:min(200, rec.Body.Len())])
	}
}

func TestProfilesSwitchTheFilterAndSoundVolume(t *testing.T) {
	h := srv(t)
	var p engine.Profiles
	if rec := send(t, h, "PUT", "/api/filter", `{"rarities":{"Common":true}}`); rec.Code != 200 {
		t.Fatalf("save filter: %d", rec.Code)
	}
	if rec := send(t, h, "PUT", "/api/sound/volume", `{"volume":4}`); rec.Code != 200 {
		t.Fatalf("volume: %d %s", rec.Code, rec.Body)
	}
	if rec := send(t, h, "PUT", "/api/sound/volume", `{"volume":29}`); rec.Code != 400 {
		t.Fatalf("volume out of range accepted: %d", rec.Code)
	}
	rec := send(t, h, "POST", "/api/profiles", "")
	json.Unmarshal(rec.Body.Bytes(), &p)
	if rec.Code != 200 || len(p.List) != 2 || p.Active != p.List[1].ID {
		t.Fatalf("add: %d %+v", rec.Code, p)
	}
	var f engine.Filter
	var s struct{ Volume float32 }
	get(t, h, "/api/filter", &f)
	get(t, h, "/api/sound", &s)
	if len(f.Rarities) != 0 || s.Volume != 8 {
		t.Fatalf("new profile starts empty with the default volume: %+v %v", f.Rarities, s.Volume)
	}
	if rec := send(t, h, "PUT", fmt.Sprintf("/api/profiles/%d", p.Active), `{"name":"Holo-Sim"}`); rec.Code != 200 {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if rec := send(t, h, "POST", fmt.Sprintf("/api/profiles/%d/use", p.List[0].ID), ""); rec.Code != 200 {
		t.Fatalf("use: %d %s", rec.Code, rec.Body)
	}
	get(t, h, "/api/filter", &f)
	get(t, h, "/api/sound", &s)
	if !f.Rarities["Common"] || s.Volume != 4 {
		t.Fatalf("first profile not back: %+v %v", f.Rarities, s.Volume)
	}
	get(t, h, "/api/profiles", &p)
	if p.List[1].Name != "Holo-Sim" {
		t.Fatalf("rename lost: %+v", p)
	}
	if rec := send(t, h, "DELETE", fmt.Sprintf("/api/profiles/%d", p.List[1].ID), ""); rec.Code != 200 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := send(t, h, "DELETE", fmt.Sprintf("/api/profiles/%d", p.List[0].ID), ""); rec.Code != 400 {
		t.Fatalf("last profile deleted: %d", rec.Code)
	}
	if rec := send(t, h, "POST", "/api/profiles/abc/use", ""); rec.Code != 400 {
		t.Fatalf("bad id: %d", rec.Code)
	}
}

func TestTweaksAreStoredAndChecked(t *testing.T) {
	h := srv(t)
	if rec := send(t, h, "PUT", "/api/tweaks", `{"fps":144,"skipIntro":true,"textureMB":1024}`); rec.Code != 200 {
		t.Fatalf("set: %d %s", rec.Code, rec.Body)
	}
	if rec := send(t, h, "PUT", "/api/tweaks", `{"textureMB":10}`); rec.Code != 400 {
		t.Fatalf("bad texture memory accepted: %d", rec.Code)
	}
	var got patch.Tweaks
	get(t, h, "/api/tweaks", &got)
	if got != (patch.Tweaks{FPS: 144, SkipIntro: true, TextureMB: 1024}) {
		t.Fatalf("stored: %+v", got)
	}
}

func TestPlayStartsTheChosenWayUnlessTheGameRuns(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(t.TempDir(), "steamapps", "common", "Marvel Heroes")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	running := false
	var started []launch.Way
	h := New(d, &engine.Engine{DB: d, GameDir: game, DataDir: t.TempDir(), GameRunning: func() bool { return running }},
		Options{Launch: func(w launch.Way) error { started = append(started, w); return nil }})
	var ways []map[string]any
	get(t, h, "/api/play", &ways)
	if len(ways) != 1 || ways[0]["id"] != "steam" || ways[0]["name"] != "Steam" {
		t.Fatalf("ways: %v", ways)
	}
	if rec := send(t, h, "POST", "/api/play", `{"id":"nope"}`); rec.Code != 404 {
		t.Errorf("unknown way: %d", rec.Code)
	}
	if rec := send(t, h, "POST", "/api/play", `{"id":"steam"}`); rec.Code != 200 || len(started) != 1 || started[0].URL != launch.SteamURL {
		t.Fatalf("play: %d %s %+v", rec.Code, rec.Body, started)
	}
	running = true
	if rec := send(t, h, "POST", "/api/play", `{"id":"steam"}`); rec.Code != 409 || len(started) != 1 {
		t.Errorf("started a second game: %d", rec.Code)
	}
}

func TestMutesAreStoredAndChecked(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := db.LoadSounds()
	if err != nil {
		t.Fatal(err)
	}
	h := New(d, &engine.Engine{DB: d, Sounds: sounds, GameDir: t.TempDir(), DataDir: t.TempDir()}, Options{})
	var list struct {
		Groups []string
		Sounds []map[string]string
	}
	get(t, h, "/api/sounds", &list)
	if len(list.Sounds) < 4000 || list.Groups[0] != "Hero voices" {
		t.Fatalf("got %d sounds, groups %v", len(list.Sounds), list.Groups)
	}
	if rec := send(t, h, "PUT", "/api/mutes", `["play_sfx_ui_levelup","play_sfx_ui_levelup"]`); rec.Code != 200 {
		t.Fatalf("mute: %d %s", rec.Code, rec.Body)
	}
	if rec := send(t, h, "PUT", "/api/mutes", `["play_made_up"]`); rec.Code != 400 {
		t.Fatalf("unknown sound accepted: %d", rec.Code)
	}
	var muted []string
	get(t, h, "/api/mutes", &muted)
	if len(muted) != 1 || muted[0] != "play_sfx_ui_levelup" {
		t.Fatalf("stored: %v", muted)
	}
}

func TestSoundPreviewsComeFromTheGameFiles(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := db.LoadSounds()
	if err != nil {
		t.Fatal(err)
	}
	pck, err := os.ReadFile(filepath.Join("..", "..", "testdata", "SFX_Shared_INT.pck"))
	if err != nil {
		t.Skip("testdata/SFX_Shared_INT.pck not found: run python tools/make_fixtures.py first")
	}
	game := t.TempDir()
	cooked := filepath.Join(game, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	os.MkdirAll(cooked, 0o755)
	os.WriteFile(filepath.Join(cooked, "SFX_Shared_INT.pck"), pck, 0o644)
	h := New(d, &engine.Engine{DB: d, Sounds: sounds, GameDir: game, DataDir: t.TempDir()}, Options{})
	rec := send(t, h, "POST", "/api/sound-preview", `{"name":"play_sfx_ui_levelup"}`)
	var got struct{ URL string }
	json.Unmarshal(rec.Body.Bytes(), &got)
	ogg, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(got.URL, "data:audio/ogg;base64,"))
	if rec.Code != 200 || !strings.HasPrefix(got.URL, "data:audio/ogg;base64,") || string(ogg[:4]) != "OggS" {
		t.Fatalf("preview: %d %.80s", rec.Code, rec.Body)
	}
	if rec := send(t, h, "POST", "/api/sound-preview", `{"name":"play_made_up"}`); rec.Code != 404 {
		t.Fatalf("unknown sound: %d", rec.Code)
	}
}
