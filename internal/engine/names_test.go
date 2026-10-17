package engine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/patch"
	"mholootfilter/internal/sip"
)

func readCalligraphy(t *testing.T, e *Engine) *sip.Pak {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip"))
	p, err := sip.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNameColoursComeFromItemsAndTheirGroups(t *testing.T) {
	d := loadDB(t)
	var a, b db.Item
	for _, it := range d.Items {
		if len(it.Groups) > 0 && a.Name == "" {
			a = it
		}
	}
	for _, it := range d.Items {
		if len(it.Groups) > 0 && it.Groups[0] == a.Groups[0] && it.Name != a.Name {
			b = it
			break
		}
	}
	got := NameColours(d, Filter{Items: map[string]ItemFlags{ItemKey(a): {Color: "red"}},
		Groups: map[string]ItemFlags{a.Groups[0]: {Color: "cyan"}}})
	if !slices.Contains(got["red"], a.Name) || slices.Contains(got["cyan"], a.Name) || !slices.Contains(got["cyan"], b.Name) {
		t.Fatalf("item colour must win over its group's: %v / %v", got["red"], len(got["cyan"]))
	}
	if got := NameColours(d, Filter{Items: map[string]ItemFlags{ItemKey(a): {Color: "plaid"}}}); len(got) != 0 {
		t.Fatalf("an unknown colour must be ignored: %v", got)
	}
	if got := NameColours(d, Filter{Items: map[string]ItemFlags{ItemKey(a): {Color: "#12ab34"}}}); !slices.Contains(got["#12ab34"], a.Name) {
		t.Fatalf("a hex colour must be kept: %v", got)
	}
}

func TestNameColourIsWrittenIntoMarvelGameOnly(t *testing.T) {
	e, mgPath, exePath := setupRarity(t, true)
	mgOrig, exeOrig := sha(t, mgPath), sha(t, exePath)
	it := e.DB.Items[0]
	f := Filter{Items: map[string]ItemFlags{ItemKey(it): {Color: "orange"}}}
	if r, err := e.Apply(f); err != nil || len(r.Warnings) != 0 || r.Changed != 2 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	if sha(t, mgPath) == mgOrig || sha(t, exePath) == exeOrig {
		t.Fatal("MarvelGame.upk and the exe hash were not written")
	}
	if r, err := e.Apply(f); err != nil || r.Changed != 0 {
		t.Fatalf("re-apply: %v %+v", err, r)
	}
	if r, err := e.Apply(Filter{}); err != nil || len(r.Warnings) != 0 || sha(t, mgPath) != mgOrig || sha(t, exePath) != exeOrig {
		t.Fatalf("no colour left should restore both files: %v %+v", err, r)
	}
}

func TestNameSizeTweakChangesTheGroundNameStyle(t *testing.T) {
	e := setupRetarget(t, t.TempDir())
	ns := e.DB.NameStyle
	cal := filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip")
	orig := sha(t, cal)
	if err := e.SetTweaks(patch.Tweaks{NameSize: 22}); err != nil {
		t.Fatal(err)
	}
	if r, err := e.Apply(Filter{}); err != nil || len(r.Warnings) != 0 || r.Changed != 1 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	b, _ := readCalligraphy(t, e).Read(ns.Title)
	if v, _ := sip.Value(b, ns.FontSizeField); v != 22 {
		t.Fatalf("ground name size %d, want 22", v)
	}
	e.SetTweaks(patch.Tweaks{})
	if r, err := e.Apply(Filter{}); err != nil || len(r.Warnings) != 0 || sha(t, cal) != orig {
		t.Fatalf("switching it off should restore Calligraphy.sip: %v %+v", err, r)
	}
	if err := e.SetTweaks(patch.Tweaks{NameSize: 99}); err == nil {
		t.Fatal("a name size the app does not offer must be refused")
	}
}
