package engine

import (
	"mholootfilter/internal/db"
	"mholootfilter/internal/patch"
)

// ItemFlags is what to hide for one item or group: Hide removes the model and glow, Name the name label.
type ItemFlags struct {
	Hide bool `json:"hide"`
	Name bool `json:"name"`
}

// Filter is what the user chose. Item keys come from ItemKey.
type Filter struct {
	Items    map[string]ItemFlags   `json:"items"`
	Looks    map[string]patch.Flags `json:"looks"` // item type -> every item drawn with it
	Groups   map[string]ItemFlags   `json:"groups"`
	Rarities map[string]bool        `json:"rarities"`
}

// ItemKey identifies an item: names repeat across types, so the type is part of the key.
func ItemKey(it db.Item) string { return it.Type + "|" + it.Name }

// Plan is the set of file changes that realises a Filter.
type Plan struct {
	Types     map[string]patch.Flags // item type -> references to clear in its package
	Retargets map[string]string      // prototype path -> sink key ("shown" or "hidden")
}

// Resolve works out the file changes for f. Items alone in their type are hidden through their type's package.
// Items sharing a type are pointed at a sink class instead, unless every item of that type is hidden.
func Resolve(d *db.DB, f Filter) Plan {
	p := Plan{Types: map[string]patch.Flags{}, Retargets: map[string]string{}}
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
		}
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
		p.Types[t] = patch.Flags{Glow: cur.Glow || add.Glow, Model: cur.Model || add.Model, Name: cur.Name || add.Name}
	}

	for t, items := range byType {
		flags := make([]ItemFlags, len(items))
		allHide, allName := true, true
		for i, it := range items {
			flags[i] = effective(it)
			allHide = allHide && flags[i].Hide
			allName = allName && flags[i].Name
		}
		merge(t, patch.Flags{Glow: allHide, Model: allHide, Name: allName})
		if allHide && allName {
			continue
		}
		for i, it := range items {
			fl := flags[i]
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
		merge(t, fl)
	}
	for _, sink := range p.Retargets {
		if sink == "hidden" {
			merge(d.Sinks["hidden"].Type, patch.Flags{Glow: true, Model: true, Name: true})
		} else {
			merge(d.Sinks["shown"].Type, patch.Flags{Glow: true, Model: true})
		}
	}
	return p
}
