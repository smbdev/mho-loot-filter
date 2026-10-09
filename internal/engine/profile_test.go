package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func names(p Profiles) []string {
	var n []string
	for _, pr := range p.List {
		n = append(n, pr.Name)
	}
	return n
}

func TestProfilesKeepTheirOwnFilterAndSound(t *testing.T) {
	e := &Engine{DB: loadDB(t), DataDir: t.TempDir()}
	// A filter and sound from before profiles become the first profile.
	if err := e.SaveFilter(Filter{Rarities: map[string]bool{"Common": true}}); err != nil {
		t.Fatal(err)
	}
	if err := e.SetAlert("ding.wav", []int16{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	p, err := e.Profiles()
	if err != nil || len(p.List) != 1 || p.List[0].Name != "My filter" || p.Active != p.List[0].ID {
		t.Fatalf("first profile: %+v %v", p, err)
	}
	first := p.Active

	if p, err = e.AddProfile(false); err != nil || len(p.List) != 2 || p.Active == first || p.List[1].Name != "Profile 2" {
		t.Fatalf("new profile: %+v %v", p, err)
	}
	if f, _ := e.LoadFilter(); len(f.Rarities) != 0 {
		t.Fatal("a new profile starts empty")
	}
	if a, _ := e.Alert(); a.Name != "" {
		t.Fatal("a new profile uses the built-in sound")
	}
	if err := e.SaveFilter(Filter{Rarities: map[string]bool{"Epic": true}}); err != nil {
		t.Fatal(err)
	}

	if p, err = e.UseProfile(first); err != nil || p.Active != first {
		t.Fatalf("switch back: %+v %v", p, err)
	}
	if f, _ := e.LoadFilter(); !f.Rarities["Common"] || f.Rarities["Epic"] {
		t.Fatalf("first profile's filter changed: %+v", f.Rarities)
	}
	if a, _ := e.Alert(); a.Name != "ding.wav" {
		t.Fatalf("first profile's sound lost: %q", a.Name)
	}

	if p, err = e.AddProfile(true); err != nil || p.List[2].Name != "My filter copy" {
		t.Fatalf("copy: %+v %v", p, err)
	}
	copied := p.Active
	if f, _ := e.LoadFilter(); !f.Rarities["Common"] {
		t.Fatal("a copy keeps the filter")
	}
	if a, _ := e.Alert(); a.Name != "ding.wav" {
		t.Fatal("a copy keeps the sound")
	}

	if _, err = e.RenameProfile(copied, " profile 2 "); err == nil {
		t.Fatal("names must differ")
	}
	if _, err = e.RenameProfile(copied, "  "); err == nil {
		t.Fatal("names cannot be empty")
	}
	if p, err = e.RenameProfile(copied, " Holo-Sim "); err != nil || p.List[2].Name != "Holo-Sim" {
		t.Fatalf("rename: %+v %v", p, err)
	}

	// Deleting the first profile removes its files but keeps the rest of the data folder.
	os.MkdirAll(filepath.Join(e.DataDir, "games"), 0o755)
	if p, err = e.DeleteProfile(first); err != nil || len(p.List) != 2 || p.Active != copied {
		t.Fatalf("delete first: %+v %v", p, err)
	}
	for _, f := range []string{"filter.json", "alert.wem", "alert.json"} {
		if _, err := os.Stat(filepath.Join(e.DataDir, f)); err == nil {
			t.Fatalf("%s of the deleted profile left behind", f)
		}
	}
	if _, err := os.Stat(filepath.Join(e.DataDir, "games")); err != nil {
		t.Fatal("deleting a profile must keep the backups")
	}
	// The active profile goes: the first one left takes over.
	if p, err = e.DeleteProfile(copied); err != nil || len(p.List) != 1 || p.List[0].Name != "Profile 2" || p.Active != p.List[0].ID {
		t.Fatalf("delete active: %+v %v", p, err)
	}
	if f, _ := e.LoadFilter(); !f.Rarities["Epic"] {
		t.Fatal("the profile left is in use")
	}
	if _, err = e.DeleteProfile(p.Active); err == nil {
		t.Fatal("the last profile cannot be deleted")
	}
	if _, err = e.UseProfile(99); err == nil {
		t.Fatal("unknown profile")
	}
	if p, _ = e.AddProfile(false); names(p)[1] != "Profile 3" {
		t.Fatalf("new names must not clash: %v", names(p))
	}
}
