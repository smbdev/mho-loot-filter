package engine

import (
	"slices"
	"strings"

	"mholootfilter/internal/db"
	"mholootfilter/internal/patch"
)

// ItemFlags is what to change for one item or group: Hide removes the model and glow, Name the name label,
// and Sound plays the alert when it drops.
type ItemFlags struct {
	Hide  bool `json:"hide"`
	Name  bool `json:"name"`
	Sound bool `json:"sound"`
}

// Filter is what the user chose. Item keys come from ItemKey.
type Filter struct {
	Items        map[string]ItemFlags   `json:"items"`
	Looks        map[string]patch.Flags `json:"looks"` // item type -> every item drawn with it
	Groups       map[string]ItemFlags   `json:"groups"`
	Rarities     map[string]bool        `json:"rarities"`     // rarity -> glow hidden
	RaritySounds map[string]bool        `json:"raritySounds"` // rarity -> alert played (Cosmic and Unique only)
	RarityHide   map[string][]string    `json:"rarityHide"`   // category -> rarities whose items are hidden
}

// ItemKey identifies an item: names repeat across types, so the type is part of the key.
func ItemKey(it db.Item) string { return it.Type + "|" + it.Name }

// Plan is the set of file changes that realises a Filter.
type Plan struct {
	Types     map[string]patch.Flags // item type -> changes to its package
	Clones    map[string]patch.Flags // clone id (see CloneSource) -> changes to a copy of its class
	Retargets map[string]string      // prototype path -> sink key ("shown" or "hidden"), or the type whose copy it uses
	// Unclickable holds the prototypes of hidden items: the game picks items by their bounds, not their model, so
	// these get bounds that can never be clicked.
	Unclickable map[string]bool
}

// Resolve works out the file changes for f. Items alone in their type are changed through their type's package.
// Items sharing a type are pointed at a sink class (hidden) or at a copy of their class (alert), unless every item
// of that type gets the same change.
func Resolve(d *db.DB, f Filter) Plan {
	p := Plan{Types: map[string]patch.Flags{}, Clones: map[string]patch.Flags{}, Retargets: map[string]string{},
		Unclickable: map[string]bool{}}
	byType := map[string][]db.Item{}
	for _, it := range d.Items {
		byType[it.Type] = append(byType[it.Type], it)
	}
	effective := func(it db.Item) ItemFlags {
		fl := f.Items[ItemKey(it)]
		for _, g := range it.Groups {
			gf := f.Groups[g]
			fl.Hide = fl.Hide || gf.Hide
			fl.Name = fl.Name || gf.Name
			fl.Sound = fl.Sound || gf.Sound
		}
		if f.Looks[it.Type].Model && len(d.Types[it.Type].Model) == 0 {
			fl.Hide = true // the look's model comes from a parent class, so each item is hidden through a sink
		}
		if r := scenarioRarity(it); r != "" && slices.Contains(f.RarityHide[DangerRoom], r) {
			fl.Hide, fl.Name = true, true
		}
		fl.Sound = fl.Sound && !fl.Hide // a hidden item is pointed at a sink, which has no alert
		if fl.Hide && f.Looks[it.Type].Name {
			fl.Name = true // names are hidden for the whole look, so keep this item's name hidden too
		}
		return fl
	}
	merge := func(t string, add patch.Flags) {
		if add.None() {
			return
		}
		cur := p.Types[t]
		p.Types[t] = patch.Flags{Glow: cur.Glow || add.Glow, Model: cur.Model || add.Model, Name: cur.Name || add.Name,
			Sound: cur.Sound || add.Sound}
	}

	for t, items := range byType {
		flags := make([]ItemFlags, len(items))
		allHide, allName, allSound, anySound := true, true, true, false
		for i, it := range items {
			flags[i] = effective(it)
			if flags[i].Hide || f.Looks[t].Model {
				for _, pr := range it.Protos {
					p.Unclickable[pr.Path] = true
				}
			}
			allHide = allHide && flags[i].Hide
			allName = allName && flags[i].Name
			allSound = allSound && (flags[i].Sound || flags[i].Hide)
			anySound = anySound || flags[i].Sound
		}
		allSound = allSound && anySound
		allHide = allHide && len(d.Types[t].Model) > 0 // without a model of its own, the package cannot hide them
		merge(t, patch.Flags{Glow: allHide, Model: allHide, Name: allName, Sound: allSound})
		if allHide && allName {
			continue
		}
		for i, it := range items {
			fl := flags[i]
			if fl.Sound && !allSound {
				for _, pr := range it.Protos {
					p.Retargets[pr.Path] = t
				}
				p.Clones[t] = patch.Flags{Sound: true}
				continue
			}
			if !fl.Hide && isPortal(it) && len(f.RarityHide[DangerRoom]) > 0 {
				for _, pr := range it.Protos {
					p.Retargets[pr.Path] = ScenarioClone
				}
				p.Clones[ScenarioClone] = patch.Flags{}
				continue
			}
			if !fl.Hide || (allHide && !fl.Name) {
				continue // visible, or already hidden by its type with the name still shown
			}
			sink := "shown"
			if fl.Name {
				sink = "hidden"
			}
			for _, pr := range it.Protos {
				p.Retargets[pr.Path] = sink
			}
		}
	}
	for t, fl := range f.Looks {
		fl.Glow = fl.Glow || fl.Model // hiding every item of a look removes their glow as well
		fl.Model = fl.Model && len(d.Types[t].Model) > 0
		merge(t, fl)
	}
	for _, target := range p.Retargets {
		switch target {
		case "hidden":
			merge(d.Sinks["hidden"].Type, patch.Flags{Glow: true, Model: true, Name: true})
		case "shown":
			merge(d.Sinks["shown"].Type, patch.Flags{Glow: true, Model: true})
		}
	}
	for id := range p.Clones {
		fl := p.Types[CloneSource(id)] // a copy looks like the items it stands in for
		fl.Sound = id != ScenarioClone
		p.Clones[id] = fl
	}
	return p
}

// ScenarioClone is the clone id of the class copy that Danger Room scenario portals use while the grid hides some of
// their rarities: they share the loot bag class with hundreds of other items, and MarvelGame.upk's rarity code tells
// them apart by this copy. Every other clone id is the item type it copies, for items that play the alert.
const ScenarioClone = "dangerroom"

const scenarioSource = "marvelitem_loot"

// CloneSource returns the item type a clone id copies.
func CloneSource(id string) string {
	if id == ScenarioClone {
		return scenarioSource
	}
	return id
}

// isPortal reports whether an item is a Danger Room scenario portal, whose rarity is decided when it drops.
func isPortal(it db.Item) bool {
	return it.Type == scenarioSource && slices.Contains(it.Groups, "dangerroom")
}

// scenarioRarity returns the rarity a Danger Room scenario crate is named for, such as Rare for "Danger Room Rare
// Scenario", or "" for other items. Each crate rarity is an item of its own.
func scenarioRarity(it db.Item) string {
	if !slices.Contains(it.Groups, "dangerroom") || isPortal(it) {
		return ""
	}
	for _, r := range []string{"Uncommon", "Common", "Rare", "Epic", "Cosmic", "Unique"} { // Uncommon before Common
		if strings.Contains(it.Name, r) {
			return r
		}
	}
	return ""
}
