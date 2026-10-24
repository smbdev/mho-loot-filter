package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mholootfilter/internal/db"
)

const cosmicMedallion = "Entity/Items/Medals/MedalBlueprints/Endgame/CosmicMedallions/CosmicDoctorDoomEGM.prototype"

func TestDropItemSourcesAndChances(t *testing.T) {
	h := srv(t)
	var res struct {
		Item    struct{ Name, Label string }
		Sources []struct {
			Name string
			P    []float64
		}
	}
	get(t, h, "/api/drops/item?path="+cosmicMedallion, &res)
	if res.Item.Name != "Doctor Doom Medallion" || res.Item.Label == "" {
		t.Fatalf("item: %+v", res.Item)
	}
	found := false
	for _, s := range res.Sources {
		if s.Name == "Doctor Doom" && s.P[2] > 0.0099 && s.P[2] < 0.0101 && s.P[0] == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no 1%% Cosmic Doom source: %+v", res.Sources)
	}
}

func TestDropSearchEmptyAndShortQueries(t *testing.T) {
	h := srv(t)
	var items []map[string]any
	get(t, h, "/api/drops/items?q=d", &items)
	if len(items) != 0 {
		t.Fatalf("1-letter query returned %d items", len(items))
	}
	get(t, h, "/api/drops/items?q=zzqqxx", &items)
	if len(items) != 0 {
		t.Fatal("nonsense query matched")
	}
	get(t, h, "/api/drops/items?q=medallion", &items)
	if len(items) == 0 || len(items) > 100 {
		t.Fatalf("medallion search: %d", len(items))
	}
	var bosses []struct{ Name string }
	get(t, h, "/api/drops/bosses", &bosses)
	if len(bosses) < 50 {
		t.Fatalf("all bosses: %d", len(bosses))
	}
}

func TestDropBossGroupsVariants(t *testing.T) {
	h := srv(t)
	var res struct {
		Name     string
		Variants []struct {
			ID   string
			Rows []struct{ Path string }
		}
	}
	get(t, h, "/api/drops/boss?name=Doctor%20Doom", &res)
	if len(res.Variants) < 2 {
		t.Fatalf("Doctor Doom variants: %d", len(res.Variants))
	}
}

func TestDropItemWithNoSourceOnATier(t *testing.T) {
	h := srv(t)
	var res struct {
		Sources []struct {
			ID string
			P  []float64
		}
	}
	get(t, h, "/api/drops/item?path="+cosmicMedallion, &res)
	seen := false
	for _, s := range res.Sources {
		if s.ID == "Entity/Characters/Bosses/PVEDailies/Cosmic/EGD09CDrDoomPhase3.prototype" {
			seen = true
			if s.P[0] != 0 || s.P[2] == 0 {
				t.Fatalf("Cosmic Doom gives the Cosmic medallion on Red and Cosmic only: %+v", s)
			}
		}
	}
	if !seen {
		t.Fatal("Cosmic terminal Doom is not a source")
	}
}

func TestDropUnknownItemAndBoss404(t *testing.T) {
	h := srv(t)
	for _, url := range []string{"/api/drops/item?path=Nope", "/api/drops/boss?name=Nobody"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", url, nil)
		req.Host = "127.0.0.1:4000"
		h.ServeHTTP(rec, req)
		if rec.Code != 404 {
			t.Fatalf("%s: %d", url, rec.Code)
		}
	}
}

func TestDropBossVersionsThatShareATableMerge(t *testing.T) {
	h := srv(t)
	var res struct {
		Variants []struct {
			IDs   []string
			Where string
		}
	}
	get(t, h, "/api/drops/boss?name=Juggernaut", &res)
	ids := 0
	for _, v := range res.Variants {
		ids += len(v.IDs)
	}
	// Juggernaut's two Holo-Sim entities share one table: one version; each terminal colour is its own version, as
	// it runs on its own difficulty
	if len(res.Variants) != 16 || ids != 17 {
		t.Fatalf("Juggernaut: %d versions holding %d sources, want 16 and 17", len(res.Variants), ids)
	}
}

func TestDropSearchConditionOnlyItemShowsItsChance(t *testing.T) {
	h := srv(t)
	var items []struct {
		Name        string
		Best        []float64
		Conditional bool
	}
	get(t, h, "/api/drops/items?q=kraven%27s%20last%20hunt", &items) // drops only in the Daily Bugle
	if len(items) == 0 || items[0].Best[2] == 0 || !items[0].Conditional {
		t.Fatalf("Kraven's Last Hunt: %+v", items)
	}
}

