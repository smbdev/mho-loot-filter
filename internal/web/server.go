// Package web serves the filter UI and its JSON API.
package web

import (
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"mholootfilter/internal/db"
	"mholootfilter/internal/engine"
	"mholootfilter/internal/icons"
	"mholootfilter/internal/wwise"
)

//go:embed static
var static embed.FS

// Options connects the API to the desktop: finding installs, remembering the chosen folder, a folder dialog.
type Options struct {
	Version    string                 // the app version, shown in the sidebar
	OpenURL    func(url string) error // opens a web page in the default browser
	Searching  func() bool            // true while installs are still being looked for
	Detect     func() []string
	SaveFolder func(dir string) error
	PickFolder func() (string, error)
}

func New(d *db.DB, e *engine.Engine, opts Options) http.Handler {
	byType := map[string][]string{}
	byKey := map[string]db.Item{}
	groupKeys := map[string][]string{}
	inGroup := map[string]map[string]bool{} // group -> item keys
	heroKeys := map[string][]string{}       // hero -> keys of their uniques
	for _, it := range d.Items {
		byType[it.Type] = append(byType[it.Type], it.Name)
		byKey[engine.ItemKey(it)] = it
		if it.Hero != "" {
			heroKeys[it.Hero] = append(heroKeys[it.Hero], engine.ItemKey(it))
			for _, id := range []string{heroGroup(it.Hero), allHeroes} {
				if inGroup[id] == nil {
					inGroup[id] = map[string]bool{}
				}
				inGroup[id][engine.ItemKey(it)] = true
			}
		}
		for _, g := range it.Groups {
			groupKeys[g] = append(groupKeys[g], engine.ItemKey(it))
			if inGroup[g] == nil {
				inGroup[g] = map[string]bool{}
			}
			inGroup[g][engine.ItemKey(it)] = true
		}
	}
	// namesAlone: a group can hide names without hiding items only when it holds every item of each look it
	// touches, because a name label belongs to the look.
	namesAlone := map[string]bool{}
	for g, keys := range inGroup {
		perType := map[string]int{}
		for _, it := range d.Items {
			if keys[engine.ItemKey(it)] {
				perType[it.Type]++
			}
		}
		namesAlone[g] = true
		for t, n := range perType {
			if n != len(byType[t]) {
				namesAlone[g] = false
			}
		}
	}
	describe := func(it db.Item) map[string]any {
		t := d.Types[it.Type]
		return map[string]any{
			"key": engine.ItemKey(it), "name": it.Name, "detail": it.Detail, "type": it.Type, "category": t.Category, "rarityGlow": t.RarityGlow,
			"groups":     it.Groups,
			"sharedWith": len(byType[it.Type]) - 1,
			"canGlow":    len(t.Glow) > 0, "canName": len(t.Name) > 0,
			// Uniques drawn with the rarity effect always play the Unique rarity's sound, whatever their class says.
			"soundByRarity": t.Category == "Uniques" && t.RarityGlow,
		}
	}
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	fail := func(w http.ResponseWriter, err error) {
		code := 500
		if errors.Is(err, engine.ErrGameRunning) {
			code = 409
		}
		reply(w, code, map[string]string{"error": err.Error()})
	}
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, it := range d.Search(r.URL.Query().Get("q"), 200) {
			out = append(out, describe(it))
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("GET /api/item", func(w http.ResponseWriter, r *http.Request) {
		it, ok := byKey[r.URL.Query().Get("key")]
		if !ok {
			reply(w, 404, map[string]string{"error": "unknown item"})
			return
		}
		reply(w, 200, describe(it))
	})
	mux.HandleFunc("GET /api/group", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if inGroup[id] == nil {
			reply(w, 404, map[string]string{"error": "unknown group"})
			return
		}
		out := []map[string]any{}
		for _, it := range d.Items {
			if inGroup[id][engine.ItemKey(it)] {
				out = append(out, describe(it))
			}
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("GET /api/groups", func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, g := range d.Groups {
			out = append(out, map[string]any{"id": g.ID, "label": g.Label, "note": g.Note, "count": len(groupKeys[g.ID]),
				"keys": groupKeys[g.ID], "namesAlone": namesAlone[g.ID]})
		}
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i]["label"].(string)) < strings.ToLower(out[j]["label"].(string))
		})
		reply(w, 200, out)
	})
	var iconMu sync.Mutex
	var iconSet *icons.Set
	iconDir := ""
	mux.HandleFunc("POST /api/icons", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Keys []string `json:"keys"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Keys) > 1000 {
			reply(w, 400, map[string]string{"error": "bad icon request"})
			return
		}
		iconMu.Lock()
		if dir := e.Dir(); iconSet == nil || dir != iconDir { // icons come from the current game folder
			iconSet, iconDir = icons.New(dir), dir
		}
		set := iconSet
		iconMu.Unlock()
		out := map[string]string{}
		for _, k := range body.Keys {
			if it, ok := byKey[k]; ok && it.Icon != "" {
				if b, err := set.PNG(it.Icon); err == nil {
					out[k] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
				}
			}
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("GET /api/heroes", func(w http.ResponseWriter, r *http.Request) {
		var all []string
		for _, h := range sortedKeys(heroKeys) {
			all = append(all, heroKeys[h]...)
		}
		out := []map[string]any{{"id": allHeroes, "label": "All heroes", "note": "Every hero's own uniques",
			"count": len(all), "keys": all, "namesAlone": false, "all": true}}
		for _, h := range sortedKeys(heroKeys) {
			out = append(out, map[string]any{"id": heroGroup(h), "label": h, "note": h + "'s own uniques",
				"count": len(heroKeys[h]), "keys": heroKeys[h], "namesAlone": false})
		}
		reply(w, 200, out)
	})
	mux.HandleFunc("GET /api/rarity-categories", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, engine.RarityCategories)
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		detected := []string{}
		if opts.Detect != nil {
			detected = append(detected, opts.Detect()...)
		}
		searching := opts.Searching != nil && opts.Searching()
		reply(w, 200, map[string]any{"gameDir": e.Dir(), "gameFound": engine.ValidGameDir(e.Dir()) == nil, "detected": detected, "searching": searching})
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			GameDir string `json:"gameDir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			reply(w, 400, map[string]string{"error": err.Error()})
			return
		}
		dir := strings.Trim(strings.TrimSpace(body.GameDir), `"`)
		if err := engine.ValidGameDir(dir); err != nil {
			reply(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if opts.SaveFolder != nil {
			if err := opts.SaveFolder(dir); err != nil {
				fail(w, err)
				return
			}
		}
		if err := e.SetGameDir(dir); err != nil {
			reply(w, 400, map[string]string{"error": err.Error()})
			return
		}
		reply(w, 200, map[string]any{"gameDir": e.Dir(), "gameFound": true})
	})
	mux.HandleFunc("POST /api/pick-folder", func(w http.ResponseWriter, r *http.Request) {
		if opts.PickFolder == nil {
			reply(w, 501, map[string]string{"error": "type or paste the folder path instead"})
			return
		}
		path, err := opts.PickFolder()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]string{"path": path})
	})
	mux.HandleFunc("GET /api/type", func(w http.ResponseWriter, r *http.Request) {
		k := r.URL.Query().Get("key")
		t, ok := d.Types[k]
		if !ok {
			reply(w, 404, map[string]string{"error": "unknown type"})
			return
		}
		reply(w, 200, map[string]any{"key": k, "category": t.Category, "items": byType[k], "rarityGlow": t.RarityGlow})
	})
	mux.HandleFunc("GET /api/filter", func(w http.ResponseWriter, r *http.Request) {
		f, err := e.LoadFilter()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, f)
	})
	mux.HandleFunc("PUT /api/filter", func(w http.ResponseWriter, r *http.Request) {
		var f engine.Filter
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			reply(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if err := e.SaveFilter(f); err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, f)
	})
	mux.HandleFunc("POST /api/apply", func(w http.ResponseWriter, r *http.Request) {
		f, err := e.LoadFilter()
		if err == nil {
			var rep engine.Report
			if rep, err = e.Apply(f); err == nil {
				reply(w, 200, rep)
				return
			}
		}
		fail(w, err)
	})
	mux.HandleFunc("POST /api/restore", func(w http.ResponseWriter, r *http.Request) {
		rep, err := e.RestoreAll()
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, rep)
	})
	mux.HandleFunc("GET /api/sound", func(w http.ResponseWriter, r *http.Request) {
		wem, name := e.Alert()
		wav, err := wwise.WAV(wem)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{"name": name, "custom": name != "", "wav": base64.StdEncoding.EncodeToString(wav),
			"maxSeconds": engine.MaxAlertSeconds})
	})
	mux.HandleFunc("PUT /api/sound", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name    string `json:"name"`
			Samples []byte `json:"samples"` // mono 16-bit little-endian at wwise.Rate, base64 in JSON
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Samples) < 2 {
			reply(w, 400, map[string]string{"error": "that file could not be read as a sound"})
			return
		}
		samples := make([]int16, len(body.Samples)/2)
		for i := range samples {
			samples[i] = int16(binary.LittleEndian.Uint16(body.Samples[2*i:]))
		}
		if err := e.SetAlert(body.Name, samples); err != nil {
			reply(w, 400, map[string]string{"error": err.Error()})
			return
		}
		reply(w, 200, map[string]any{})
	})
	mux.HandleFunc("DELETE /api/sound", func(w http.ResponseWriter, r *http.Request) {
		if err := e.SetAlert("", nil); err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{})
	})
	mux.HandleFunc("GET /api/update", func(w http.ResponseWriter, r *http.Request) {
		latest, err := latestRelease(r.Context(), opts.Version)
		if err != nil {
			reply(w, 502, map[string]string{"error": "Could not check for updates: " + err.Error()})
			return
		}
		reply(w, 200, map[string]any{"current": opts.Version, "latest": strings.TrimPrefix(latest.Tag, "v"),
			"newer": newer(latest.Tag, opts.Version), "url": latest.URL})
	})
	mux.HandleFunc("POST /api/open-release", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !strings.HasPrefix(body.URL, releasesPage) {
			reply(w, 400, map[string]string{"error": "only the filter's release page can be opened"})
			return
		}
		if opts.OpenURL == nil {
			reply(w, 501, map[string]string{"error": "open " + body.URL + " in your browser"})
			return
		}
		if err := opts.OpenURL(body.URL); err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		running := e.GameRunning != nil && e.GameRunning()
		reply(w, 200, map[string]any{"gameDir": e.Dir(), "gameFound": engine.ValidGameDir(e.Dir()) == nil, "gameRunning": running,
			"version": opts.Version})
	})
	sub, _ := fs.Sub(static, "static")
	mux.Handle("GET /", http.FileServerFS(sub))
	return localOnly(mux)
}

// localOnly rejects requests that did not come from the filter's own page. The server runs as administrator,
// so other websites must not be able to drive it, neither by posting to it nor through DNS rebinding.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLocalHost(u.Host) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	return host == "127.0.0.1" || host == "localhost"
}

// heroGroup is the group id of a hero's uniques, served by /api/group like the ready-made groups.
func heroGroup(hero string) string { return "hero:" + hero }

// allHeroes is the group id of every hero's own uniques together.
const allHeroes = "heroes:all"

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
