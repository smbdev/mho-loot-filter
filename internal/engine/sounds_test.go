package engine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/patch"
	"mholootfilter/internal/wwise"
)

// hasEvent reports whether any bank of the file package at path holds an event with id.
func hasEvent(t *testing.T, path string, id uint32) bool {
	t.Helper()
	pck, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	banks, err := wwise.Banks(pck)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range banks {
		bank, _ := wwise.Bank(pck, b)
		objs, err := wwise.Objects(bank)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(objs, func(o wwise.Object) bool { return o.Type == wwise.HircEvent && o.ID == id }) {
			return true
		}
	}
	return false
}

func TestMutedSoundsLoseTheirEventAndComeBackExactly(t *testing.T) {
	e, _, _ := setup(t)
	pck := testdata(t, patch.SoundPackage)
	cooked := filepath.Join(e.GameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	shared, other := filepath.Join(cooked, patch.SoundPackage), filepath.Join(cooked, "SFX_Other_INT.pck")
	os.WriteFile(shared, pck, 0o644)
	os.WriteFile(other, pck, 0o644) // stands in for the large packages, which get no backup
	sounds, err := db.LoadSounds()
	if err != nil {
		t.Fatal(err)
	}
	const steps = "play_footstepgroundtypevsfootmaterial"
	i := slices.IndexFunc(sounds.Sounds, func(s db.Sound) bool { return s.Name == steps })
	if i < 0 || sounds.Sounds[i].At[0].File != patch.SoundPackage {
		t.Fatalf("footsteps are not in %s: %+v", patch.SoundPackage, sounds.Sounds[i])
	}
	at := sounds.Sounds[i].At[0]
	e.Sounds = &db.Sounds{MuteXor: sounds.MuteXor, Files: map[string]string{patch.SoundPackage: patch.Sha1Hex(pck), "SFX_Other_INT.pck": patch.Sha1Hex(pck)},
		Sounds: []db.Sound{{Name: steps, At: []db.SoundAt{at, {File: "SFX_Other_INT.pck", Bank: at.Bank, Offset: at.Offset}}}}}
	e.DB.SoundPackageSha1 = patch.Sha1Hex(pck)
	id := wwise.ShortID(steps)

	if err := e.SetMutes([]string{"play_nothing"}); err == nil {
		t.Fatal("unknown sound accepted")
	}
	if err := e.SetMutes([]string{steps}); err != nil {
		t.Fatal(err)
	}
	r, err := e.Apply(Filter{})
	if err != nil || len(r.Warnings) != 0 || r.Changed != 2 {
		t.Fatalf("mute: %v %+v", err, r)
	}
	for _, p := range []string{shared, other} {
		if hasEvent(t, p, id) || !hasEvent(t, p, id^sounds.MuteXor) {
			t.Fatalf("%s: footsteps still play", filepath.Base(p))
		}
	}
	if r, err := e.Apply(Filter{}); err != nil || r.Changed != 0 {
		t.Fatalf("a second apply rewrote files: %v %+v", err, r)
	}

	e.SetMutes(nil)
	if r, err := e.Apply(Filter{}); err != nil || len(r.Warnings) != 0 || r.Changed != 2 {
		t.Fatalf("unmute: %v %+v", err, r)
	}
	for _, p := range []string{shared, other} {
		if sha(t, p) != patch.Sha1Hex(pck) {
			t.Fatalf("%s: not the original after unmuting", filepath.Base(p))
		}
	}

	e.SetMutes([]string{steps})
	e.Apply(Filter{})
	os.WriteFile(other, append(pck[:len(pck):len(pck)], 0), 0o644) // a game update replaced it
	if r, err := e.RestoreAll(); err != nil || len(r.Warnings) != 1 || sha(t, shared) != patch.Sha1Hex(pck) {
		t.Fatalf("restore: %v %+v", err, r)
	}
	if m, _ := e.Mutes(); len(m) != 0 {
		t.Fatalf("restore kept mutes: %v", m)
	}
}

func TestMutesWorkInTheBankTheAlertRebuilds(t *testing.T) {
	e, _, _ := setup(t)
	pck := testdata(t, patch.SoundPackage)
	shared := filepath.Join(e.GameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole", patch.SoundPackage)
	os.WriteFile(shared, pck, 0o644)
	sounds, err := db.LoadSounds()
	if err != nil {
		t.Fatal(err)
	}
	e.Sounds = sounds
	e.DB.SoundPackageSha1 = patch.Sha1Hex(pck)
	const pickup = "play_sfx_loot_pickup_runestone" // in the item sound bank, which the alert rebuilds
	if err := e.SetMutes([]string{pickup}); err != nil {
		t.Fatal(err)
	}
	r, err := e.Apply(Filter{RaritySounds: map[string]bool{"Cosmic": true}})
	if err != nil || len(r.Warnings) != 0 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	if id := wwise.ShortID(pickup); hasEvent(t, shared, id) || !hasEvent(t, shared, id^sounds.MuteXor) {
		t.Fatal("the pickup sound still plays")
	}
}