func TestDropEventOnlyItemsAreLeftOut(t *testing.T) {
	h := srv(t)
	var items []struct{ Name string }
	get(t, h, "/api/drops/items?q=chest%20of%20odin", &items)
	if len(items) != 0 {
		t.Fatalf("Chest of Odin's Bounty drops only during an event: %+v", items)
	}
}

func TestDropBossRowsMarkChaseItems(t *testing.T) {
	h := srv(t)
	var res struct {
		Variants []struct {
			Rows []struct {
				Path  string
				Chase bool
			}
		}
	}
	get(t, h, "/api/drops/boss?name=Doctor%20Doom", &res)
	chase, rune := false, false
	for _, v := range res.Variants {
		for _, r := range v.Rows {
			if r.Path == cosmicMedallion && r.Chase {
				chase = true
			}
			if strings.Contains(r.Path, "/Runewords/Glyphs/") {
				rune = rune || r.Chase
			}
		}
	}
	if !chase || rune {
		t.Fatalf("Cosmic medallion chase: %v, a rune chase: %v", chase, rune)
	}
}

func TestDropBossFightVersionsAreFlagged(t *testing.T) {
	h := srv(t)
	var res struct {
		Variants []struct {
			Fight bool
			Where string
		}
	}
	get(t, h, "/api/drops/boss?name=Skrull%20Cyclops", &res)
	if len(res.Variants) != 2 || !res.Variants[0].Fight || !strings.Contains(res.Variants[0].Where, "Abnormal Mutations") ||
		!strings.HasSuffix(res.Variants[1].Where, "(Cosmic)") {
		t.Fatalf("Skrull Cyclops: %+v", res.Variants)
	}
}

func TestDropPlacesListAndFilter(t *testing.T) {
	h := srv(t)
	var places []struct {
		ID, Name, Group string
		Bosses          int
	}
	get(t, h, "/api/drops/places", &places)
	if len(places) < 20 || places[0].Group != "Terminals" {
		t.Fatalf("places: %d, first %+v", len(places), places)
	}
	get(t, h, "/api/drops/places?q=castle", &places)
	if len(places) != 1 || places[0].Name != "Castle Doom" || places[0].Bosses < 2 {
		t.Fatalf("castle: %+v", places)
	}
	get(t, h, "/api/drops/places?q=zzqq", &places)
	if len(places) != 0 {
		t.Fatalf("nonsense filter matched %d", len(places))
	}
}

func TestDropPlacePageGroupsVersionsByBoss(t *testing.T) {
	h := srv(t)
	var res struct {
		Name   string
		Bosses []struct {
			Name     string
			Versions []struct{ ID, Version string }
		}
	}
	get(t, h, "/api/drops/place?id=terminal-castle-doom", &res)
	var doom []struct{ ID, Version string }
	for _, b := range res.Bosses {
		if b.Name == "Doctor Doom" {
			doom = b.Versions
		}
	}
	if res.Name != "Castle Doom" || len(doom) != 3 {
		t.Fatalf("Castle Doom: %+v", res)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/drops/place?id=nope", nil)
	req.Host = "127.0.0.1:4000"
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("unknown place: %d", rec.Code)
	}
}

func TestDropIconFallsBackThroughTheItemsIconChain(t *testing.T) {
	// Art331's IconPath texture does not exist; its IconPathHiRes one does
	get := func(name string) ([]byte, error) {
		if name == "MarvelUIIcons_HD.Item_Art331" {
			return []byte("png"), nil
		}
		return nil, errors.New("no such texture")
	}
	if b, err := firstIcon("MarvelUIIcons.Art_331|MarvelUIIcons_HD.Item_Art331", get); err != nil || string(b) != "png" {
		t.Fatalf("fallback: %q %v", b, err)
	}
	if _, err := firstIcon("MarvelUIIcons.Art_366", get); err == nil {
		t.Fatal("a missing texture must fail")
	}
}

const crossbonesCosmic = "Entity/Items/Artifacts/Prototypes/SpecialArtifacts/CosmicArtifacts/Art338Cosmic.prototype"

