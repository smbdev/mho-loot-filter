package db

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"sort"
)

//go:embed drops.json.gz
var rawDrops []byte

// DropItem is an item, or a random-item line, that a drop row names.
type DropItem struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Label   string `json:"label"`   // tells items with the same name apart, such as "Cosmic"
	Icon    string `json:"icon"`    // texture names to try in turn, "A|B": IconPath, IconPathHiRes, StoreIconPath
	Rarity  string `json:"rarity"`  // "cosmic", "unique" or "" (name colour)
	Section string `json:"section"` // "gear" or "currency"
	Random  bool   `json:"random"`  // a random item of a kind, not one named item
	Chase   bool   `json:"chase"`   // a unique, artifact or medallion of a boss's own
	// Card is the item's tooltip as the game lays it out (tools/tooltip.py); only the page reads it
	Card json.RawMessage `json:"card,omitempty"`
	// Price is what a vendor asks, [{amount, currency}], when it is one fixed number
	Price json.RawMessage `json:"price,omitempty"`
	// Service is a crafter's or enchanter's service (a crafting recipe): {tab, unlock, credits}
	Service json.RawMessage `json:"service,omitempty"`
	Slot    int             `json:"slot,omitempty"`  // the hero's Gear Slot 1 to 5, or 0
	Types   []string        `json:"types,omitempty"` // advanced search kinds: rings, artifacts, uniques, ...
}

// AchCategory is a category of the game's Achievements window and its subcategories, in the window's order.
type AchCategory struct {
	Name string   `json:"name"`
	Subs []string `json:"subs"`
}

// Achievement is one achievement the game's window can show.
type Achievement struct {
	ID     int             `json:"id"`
	Name   string          `json:"name"`
	Desc   json.RawMessage `json:"desc"`   // text segments, [[text, colour or null]]
	Reward string          `json:"reward"` // the reward line without "Reward: "
	Items  []int           `json:"items"`  // the reward's items, indexes into Items
	Icon   string          `json:"icon"`   // the artwork's texture name
	Points int             `json:"points"`
	Cat    int             `json:"cat"`
	Sub    int             `json:"sub"`
	Order  int             `json:"order"`
	Vis    int             `json:"vis"`    // AchievementVisibleState: 4 = shown once earned (secret)
	Parent int             `json:"parent"` // the tier before it, or 0
	Party  bool            `json:"party"`
	Types  []string        `json:"rtypes"` // reward types: coffer, orb, gear, title, none, ...
	// Objectives are what the game's expanded row lists: other achievements (Dep) or items to loot (Item)
	Objectives []AchObjective `json:"objectives"`
}

// AchObjective is one step of an achievement.
type AchObjective struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Dep  int    `json:"dep"`  // the achievement it is, or 0
	Item int    `json:"item"` // the item it asks for, an index into Items, or -1
}

// DropRow is one item a source's table can drop, with its chance of at least one and expected count per tier.
type DropRow struct {
	Item   int       `json:"i"`
	Pool   int       `json:"pool"` // shared pool it comes through, or -1
	Labels []int     `json:"l"`    // conditions such as an event
	P      []float64 `json:"p"`
	N      []float64 `json:"n"`
	Server int       `json:"s"`              // 0 game data, 1 added by the server, 2 changed by the server
	From   int       `json:"from,omitempty"` // vendor stock: the first vendor rank it is stocked at
	To     int       `json:"to,omitempty"`   // and the last, or 0 for that rank and up
}

type DropTable struct {
	Rows []DropRow `json:"rows"`
}

// DropPlace is a place bosses are found in: a terminal, patrol, game mode, raid or story chapter.
type DropPlace struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"` // "Terminals", "Patrols", "Game modes", "Raids" or "Story"
}

