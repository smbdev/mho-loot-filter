package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"mholootfilter/internal/db"
)

// firstIcon returns the first texture of an icon chain ("A|B") that exists: an item's IconPath can name a
// texture the game files lack while its IconPathHiRes one is there.
func firstIcon(chain string, icon func(string) ([]byte, error)) ([]byte, error) {
	var err error
	for _, name := range strings.Split(chain, "|") {
		var b []byte
		if b, err = icon(name); err == nil {
			return b, nil
		}
	}
	return nil, err
}

// shopOrder ranks the kinds an item can come from instead of a drop, for the search's "only" note.
var shopOrder = map[string]int{"vendor": 1, "service": 1, "recipe": 2, "achievement": 3, "store": 4}

// dropRoutes serves the drop finder: item and boss search, item, boss and place pages, item icons and boss portraits.
// icon returns a texture as PNG bytes from the current game folder.
func dropRoutes(mux *http.ServeMux, reply func(http.ResponseWriter, int, any), icon func(string) ([]byte, error), dataDir string) {
	var once sync.Once
	var drops *db.Drops
	var loadErr error
	data := func(w http.ResponseWriter) *db.Drops {
		once.Do(func() { drops, loadErr = db.LoadDrops() })
		if loadErr != nil {
			reply(w, 500, map[string]string{"error": "The drop data is damaged: " + loadErr.Error()})
			return nil
		}
		return drops
	}
	labels := func(d *db.Drops, ids []int) []string {
		out := []string{}
		for _, l := range ids {
			out = append(out, d.Labels[l])
		}
		return out
	}

	achievementRoutes(mux, reply, data, dataDir)
	mux.HandleFunc("GET /api/drops/advanced", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		q := r.URL.Query()
		var stats []string
		for _, s := range q["stat"] {
			if s = strings.TrimSpace(s); s != "" {
				stats = append(stats, s)
			}
		}
		var slots []int
		for _, s := range q["slot"] {
			if n, err := strconv.Atoi(s); err == nil {
				slots = append(slots, n)
			}
		}
		reply(w, 200, advancedSearch(d, q.Get("q"), stats, slots, q["type"], q.Get("sort") == "stat"))
	})
	mux.HandleFunc("GET /api/drops/stats", func(w http.ResponseWriter, r *http.Request) {
		if d := data(w); d != nil {
			reply(w, 200, statsOf(d).list)
		}
	})
	mux.HandleFunc("GET /api/drops/items", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		out := []map[string]any{}
		if len([]rune(q)) < 2 {
			reply(w, 200, out)
			return
		}
		type hit struct {
			i                int
			best, conditions [3]float64 // best without and with conditions such as an event
			only             string     // with no drop: the kind of source it comes from instead
		}
		var hits []hit
		for i, it := range d.Items {
			if !strings.Contains(strings.ToLower(it.Name), q) || len(d.ItemSources[i]) == 0 {
				continue
			}
			h := hit{i: i}
			for _, si := range d.ItemSources[i] {
				if k := d.Sources[si].Kind; k != "boss" && k != "box" {
					if k == "vendor" && len(it.Service) > 0 {
						k = "service" // a crafter's service, offered rather than sold
					}
					if h.only == "" || shopOrder[k] < shopOrder[h.only] {
						h.only = k // the first of vendor, recipe, achievement, store
					}
					continue
				}
				for _, row := range d.Tables[d.Sources[si].Table].Rows {
					if row.Item != i {
						continue
					}
					for t := 0; t < 3; t++ {
						if len(row.Labels) == 0 {
							h.best[t] = max(h.best[t], row.P[t])
						} else {
							h.conditions[t] = max(h.conditions[t], row.P[t])
						}
					}
				}
			}
			hits = append(hits, h)
		}
		sort.SliceStable(hits, func(a, b int) bool {
			return max(hits[a].best[2], hits[a].conditions[2]/2) > max(hits[b].best[2], hits[b].conditions[2]/2)
		})
		for _, h := range hits[:min(len(hits), 100)] {
			it := d.Items[h.i]
			best, conditional := h.best, false
			if best == [3]float64{} && h.conditions != [3]float64{} { // only drops with a condition, such as during an event
				best, conditional = h.conditions, true
			}
			only := ""
			if best == [3]float64{} {
				only = h.only
			}
			out = append(out, map[string]any{"path": it.Path, "name": it.Name, "label": it.Label, "icon": it.Icon,
				"rarity": it.Rarity, "random": it.Random, "best": best, "conditional": conditional, "only": only})
		}
		reply(w, 200, out)
	})

	mux.HandleFunc("GET /api/drops/bosses", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		count := map[string]int{}
		for _, s := range d.Sources {
			if s.Kind == "boss" {
				count[s.Name]++
			}
		}
		out := []map[string]any{}
		for _, n := range d.BossNames {
			if q == "" || strings.Contains(strings.ToLower(n), q) {
				out = append(out, map[string]any{"name": n, "variants": count[n]})
			}
		}
		reply(w, 200, out)
	})

	mux.HandleFunc("GET /api/drops/places", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		out := []map[string]any{}
		for i, p := range d.Places {
			if q != "" && !strings.Contains(strings.ToLower(p.Name), q) {
				continue
			}
			names := map[string]bool{}
			for _, si := range d.PlaceSources[i] {
				names[d.Sources[si].Name] = true
			}
			out = append(out, map[string]any{"id": p.ID, "name": p.Name, "group": p.Group, "bosses": len(names)})
		}
		reply(w, 200, out)
	})

	mux.HandleFunc("GET /api/drops/place", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		i, ok := d.PlaceByID[r.URL.Query().Get("id")]
		if !ok {
			reply(w, 404, map[string]string{"error": "unknown place"})
			return
		}
		byName := map[string][]map[string]any{}
		seen := map[string]bool{} // versions that share a table and a hint show once, as on the boss page
		for _, si := range d.PlaceSources[i] {
			s := d.Sources[si]
			key := s.Name + "|" + strconv.Itoa(s.Table) + "|" + s.Version
			if seen[key] {
				continue
			}
			seen[key] = true
			byName[s.Name] = append(byName[s.Name], map[string]any{"id": s.ID, "version": s.Version, "icon": s.Icon})
		}
		names := make([]string, 0, len(byName))
		for n := range byName {
			names = append(names, n)
		}
		sort.Strings(names)
		bosses := []map[string]any{}
		for _, n := range names {
			icon := "" // the first version with a portrait
			for _, v := range byName[n] {
				if icon == "" {
					icon = v["icon"].(string)
				}
			}
			bosses = append(bosses, map[string]any{"name": n, "versions": byName[n], "icon": icon})
		}
		p := d.Places[i]
		reply(w, 200, map[string]any{"id": p.ID, "name": p.Name, "group": p.Group, "bosses": bosses})
	})

	mux.HandleFunc("GET /api/drops/item", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		i, ok := d.ItemByPath[r.URL.Query().Get("path")]
		if !ok {
			reply(w, 404, map[string]string{"error": "unknown item"})
			return
		}
		sources := []map[string]any{}
		pools := map[int]map[string]any{}
		for _, si := range d.ItemSources[i] {
			s := d.Sources[si]
			for _, row := range d.Tables[s.Table].Rows {
				if row.Item != i {
					continue
				}
				if row.Pool >= 0 {
					p := pools[row.Pool]
					if p == nil {
						p = map[string]any{"name": d.Pools[row.Pool].Name, "best": []float64{0, 0, 0}, "count": 0}
						pools[row.Pool] = p
					}
					best := p["best"].([]float64)
					for t := 0; t < 3; t++ {
						best[t] = max(best[t], row.P[t])
					}
					p["count"] = p["count"].(int) + 1
					continue
				}
				place, placeName := "", ""
				if s.Place >= 0 && s.Place < len(d.Places) {
					place, placeName = d.Places[s.Place].ID, d.Places[s.Place].Name
				}
				sources = append(sources, map[string]any{"id": s.ID, "kind": s.Kind, "name": s.Name, "where": s.Where,
					"p": row.P, "n": row.N, "labels": labels(d, row.Labels), "server": row.Server, "icon": s.Icon,
					"place": place, "placeName": placeName, "version": s.Version, "from": row.From, "to": row.To, "event": s.Event,
					"crafter": s.Crafter, "inputs": s.Inputs})
			}
		}
		poolList := []map[string]any{}
		for _, p := range pools {
			poolList = append(poolList, p)
		}
		it := d.Items[i]
		reply(w, 200, map[string]any{"item": map[string]any{"path": it.Path, "name": it.Name, "label": it.Label,
			"icon": it.Icon, "rarity": it.Rarity, "random": it.Random, "card": it.Card, "price": it.Price, "service": it.Service},
			"sources": sources, "pools": poolList})
	})

	mux.HandleFunc("GET /api/drops/boss", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		name := r.URL.Query().Get("name")
		variants := []map[string]any{}
		byTable := map[int]map[string]any{} // versions that share a table are one version with every hint
		for _, s := range d.Sources {
			if s.Kind != "boss" || s.Name != name {
				continue
			}
			place := map[string]any{"place": "", "placeName": "", "version": s.Version}
			if s.Place >= 0 && s.Place < len(d.Places) {
				place["place"], place["placeName"] = d.Places[s.Place].ID, d.Places[s.Place].Name
			}
			if v := byTable[s.Table]; v != nil {
				v["ids"] = append(v["ids"].([]string), s.ID)
				v["places"] = append(v["places"].([]map[string]any), place)
				if !strings.Contains(", "+v["where"].(string)+", ", ", "+s.Where+", ") && s.Where != "" {
					v["where"] = v["where"].(string) + " / " + s.Where
				}
				continue
			}
			rows := []map[string]any{}
			for _, row := range d.Tables[s.Table].Rows {
				it := d.Items[row.Item]
				pool := ""
				if row.Pool >= 0 {
					pool = d.Pools[row.Pool].Name
				}
				rows = append(rows, map[string]any{"path": it.Path, "name": it.Name, "label": it.Label, "icon": it.Icon,
					"rarity": it.Rarity, "section": it.Section, "random": it.Random, "chase": it.Chase, "p": row.P, "n": row.N,
					"labels": labels(d, row.Labels), "server": row.Server, "pool": pool})
			}
			v := map[string]any{"id": s.ID, "ids": []string{s.ID}, "where": s.Where, "fight": s.Fight, "rows": rows, "icon": s.Icon,
				"places": []map[string]any{place}}
			byTable[s.Table] = v
			variants = append(variants, v)
		}
		if len(variants) == 0 {
			reply(w, 404, map[string]string{"error": "unknown boss"})
			return
		}
		icon := ""
		for _, v := range variants {
			if icon == "" {
				icon = v["icon"].(string)
			}
		}
		reply(w, 200, map[string]any{"name": name, "variants": variants, "icon": icon})
	})

	mux.HandleFunc("GET /api/drops/vendors", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		out := []map[string]any{}
		for _, n := range d.VendorNames {
			if q == "" || strings.Contains(strings.ToLower(n), q) {
				out = append(out, map[string]any{"name": n})
			}
		}
		reply(w, 200, out)
	})

	mux.HandleFunc("GET /api/drops/vendor", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		name := r.URL.Query().Get("name")
		tabs := []map[string]any{}
		seen := map[int]bool{} // two vendor types with one name and one table show once
		for _, s := range d.Sources {
			if s.Kind != "vendor" || s.Name != name || seen[s.Table] {
				continue
			}
			seen[s.Table] = true
			rows := []map[string]any{}
			for _, row := range d.Tables[s.Table].Rows {
				it := d.Items[row.Item]
				rows = append(rows, map[string]any{"path": it.Path, "name": it.Name, "label": it.Label, "rarity": it.Rarity,
					"random": it.Random, "price": it.Price, "service": it.Service, "p": row.P, "n": row.N,
					"labels": labels(d, row.Labels), "server": row.Server, "from": row.From, "to": row.To})
			}
			tabs = append(tabs, map[string]any{"id": s.ID, "event": s.Event, "tabs": s.Tabs, "rows": rows})
		}
		if len(tabs) == 0 {
			reply(w, 404, map[string]string{"error": "unknown vendor"})
			return
		}
		reply(w, 200, map[string]any{"name": name, "tabs": tabs})
	})

	mux.HandleFunc("POST /api/drops/icons", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		var body struct {
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Paths) > 1000 {
			reply(w, 400, map[string]string{"error": "bad icon request"})
			return
		}
		out := map[string]string{}
		for _, p := range body.Paths { // an item path, a source id for a boss portrait or box icon, or ach:<id>
			chain := ""
			if i, ok := d.ItemByPath[p]; ok {
				chain = d.Items[i].Icon
			} else if i, ok := d.SourceByID[p]; ok {
				chain = d.Sources[i].Icon
			} else if id, err := strconv.Atoi(strings.TrimPrefix(p, "ach:")); err == nil && strings.HasPrefix(p, "ach:") {
				chain = d.AchievementIcon(id)
			}
			if chain == "" {
				continue
			}
			if b, err := firstIcon(chain, icon); err == nil {
				out[p] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
			}
		}
		reply(w, 200, out)
	})
}