func TestDropItemCardPortraitsAndPlaces(t *testing.T) {
	h := srv(t)
	var res struct {
		Item struct {
			Card struct {
				Title    string
				Color    string
				Sections []struct{ Kind string }
			}
		}
		Sources []struct {
			Name, Kind, Icon, Place, PlaceName, Version string
		}
	}
	get(t, h, "/api/drops/item?path="+crossbonesCosmic, &res)
	if res.Item.Card.Title != "Crossbones's Cosmically Enhanced Teaching Credentials" || res.Item.Card.Color != "D8E415" || len(res.Item.Card.Sections) == 0 {
		t.Fatalf("card: %+v", res.Item.Card)
	}
	placed := false
	for _, s := range res.Sources {
		if s.Kind == "boss" && s.Icon == "" {
			t.Fatalf("boss without a portrait: %+v", s)
		}
		if s.Place != "" && s.PlaceName != "" {
			placed = true
		}
	}
	if !placed {
		t.Fatalf("no source with a place: %+v", res.Sources)
	}
}

func TestDropBossAndPlacePortraits(t *testing.T) {
	h := srv(t)
	var boss struct {
		Icon     string
		Variants []struct{ Icon string }
	}
	get(t, h, "/api/drops/boss?name=Doctor%20Doom", &boss)
	if boss.Icon == "" || boss.Variants[0].Icon == "" {
		t.Fatalf("boss portrait: %+v", boss)
	}
	var place struct {
		Bosses []struct{ Icon string }
	}
	get(t, h, "/api/drops/place?id=terminal-castle-doom", &place)
	if len(place.Bosses) == 0 || place.Bosses[0].Icon == "" {
		t.Fatalf("place portraits: %+v", place)
	}
}