// DropSource is a boss, named enemy or box and the table it drops from.
type DropSource struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // "boss", "box", "vendor", "recipe", "achievement" or "store"
	Name    string `json:"name"`
	Where   string `json:"where"`
	Table   int    `json:"table"`
	Fight   bool   `json:"fight"`   // a group fight's reward, one roll each time the group is beaten
	Place   int    `json:"place"`   // index into Places, or -1
	Version string `json:"version"` // tells a boss's versions in one place apart, such as "Red terminal"
	Icon    string `json:"icon"`    // the boss's portrait or the box's icon, texture names to try in turn
	Event   string `json:"event"`   // the events a vendor only appears in
	Crafter string `json:"crafter"` // the vendors that craft a recipe
	// Tabs are a crafter's service tabs in the game's order
	Tabs []string `json:"tabs,omitempty"`
	// Inputs are a recipe's ingredients, [{item, count, text}]
	Inputs json.RawMessage `json:"inputs,omitempty"`
}

type DropPool struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// Drops is the drop finder's data, generated by tools/build_drops.py.
type Drops struct {
	Version int          `json:"version"`
	Tiers   []string     `json:"tiers"`
	Labels  []string     `json:"labels"`
	Items   []DropItem   `json:"items"`
	Pools   []DropPool   `json:"pools"`
	Places  []DropPlace  `json:"places"`
	Tables  []DropTable  `json:"tables"`
	Sources []DropSource `json:"sources"`

	AchCategories []AchCategory `json:"achCategories"`
	Achievements  []Achievement `json:"achievements"`

	ItemByPath  map[string]int `json:"-"`
	SourceByID  map[string]int `json:"-"`
	ItemSources map[int][]int  `json:"-"` // item index -> indexes of the sources whose table has it
	BossNames   []string       `json:"-"` // sorted, without repeats
	VendorNames []string       `json:"-"` // sorted, without repeats

	PlaceByID    map[string]int `json:"-"`
	PlaceSources map[int][]int  `json:"-"` // place index -> indexes of the sources found there
}

// AchievementIcon is an achievement's artwork, or its first reward item's icon when it has none of its own.
func (d *Drops) AchievementIcon(id int) string {
	for _, a := range d.Achievements {
		if a.ID != id {
			continue
		}
		if a.Icon != "" {
			return a.Icon
		}
		if len(a.Items) > 0 {
			return d.Items[a.Items[0]].Icon
		}
	}
	return ""
}

// LoadDrops reads the embedded drop data.
func LoadDrops() (*Drops, error) { return ParseDrops(rawDrops) }

// ParseDrops reads gzip-compressed drop data and builds its indexes.
func ParseDrops(gz []byte) (*Drops, error) {
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var d Drops
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, err
	}
	d.ItemByPath = make(map[string]int, len(d.Items))
	for i, it := range d.Items {
		d.ItemByPath[it.Path] = i
	}
	d.SourceByID = make(map[string]int, len(d.Sources))
	for i, s := range d.Sources {
		d.SourceByID[s.ID] = i
	}
	d.PlaceByID = make(map[string]int, len(d.Places))
	for i, p := range d.Places {
		d.PlaceByID[p.ID] = i
	}
	d.PlaceSources = map[int][]int{}
	d.ItemSources = map[int][]int{}
	names := map[string]bool{}
	vendorSeen := map[string]bool{}
	for si, s := range d.Sources {
		seen := map[int]bool{}
		for _, row := range d.Tables[s.Table].Rows {
			if !seen[row.Item] {
				seen[row.Item] = true
				d.ItemSources[row.Item] = append(d.ItemSources[row.Item], si)
			}
		}
		if s.Kind == "boss" {
			names[s.Name] = true
		}
		if s.Kind == "vendor" && !vendorSeen[s.Name] {
			vendorSeen[s.Name] = true
			d.VendorNames = append(d.VendorNames, s.Name)
		}
		if s.Place >= 0 && s.Place < len(d.Places) {
			d.PlaceSources[s.Place] = append(d.PlaceSources[s.Place], si)
		}
	}
	for n := range names {
		d.BossNames = append(d.BossNames, n)
	}
	sort.Strings(d.BossNames)
	sort.Strings(d.VendorNames)
	return &d, nil
}
