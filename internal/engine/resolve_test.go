package engine

import (
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/patch"
)

func item(t *testing.T, d *db.DB, name string) db.Item {
	t.Helper()
	for _, it := range d.Items {
		if it.Name == name {
			return it
		}
	}
	t.Fatalf("no item %q", name)
	return db.Item{}
}

func loadDB(t *testing.T) *db.DB {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResolveSingleItemTypeUsesItsPackage(t *testing.T) {
	d := loadDB(t)
	relic := item(t, d, "Relic of Atlantis")
	p := Resolve(d, Filter{Items: map[string]ItemFlags{ItemKey(relic): {Hide: true}}})
	if p.Types[relic.Type] != (patch.Flags{Glow: true, Model: true}) || len(p.Retargets) != 0 {
		t.Fatalf("got %+v", p)
	}
	p = Resolve(d, Filter{Items: map[string]ItemFlags{ItemKey(relic): {Name: true}}})
	if p.Types[relic.Type] != (patch.Flags{Name: true}) {
		t.Fatalf("name only: got %+v", p.Types[relic.Type])
	}
}

func TestResolveSharedItemIsRetargeted(t *testing.T) {
	d := loadDB(t)
	axe := item(t, d, "Uru-Forged Battle Axe")
	p := Resolve(d, Filter{Items: map[string]ItemFlags{ItemKey(axe): {Hide: true}}})
	if _, touched := p.Types[axe.Type]; touched {
		t.Fatal("the shared Uru-Forged type must not change")
	}
	for _, pr := range axe.Protos {
		if p.Retargets[pr.Path] != "shown" {
			t.Fatalf("%s not retargeted to the shown sink: %+v", pr.Path, p.Retargets)
		}
	}
	if len(p.Retargets) != len(axe.Protos) || p.Types[d.Sinks["shown"].Type] != (patch.Flags{Glow: true, Model: true}) {
		t.Fatalf("got %+v", p)
	}
	p = Resolve(d, Filter{Items: map[string]ItemFlags{ItemKey(axe): {Hide: true, Name: true}}})
	if p.Retargets[axe.Protos[0].Path] != "hidden" || p.Types[d.Sinks["hidden"].Type] != (patch.Flags{Glow: true, Model: true, Name: true}) {
		t.Fatalf("hide with name: got %+v", p)
	}
}

func TestResolveWholeTypeHiddenUsesPackage(t *testing.T) {
	d := loadDB(t)
	axe := item(t, d, "Uru-Forged Battle Axe")
	p := Resolve(d, Filter{Groups: map[string]ItemFlags{"uru": {Hide: true}}})
	if p.Types[axe.Type] != (patch.Flags{Glow: true, Model: true}) {
		t.Fatalf("whole Uru-Forged type should be patched: %+v", p.Types[axe.Type])
	}
	for path := range p.Retargets {
		for _, pr := range axe.Protos {
			if pr.Path == path {
				t.Fatal("items of a fully hidden type should not be retargeted")
			}
		}
	}
}

func TestGroupAndItemCombine(t *testing.T) {
	d := loadDB(t)
	lizard := item(t, d, "Lizard Medallion")
	key := ItemKey(lizard)
	hidden := func(f Filter) bool {
		p := Resolve(d, f)
		return p.Retargets[lizard.Protos[0].Path] != "" || p.Types[lizard.Type].Model
	}
	if !hidden(Filter{Items: map[string]ItemFlags{key: {Hide: true}}, Groups: map[string]ItemFlags{"medallions": {}}}) {
		t.Fatal("item switched on, group off: should be hidden")
	}
	if !hidden(Filter{Groups: map[string]ItemFlags{"medallions": {Hide: true}}}) {
		t.Fatal("group on: should be hidden")
	}
	if hidden(Filter{Groups: map[string]ItemFlags{"relics": {Hide: true}}}) {
		t.Fatal("a group the medallion is not in must not hide it")
	}
}

func TestResolveLooksAndEmpty(t *testing.T) {
	d := loadDB(t)
	axe := item(t, d, "Uru-Forged Battle Axe")
	p := Resolve(d, Filter{Looks: map[string]patch.Flags{axe.Type: {Name: true}}})
	if p.Types[axe.Type] != (patch.Flags{Name: true}) {
		t.Fatalf("look group: %+v", p.Types)
	}
	if p := Resolve(d, Filter{}); len(p.Types) != 0 || len(p.Retargets) != 0 {
		t.Fatalf("empty filter should change nothing: %+v", p)
	}
}

func TestResolveNameOnlyOnSharedItemIsIgnored(t *testing.T) {
	d := loadDB(t)
	axe := item(t, d, "Uru-Forged Battle Axe")
	if p := Resolve(d, Filter{Items: map[string]ItemFlags{ItemKey(axe): {Name: true}}}); len(p.Types) != 0 || len(p.Retargets) != 0 {
		t.Fatalf("a shared look cannot hide one visible item's name: %+v", p)
	}
}

func TestHidingAnItemKeepsItsLookNameHidden(t *testing.T) {
	d := loadDB(t)
	axe := item(t, d, "Uru-Forged Battle Axe")
	p := Resolve(d, Filter{
		Items: map[string]ItemFlags{ItemKey(axe): {Hide: true}},
		Looks: map[string]patch.Flags{axe.Type: {Name: true}},
	})
	if p.Retargets[axe.Protos[0].Path] != "hidden" {
		t.Fatalf("names are hidden for the whole look, so the hidden axe must use the no-name sink: %+v", p.Retargets)
	}
}

func TestHideAllInLookAlsoHidesGlow(t *testing.T) {
	d := loadDB(t)
	axe := item(t, d, "Uru-Forged Battle Axe")
	p := Resolve(d, Filter{Looks: map[string]patch.Flags{axe.Type: {Model: true}}})
	if p.Types[axe.Type] != (patch.Flags{Glow: true, Model: true}) {
		t.Fatalf("hiding every item of a look removes model and glow: %+v", p.Types[axe.Type])
	}
}