func TestDropIconsAnswerSourceIDs(t *testing.T) {
	icon := func(name string) ([]byte, error) { return []byte(name), nil } // the texture name stands in for its PNG
	mux := http.NewServeMux()
	dropRoutes(mux, func(w http.ResponseWriter, code int, v any) { w.WriteHeader(code); json.NewEncoder(w).Encode(v) }, icon)
	d, _ := db.LoadDrops()
	var boss db.DropSource
	for _, s := range d.Sources {
		if s.Kind == "boss" && s.Icon != "" {
			boss = s
			break
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/drops/icons", strings.NewReader(`{"paths":["`+boss.ID+`"]}`)))
	var got map[string]string
	json.Unmarshal(rec.Body.Bytes(), &got)
	if !strings.HasPrefix(got[boss.ID], "data:image/png;base64,") {
		t.Fatalf("no portrait for %s: %s", boss.ID, rec.Body)
	}
}

func TestPatrolVersionsOnlyOnTheirDifficulties(t *testing.T) {
	h := srv(t)
	var res struct {
		Sources []struct {
			PlaceName, Version string
			P                  []float64
		}
	}
	// the regular patrols open on Normal and Heroic only, their Cosmic maps on Cosmic only
	get(t, h, "/api/drops/item?path=Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot1/Unique485.prototype", &res)
	seen := 0
	for _, s := range res.Sources {
		if s.PlaceName != "Hightown Patrol" {
			continue
		}
		seen++
		if s.Version == "" && (s.P[0] == 0 || s.P[2] != 0) || s.Version == "Cosmic" && (s.P[0] != 0 || s.P[1] != 0 || s.P[2] == 0) {
			t.Fatalf("Hightown Patrol %q chances %v", s.Version, s.P)
		}
	}
	if seen != 2 {
		t.Fatalf("want both Hightown Patrol versions, got %d", seen)
	}
}

func TestDropBossVersionsNameTheirPlaces(t *testing.T) {
	h := srv(t)
	var boss struct {
		Variants []struct {
			Places []struct{ Place, PlaceName, Version string }
		}
	}
	get(t, h, "/api/drops/boss?name=Crossbones", &boss)
	names := map[string]bool{}
	for _, v := range boss.Variants {
		for _, p := range v.Places {
			names[p.PlaceName+"|"+p.Version] = true
		}
	}
	if !names["Hightown Patrol|"] || !names["Hightown Patrol|Cosmic"] {
		t.Fatalf("Crossbones' places: %v", names)
	}
}

func TestDropItemShopSources(t *testing.T) {
	h := srv(t)
	type src struct {
		Kind, Name, Event, Crafter string
		From, To                   int
		Inputs                     []struct {
			Item, Text string
			Count      int
		}
		P []float64
	}
	var doop struct {
		Item struct {
			Price []struct {
				Amount   int
				Currency string
			}
		}
		Sources []src
	}
	get(t, h, "/api/drops/item?path=Entity/Items/Artifacts/Prototypes/SpecialArtifacts/Art242.prototype", &doop)
	sold := false
	for _, s := range doop.Sources {
		if s.Kind == "vendor" && s.Name == "Cosmic Chaos Supplier" && s.Event == "Cosmic Chaos" && s.From == 1 && s.To == 0 && s.P[0] > 0 {
			sold = true
		}
	}
	if !sold || len(doop.Item.Price) == 0 || doop.Item.Price[0].Currency != "Cosmic Worldstones" {
		t.Fatalf("Doop Ectoplasm: %+v", doop)
	}
	var twilight struct{ Sources []src }
	get(t, h, "/api/drops/item?path=Entity/Items/Armor/UniquePrototypes/Avatars/AnyHero/Slot1/Unique303.prototype", &twilight)
	made := false
	for _, s := range twilight.Sources {
		if s.Kind == "recipe" && s.Crafter != "" && len(s.Inputs) > 0 {
			made = true
		}
	}
	if !made {
		t.Fatalf("Fragment of Twilight is crafted: %+v", twilight.Sources)
	}
}

func TestDropSearchFindsShopOnlyItems(t *testing.T) {
	h := srv(t)
	var items []struct {
		Path string
		Only string
		Best []float64
	}
	get(t, h, "/api/drops/items?q=doop%20ectoplasm", &items)
	for _, it := range items {
		if it.Path == "Entity/Items/Artifacts/Prototypes/SpecialArtifacts/Art242.prototype" {
			if it.Only != "vendor" || it.Best[2] != 0 {
				t.Fatalf("a vendor-only item shows where it comes from, not a drop chance: %+v", it)
			}
			return
		}
	}
	t.Fatalf("Doop Ectoplasm not found: %+v", items)
}

func TestDropVendorStockByRank(t *testing.T) {
	h := srv(t)
	type row struct {
		Path, Name string
		From, To   int
		P          []float64
		Service    *struct {
			Tab     string
			Credits int
		}
	}
	var v struct {
		Name string
		Tabs []struct {
			Event string
			Tabs  []string
			Rows  []row
		}
	}
	get(t, h, "/api/drops/vendor?name=Crafter", &v)
	if len(v.Tabs) == 0 || len(v.Tabs[0].Tabs) < 3 || v.Tabs[0].Tabs[0] != "Catalyst" {
		t.Fatalf("Crafter: %+v", v)
	}
	seen := map[string]int{}
	var affix *row
	for i, r := range v.Tabs[0].Rows {
		seen[r.Path]++
		if r.Path == "Entity/Items/Crafting/Recipes/Tab1Catalyst/Grade1Offense.prototype" {
			affix = &v.Tabs[0].Rows[i]
		}
	}
	for path, n := range seen {
		if n > 1 {
			t.Fatalf("%s listed %d times", path, n)
		}
	}
	if affix == nil || affix.From != 2 || affix.To != 0 || affix.Service == nil || affix.Service.Tab != "Catalyst" || affix.Service.Credits != 500 {
		t.Fatalf("Offensive Affix I: %+v", affix)
	}
	var item struct {
		Item    struct{ Service *struct{ Tab string } }
		Sources []struct {
			Kind     string
			From, To int
		}
	}
	get(t, h, "/api/drops/item?path=Entity/Items/Crafting/Recipes/Tab1Catalyst/Grade1Offense.prototype", &item)
	if item.Item.Service == nil || len(item.Sources) != 1 || item.Sources[0].From != 2 {
		t.Fatalf("Offensive Affix I page: %+v", item)
	}
	var hits []struct{ Path, Only string }
	get(t, h, "/api/drops/items?q=offensive%20affix%20i", &hits)
	if len(hits) == 0 || hits[0].Only != "service" {
		t.Fatalf("a crafting service is found as a service: %+v", hits)
	}
	var list []struct{ Name string }
	get(t, h, "/api/drops/vendors", &list)
	if len(list) < 20 {
		t.Fatalf("vendors: %d", len(list))
	}
}
