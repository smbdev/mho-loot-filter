package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"mholootfilter/internal/db"
)

// matchWords is true when every word of the query is in one of the fields (case-insensitive); the Achievements page
// searches with the same rule.
func matchWords(query string, fields ...string) bool {
	all := strings.ToLower(strings.Join(fields, "\n"))
	for _, w := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(all, w) {
			return false
		}
	}
	return true
}

// statLines are the stat lines of an item's card (the "lines" sections), as plain text, then the bonuses it can roll
// (the "options" sections, such as a Bloodstone Demonband's random bonuses) marked "Can roll: "; descriptions are
// not stats.
func statLines(card json.RawMessage) []string {
	var c struct {
		Sections []struct {
			Kind    string        `json:"kind"`
			Lines   [][][]*string `json:"lines"`
			Options [][][]*string `json:"options"`
		} `json:"sections"`
	}
	if len(card) == 0 || json.Unmarshal(card, &c) != nil {
		return nil
	}
	text := func(line [][]*string) string {
		var b strings.Builder
		for _, seg := range line {
			if len(seg) > 0 && seg[0] != nil {
				b.WriteString(*seg[0])
			}
		}
		return b.String()
	}
	var out, rolls []string
	for _, s := range c.Sections {
		switch s.Kind {
		case "lines":
			for _, line := range s.Lines {
				out = append(out, text(line))
			}
		case "options":
			for _, line := range s.Options {
				rolls = append(rolls, "Can roll: "+text(line))
			}
		}
	}
	return append(out, rolls...)
}

var number = regexp.MustCompile(`\d[\d,]*(?:\.\d+)?`)

// statValue is the size of a stat in a tooltip line: the last number before the stat's name, which is the top of a
// range ("+343-442 Critical Hit Rating" is 442), else the biggest number in the line.
func statValue(line, term string) float64 {
	at := strings.Index(strings.ToLower(line), strings.ToLower(term))
	before, biggest := -1.0, 0.0
	for _, loc := range number.FindAllStringIndex(line, -1) {
		v, err := strconv.ParseFloat(strings.ReplaceAll(line[loc[0]:loc[1]], ",", ""), 64)
		if err != nil {
			continue
		}
		if at >= 0 && loc[1] <= at {
			before = v
		}
		biggest = max(biggest, v)
	}
	if before >= 0 {
		return before
	}
	return biggest
}

var plainStat = regexp.MustCompile(`^(?:Can roll: )?(?:Rank \d+: )?[+-]?\d[\d,]*(?:\.\d+)?%?(?:-\d[\d,]*(?:\.\d+)?%?)? (\D+)$`)

// statName is the stat a plain stat line gives ("+700-1,000 Energy Damage Rating" gives Energy Damage Rating), or ""
// for a line that is an effect ("When you hit, gain 60 Energy for 5s").
func statName(line string) string {
	if m := plainStat.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

// searchable is an item the advanced search lists
func searchable(d *db.Drops, i int) bool { return !d.Items[i].Random && len(d.ItemSources[i]) > 0 }

type statIndex struct {
	list  []map[string]any  // name and how many searchable items have it, most items first
	names map[string]string // lower case to the game's spelling
}

var statIndexes sync.Map // *db.Drops to its statIndex, built on first use

// statsOf are the stat names on searchable items' tooltips, for the stat picker and for exact matching
func statsOf(d *db.Drops) statIndex {
	if s, ok := statIndexes.Load(d); ok {
		return s.(statIndex)
	}
	count, names := map[string]int{}, map[string]string{}
	for i, it := range d.Items {
		if !searchable(d, i) {
			continue
		}
		has := map[string]bool{}
		for _, l := range statLines(it.Card) {
			if n := statName(l); n != "" {
				if spelled, ok := names[strings.ToLower(n)]; ok {
					n = spelled // one stat spelled two ways counts once, under the first spelling seen
				}
				names[strings.ToLower(n)] = n
				has[n] = true
			}
		}
		for n := range has {
			count[n]++
		}
	}
	s := statIndex{list: []map[string]any{}, names: names}
	for n, c := range count {
		s.list = append(s.list, map[string]any{"name": n, "items": c})
	}
	sort.Slice(s.list, func(a, b int) bool {
		if ca, cb := s.list[a]["items"].(int), s.list[b]["items"].(int); ca != cb {
			return ca > cb
		}
		return s.list[a]["name"].(string) < s.list[b]["name"].(string)
	})
	statIndexes.Store(d, s)
	return s
}

// advancedSearch lists the items whose name has every word of name, with every stat term in some stat line, in any of the slots and any of the types
// (each group only when it has ticks), sorted by name, or with byStat by the first stat's size, biggest first.
// A term that is a stat's name matches that stat only (Energy is not Energy Damage Rating); any other term matches
// any line with it in. With no term and no tick it finds nothing.
func advancedSearch(d *db.Drops, name string, stats []string, slots []int, types []string, byStat bool) []map[string]any {
	out := []map[string]any{}
	if len(stats) == 0 && len(slots) == 0 && len(types) == 0 {
		return out
	}
	names := statsOf(d).names
	for i, it := range d.Items {
		if !searchable(d, i) || !matchWords(name, it.Name) {
			continue
		}
		if len(slots) > 0 && !slices.Contains(slots, it.Slot) {
			continue
		}
		if len(types) > 0 && !slices.ContainsFunc(types, func(t string) bool { return slices.Contains(it.Types, t) }) {
			continue
		}
		var match []string
		size := 0.0 // how big the first stat is on this item
		if len(stats) > 0 {
			lines := statLines(it.Card)
			ok := true
			for _, term := range stats {
				found := false
				exact, isStat := names[strings.ToLower(term)]
				for _, l := range lines {
					if isStat && strings.EqualFold(statName(l), exact) || !isStat && strings.Contains(strings.ToLower(l), strings.ToLower(term)) {
						found = true
						if term == stats[0] {
							size = max(size, statValue(l, term))
						}
						if !slices.Contains(match, l) {
							match = append(match, l)
						}
					}
				}
				ok = ok && found
			}
			if !ok {
				continue
			}
		}
		out = append(out, map[string]any{"path": it.Path, "name": it.Name, "label": it.Label, "icon": it.Icon,
			"rarity": it.Rarity, "match": match, "size": size})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if sa, sb := out[a]["size"].(float64), out[b]["size"].(float64); byStat && sa != sb {
			return sa > sb
		}
		return out[a]["name"].(string) < out[b]["name"].(string)
	})
	return out
}

