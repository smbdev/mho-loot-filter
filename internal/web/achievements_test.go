package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/engine"
)

func srvIn(t *testing.T, dataDir string) http.Handler {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	return New(d, &engine.Engine{DB: d, GameDir: t.TempDir(), DataDir: dataDir, GameRunning: func() bool { return false }}, Options{})
}

type advHit struct {
	Name, Path string
	Match      []string
}

func names(hits []advHit) map[string]bool {
	out := map[string]bool{}
	for _, h := range hits {
		out[h.Name] = true
	}
	return out
}

func TestAdvancedStatAndType(t *testing.T) {
	var hits []advHit
	get(t, srv(t), "/api/drops/advanced?stat=Critical+Hit+Rating&type=uniques", &hits)
	for _, h := range hits {
		if h.Name == "Heavily Modified Mercenary Assault Rifle" {
			if len(h.Match) == 0 || !strings.Contains(h.Match[0], "+343-442 Critical Hit Rating") {
				t.Fatalf("match lines: %v", h.Match)
			}
			return
		}
	}
	t.Fatalf("rifle not found in %d hits", len(hits))
}

func TestAdvancedFindsLegendaryRank(t *testing.T) {
	var hits []advHit
	get(t, srv(t), "/api/drops/advanced?stat=Odinrage", &hits)
	if !names(hits)["Gungnir"] {
		t.Fatalf("no Gungnir: %v", hits)
	}
	get(t, srv(t), "/api/drops/advanced?stat=Critical&q=gungnir", &hits)
	if len(hits) == 0 || !names(hits)["Gungnir"] || len(names(hits)) != 1 {
		t.Fatalf("name words narrow the list: %v", names(hits))
	}
}

func TestAdvancedGroupsCombine(t *testing.T) {
	h := srv(t)
	var hits []advHit
	get(t, h, "/api/drops/advanced?slot=1&type=rings", &hits)
	if len(hits) != 0 {
		t.Fatalf("rings have no gear slot, got %v", hits)
	}
	get(t, h, "/api/drops/advanced?type=rings&type=artifacts", &hits)
	ring, artifact := false, false
	for _, x := range hits {
		ring = ring || strings.Contains(x.Path, "/Rings/") || strings.Contains(x.Path, "Ring")
		artifact = artifact || strings.Contains(x.Path, "/Artifacts/")
	}
	if !ring || !artifact {
		t.Fatalf("rings %v artifacts %v in %d hits", ring, artifact, len(hits))
	}
}

func TestAdvancedEmpty(t *testing.T) {
	var hits []advHit
	get(t, srv(t), "/api/drops/advanced", &hits)
	if hits == nil || len(hits) != 0 {
		t.Fatalf("want [], got %v", hits)
	}
}

func TestAchievementsRoute(t *testing.T) {
	var res struct {
		Categories   []struct{ Name string }
		Achievements []struct {
			Name   string
			Points int
			Items  []struct{ Name string }
		}
		Done []int
	}
	get(t, srv(t), "/api/achievements", &res)
	if len(res.Categories) == 0 || res.Done == nil {
		t.Fatalf("categories %d done %v", len(res.Categories), res.Done)
	}
	for _, a := range res.Achievements {
		if a.Name == "Cosmic Bounty Hunter: Mandarin" {
			if a.Points != 10 || len(a.Items) != 1 || a.Items[0].Name != "Coffer of Achievement" {
				t.Fatalf("mandarin: %+v", a)
			}
			return
		}
	}
	t.Fatal("no Mandarin achievement")
}

func TestAchievementTicks(t *testing.T) {
	dir := t.TempDir()
	h := srvIn(t, dir)
	var done []int
	get(t, h, "/api/achievements/done", &done)
	if done == nil || len(done) != 0 {
		t.Fatalf("no file: want [], got %v", done)
	}
	if rec := send(t, h, "PUT", "/api/achievements/done", `{"id":1916,"done":true}`); rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	get(t, h, "/api/achievements/done", &done)
	if len(done) != 1 || done[0] != 1916 {
		t.Fatalf("after tick: %v", done)
	}
	send(t, h, "PUT", "/api/achievements/done", `{"id":1916,"done":false}`)
	get(t, h, "/api/achievements/done", &done)
	if len(done) != 0 {
		t.Fatalf("after untick: %v", done)
	}
	os.WriteFile(filepath.Join(dir, "achievements.json"), []byte("{broken"), 0o644)
	get(t, h, "/api/achievements/done", &done)
	if len(done) != 0 {
		t.Fatalf("corrupt file: %v", done)
	}
	if _, err := os.Stat(filepath.Join(dir, "achievements.json.bad")); err != nil {
		t.Fatalf("corrupt file not kept: %v", err)
	}
}

func TestMatchWords(t *testing.T) {
	if !matchWords("mandarin coffer", "Cosmic Bounty Hunter: Mandarin", "Coffer of Achievement") {
		t.Fatal("words in two fields should match")
	}
	if matchWords("mandarin coffer", "Mandarin", "Infinity Orb") {
		t.Fatal("a missing word should not match")
	}
}

func TestAchievementArtworkByID(t *testing.T) {
	icon := func(name string) ([]byte, error) { return []byte(name), nil } // the texture name stands in for its PNG
	mux := http.NewServeMux()
	dropRoutes(mux, func(w http.ResponseWriter, code int, v any) { w.WriteHeader(code); json.NewEncoder(w).Encode(v) }, icon, t.TempDir())
	d, _ := db.LoadDrops()
	var plain, bare db.Achievement // one with its own artwork, one without that gives an item
	for _, a := range d.Achievements {
		if a.Icon != "" && plain.ID == 0 {
			plain = a
		}
		if a.Icon == "" && len(a.Items) > 0 && bare.ID == 0 {
			bare = a
		}
	}
	keys := []string{"ach:" + strconv.Itoa(plain.ID), "ach:" + strconv.Itoa(bare.ID)}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/drops/icons", strings.NewReader(`{"paths":["`+keys[0]+`","`+keys[1]+`"]}`)))
	var got map[string]string
	json.Unmarshal(rec.Body.Bytes(), &got)
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(plain.Icon))
	if got[keys[0]] != want {
		t.Fatalf("%s: got %q", keys[0], got[keys[0]])
	}
	if bare.ID != 0 && got[keys[1]] == "" {
		t.Fatalf("%s (%s) has no artwork, its reward item's icon should stand in", keys[1], bare.Name)
	}
}

func TestAchievementTicksSurviveAReadError(t *testing.T) {
	// a file that exists but cannot be read (here a folder in its place; on Windows, antivirus holding it) must not
	// read as "no ticks": the next tick would overwrite every saved one
	dir := t.TempDir()
	h := srvIn(t, dir)
	os.Mkdir(filepath.Join(dir, "achievements.json"), 0o755)
	if rec := send(t, h, "PUT", "/api/achievements/done", `{"id":1916,"done":true}`); rec.Code != 500 {
		t.Fatalf("put on an unreadable file: want 500, got %d %s", rec.Code, rec.Body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/achievements/done", nil)
	req.Host = "127.0.0.1:4000"
	h.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Fatalf("get on an unreadable file: want 500, got %d %s", rec.Code, rec.Body)
	}
}

func TestStatValue(t *testing.T) {
	cases := []struct {
		line, term string
		want       float64
	}{
		{"+343-442 Critical Hit Rating", "critical hit rating", 442},
		{"+471-647 Damage Rating to Ranged Powers", "damage rating", 647},
		{"When you use your Signature, gain 196-319 Critical Hit Rating and Brutal Strike Rating for 15s.", "critical hit", 319},
		{"5-7% chance to fire an explosive round, dealing 50,000 damage to your target. (10s cooldown)", "chance", 7},
		{"Rank 2: +1,500 Critical Damage Rating", "critical damage", 1500},
	}
	for _, c := range cases {
		if got := statValue(c.line, c.term); got != c.want {
			t.Errorf("%q / %q: got %v, want %v", c.line, c.term, got, c.want)
		}
	}
}

func TestAdvancedSortsByTheStatHighestFirst(t *testing.T) {
	var hits []advHit
	get(t, srv(t), "/api/drops/advanced?stat=Brutal+Strike&sort=stat", &hits)
	if len(hits) < 3 {
		t.Fatalf("hits: %d", len(hits))
	}
	last := 1e18
	for _, h := range hits {
		v := 0.0
		for _, m := range h.Match {
			v = max(v, statValue(m, "brutal strike"))
		}
		if v > last {
			t.Fatalf("%s (%v) after a lower value %v", h.Name, v, last)
		}
		last = v
	}
}

func TestAchievementObjectives(t *testing.T) {
	var res struct {
		Achievements []struct {
			ID         int
			Name       string
			Objectives []struct {
				ID   int
				Name string
				Dep  int
				Item *struct{ Path, Name string }
			}
		}
	}
	get(t, srv(t), "/api/achievements", &res)
	for _, a := range res.Achievements {
		if a.Name != "Arm Yourself" {
			continue
		}
		for _, o := range a.Objectives {
			if o.Name == "The Darkhold" {
				if o.Item == nil || !strings.HasSuffix(o.Item.Path, "/Unique079.prototype") {
					t.Fatalf("Darkhold item: %+v", o.Item)
				}
				return
			}
		}
		t.Fatalf("no Darkhold in %d objectives", len(a.Objectives))
	}
	t.Fatal("no Arm Yourself")
}

func TestAdvancedFindsBonusesAnItemCanRoll(t *testing.T) {
	// a Bloodstone Demonband's random bonuses are not fixed stats, but it can have them: search finds it, marked
	var hits []advHit
	get(t, srv(t), "/api/drops/advanced?stat=Summoned+Ally+Duration", &hits)
	for _, h := range hits {
		if h.Path == "Entity/Items/Rings/BloodstoneRing.prototype" {
			if len(h.Match) == 0 || h.Match[0] != "Can roll: +10.0-15.0% Summoned Ally Duration" {
				t.Fatalf("match: %v", h.Match)
			}
			return
		}
	}
	t.Fatalf("no Bloodstone Demonband in %d hits", len(hits))
}