// ticks are the achievements the player has ticked as done, kept in <data dir>/achievements.json.
type ticks struct {
	mu   sync.Mutex
	path string
}

func (t *ticks) load() ([]int, error) {
	done := []int{}
	b, err := os.ReadFile(t.path)
	if os.IsNotExist(err) {
		return done, nil
	}
	if err != nil {
		return nil, err // unreadable is not empty: a tick saved now would overwrite every other one
	}
	var f struct {
		Done []int `json:"done"`
	}
	if json.Unmarshal(b, &f) != nil {
		os.Rename(t.path, t.path+".bad") // keep a damaged file for the user rather than overwrite it on the next tick
		return done, nil
	}
	if f.Done != nil {
		done = f.Done
	}
	return done, nil
}

func (t *ticks) set(id int, on bool) ([]int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	done, err := t.load()
	if err != nil {
		return nil, err
	}
	done = slices.DeleteFunc(done, func(x int) bool { return x == id })
	if on {
		done = append(done, id)
	}
	sort.Ints(done)
	b, _ := json.Marshal(map[string][]int{"done": done})
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return nil, err
	}
	tmp := t.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return nil, err
	}
	return done, os.Rename(tmp, t.path)
}

func achievementRoutes(mux *http.ServeMux, reply func(http.ResponseWriter, int, any), data func(http.ResponseWriter) *db.Drops, dataDir string) {
	tk := &ticks{path: filepath.Join(dataDir, "achievements.json")}

	mux.HandleFunc("GET /api/achievements", func(w http.ResponseWriter, r *http.Request) {
		d := data(w)
		if d == nil {
			return
		}
		list := make([]map[string]any, 0, len(d.Achievements))
		for _, a := range d.Achievements {
			items := []map[string]any{}
			for _, i := range a.Items {
				it := d.Items[i]
				items = append(items, map[string]any{"path": it.Path, "name": it.Name, "icon": it.Icon, "rarity": it.Rarity})
			}
			steps := []map[string]any{}
			for _, o := range a.Objectives {
				step := map[string]any{"id": o.ID, "name": o.Name, "dep": o.Dep}
				if o.Item >= 0 && o.Item < len(d.Items) {
					step["item"] = map[string]any{"path": d.Items[o.Item].Path, "name": d.Items[o.Item].Name}
				}
				steps = append(steps, step)
			}
			list = append(list, map[string]any{"objectives": steps, "id": a.ID, "name": a.Name, "desc": a.Desc, "reward": a.Reward,
				"items": items, "icon": a.Icon, "points": a.Points, "cat": a.Cat, "sub": a.Sub, "vis": a.Vis,
				"parent": a.Parent, "party": a.Party, "rtypes": a.Types})
		}
		tk.mu.Lock()
		done, err := tk.load()
		tk.mu.Unlock()
		if err != nil {
			reply(w, 500, map[string]string{"error": "Could not read your ticks: " + err.Error()})
			return
		}
		reply(w, 200, map[string]any{"categories": d.AchCategories, "achievements": list, "done": done})
	})
	mux.HandleFunc("GET /api/achievements/done", func(w http.ResponseWriter, r *http.Request) {
		tk.mu.Lock()
		defer tk.mu.Unlock()
		done, err := tk.load()
		if err != nil {
			reply(w, 500, map[string]string{"error": "Could not read your ticks: " + err.Error()})
			return
		}
		reply(w, 200, done)
	})
	mux.HandleFunc("PUT /api/achievements/done", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID   int  `json:"id"`
			Done bool `json:"done"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == 0 {
			reply(w, 400, map[string]string{"error": "send {id, done}"})
			return
		}
		done, err := tk.set(body.ID, body.Done)
		if err != nil {
			reply(w, 500, map[string]string{"error": "Could not save your ticks: " + err.Error()})
			return
		}
		reply(w, 200, done)
	})
}
