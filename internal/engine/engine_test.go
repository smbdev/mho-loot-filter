package engine

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mholootfilter/internal/assetcache"
	"mholootfilter/internal/db"
	"mholootfilter/internal/patch"
	"mholootfilter/internal/sip"
	"mholootfilter/internal/wwise"
)

const relic = "UC__MarvelItem_Loot_Origin_RelicCritDamage_SF.upk"

func setup(t *testing.T) (*Engine, string, string) {
	t.Helper()
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	game := t.TempDir()
	cooked := filepath.Join(game, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	os.MkdirAll(cooked, 0o755)
	os.MkdirAll(filepath.Join(game, "UnrealEngine3", "Binaries", "Win64"), 0o755)
	b := testdata(t, relic)
	os.WriteFile(filepath.Join(cooked, relic), b, 0o644)
	os.MkdirAll(filepath.Join(game, "Data", "Game"), 0o755)
	os.WriteFile(filepath.Join(game, "Data", "Game", "Calligraphy.sip"), testdata(t, "Calligraphy.sip"), 0o644)
	var key string
	for k, v := range d.Types {
		if v.File == relic {
			key = k
		}
	}
	// keep the test DB to the one fixture package
	d.Types = map[string]*db.Type{key: d.Types[key]}
	e := &Engine{DB: d, GameDir: game, DataDir: t.TempDir(), GameRunning: func() bool { return false }, SkipRarity: true}
	return e, filepath.Join(cooked, relic), key
}

func sha(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return patch.Sha1Hex(b)
}

func TestApplyThenRestore(t *testing.T) {
	e, file, key := setup(t)
	orig := sha(t, file)
	r, err := e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true, Name: true}}})
	if err != nil || r.Changed != 2 { // the package, and Calligraphy.sip to make the hidden item unclickable
		t.Fatalf("apply: %v %+v", err, r)
	}
	if sha(t, file) == orig {
		t.Fatal("file not patched")
	}
	if r, _ := e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true, Name: true}}}); r.Changed != 0 {
		t.Fatal("second apply should change nothing")
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, file) != orig {
		t.Fatalf("restore failed: %v", err)
	}
}

func TestApplySkipsUnknownFile(t *testing.T) {
	e, file, key := setup(t)
	os.WriteFile(file, []byte("modified by someone else"), 0o644)
	r, err := e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true}}})
	if err != nil || r.Changed != 0 || len(r.Warnings) != 1 {
		t.Fatalf("expected one warning and no change: %v %+v", err, r)
	}
	if _, err := os.Stat(e.backup(relic)); err == nil {
		t.Fatal("must not back up an unknown file")
	}
}

func TestApplyIsResumable(t *testing.T) {
	e, file, key := setup(t)
	orig := sha(t, file)
	f := Filter{Looks: map[string]patch.Flags{key: {Glow: true, Model: true}}}
	e.Apply(f)
	os.Remove(e.statePath()) // simulate crash before state was saved elsewhere
	os.WriteFile(e.statePath(), []byte("{}"), 0o644)
	// file is patched but state forgot it: apply must recognise it via the backup and still converge
	r, _ := e.Apply(f)
	if len(r.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", r.Warnings)
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, file) != orig {
		t.Fatal("restore after resumed apply failed")
	}
}

func TestRestoreReportsMissingBackup(t *testing.T) {
	e, _, key := setup(t)
	e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true}}})
	os.RemoveAll(filepath.Dir(e.backup(relic)))
	r, _ := e.RestoreAll()
	if len(r.Warnings) == 0 {
		t.Fatal("expected a warning telling the user to verify files in Steam")
	}
}

func TestApplyRefusesWhileGameRunning(t *testing.T) {
	e, _, key := setup(t)
	e.GameRunning = func() bool { return true }
	if _, err := e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true}}}); err == nil {
		t.Fatal("expected refusal while game is running")
	}
	if _, err := e.RestoreAll(); err == nil {
		t.Fatal("expected refusal while game is running")
	}
}

func TestExeOriginalRequiresOriginalHash(t *testing.T) {
	e, _, _ := setup(t)
	exe := filepath.Join(e.GameDir, "UnrealEngine3", "Binaries", "Win64", "MarvelHeroesOmega.exe")
	b := make([]byte, 48544080)
	os.WriteFile(exe, b, 0o644)
	if _, err := e.exeOriginalSha(); err == nil {
		t.Fatal("exe without MarvelGame's original hash must be rejected")
	}
	h, _ := hex.DecodeString(e.DB.MarvelGameSha1)
	copy(b[patch.ExeHashOffset:], h)
	os.WriteFile(exe, b, 0o644)
	if _, err := e.exeOriginalSha(); err != nil {
		t.Fatalf("original exe rejected: %v", err)
	}
}

func setupRarity(t *testing.T, exeOriginal bool) (*Engine, string, string) {
	t.Helper()
	e, _, _ := setup(t)
	mgPath, exePath := addRarityFiles(t, e, exeOriginal)
	return e, mgPath, exePath
}

// addRarityFiles gives e's game folder MarvelGame.upk and an exe whose stored hash is MarvelGame's when exeOriginal.
func addRarityFiles(t *testing.T, e *Engine, exeOriginal bool) (string, string) {
	t.Helper()
	e.SkipRarity = false
	os.MkdirAll(filepath.Join(e.GameDir, "UnrealEngine3", "Binaries", "Win64"), 0o755)
	mg := testdata(t, "MarvelGame.upk")
	mgPath := filepath.Join(e.GameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole", "MarvelGame.upk")
	os.WriteFile(mgPath, mg, 0o644)
	exe := make([]byte, 48544080)
	copy(exe[patch.ExeHashOffset-15:], "marvelgame.upk\x00")
	if exeOriginal {
		h, _ := hex.DecodeString(e.DB.MarvelGameSha1)
		copy(exe[patch.ExeHashOffset:], h)
	}
	exePath := filepath.Join(e.GameDir, "UnrealEngine3", "Binaries", "Win64", "MarvelHeroesOmega.exe")
	os.WriteFile(exePath, exe, 0o644)
	return mgPath, exePath
}

func TestRarityLeavesMarvelGameAloneWhenExeUnusable(t *testing.T) {
	e, mgPath, _ := setupRarity(t, false)
	orig := sha(t, mgPath)
	r, err := e.Apply(Filter{Rarities: map[string]bool{"Common": true}})
	if err != nil {
		t.Fatal(err)
	}
	if sha(t, mgPath) != orig {
		t.Fatal("MarvelGame.upk was patched although its hash could not be written to the exe")
	}
	if len(r.Warnings) == 0 {
		t.Fatal("expected a warning")
	}
}

func TestRarityPatchesMarvelGameAndExeTogether(t *testing.T) {
	e, mgPath, exePath := setupRarity(t, true)
	mgOrig, exeOrig := sha(t, mgPath), sha(t, exePath)
	r, err := e.Apply(Filter{Rarities: map[string]bool{"Common": true}})
	if err != nil || len(r.Warnings) != 0 || r.Changed != 2 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	mg, _ := os.ReadFile(mgPath)
	exe, _ := os.ReadFile(exePath)
	sum := sha1.Sum(mg)
	if !bytes.Equal(exe[patch.ExeHashOffset:patch.ExeHashOffset+20], sum[:]) {
		t.Fatal("exe does not hold the new MarvelGame.upk hash")
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, mgPath) != mgOrig || sha(t, exePath) != exeOrig {
		t.Fatalf("restore failed: %v", err)
	}
}

func TestRestoreAfterApplyLostItsState(t *testing.T) {
	e, file, key := setup(t)
	orig := sha(t, file)
	e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true}}})
	os.WriteFile(e.statePath(), []byte("{}"), 0o644) // crash before state was saved
	r, err := e.RestoreAll()
	if err != nil || sha(t, file) != orig {
		t.Fatalf("restore left the file patched: %v %+v", err, r)
	}
}

func TestInterruptedReapplyRecovers(t *testing.T) {
	e, file, key := setup(t)
	orig := sha(t, file)
	e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true}}})
	// a second apply wrote its file and crashed before saving state: state still holds the first result
	backup, _ := os.ReadFile(e.backup(relic))
	both, err := patch.Package(backup, e.DB.Types[key], patch.Flags{Model: true, Name: true})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, both, 0o644)
	r, err := e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true, Name: true}}})
	if err != nil || len(r.Warnings) != 0 {
		t.Fatalf("re-apply after crash: %v %+v", err, r)
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, file) != orig {
		t.Fatalf("restore failed: %v", err)
	}
}

func TestConcurrentRunsDoNotInterfere(t *testing.T) {
	e, file, key := setup(t)
	orig := sha(t, file)
	f := Filter{Looks: map[string]patch.Flags{key: {Model: true}}}
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, err := e.Apply(f); errs <- err }()
		go func() { defer wg.Done(); _, err := e.RestoreAll(); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent run failed: %v", err)
		}
	}
	if r, err := e.RestoreAll(); err != nil || len(r.Warnings) != 0 || sha(t, file) != orig {
		t.Fatalf("files damaged by concurrent runs: %v %+v", err, r)
	}
}

// setupRetarget builds a game folder with Calligraphy.sip and both sink packages, using the full item database.
func setupRetarget(t *testing.T, game string) *Engine {
	t.Helper()
	d := loadDB(t)
	cooked := filepath.Join(game, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	os.MkdirAll(cooked, 0o755)
	os.MkdirAll(filepath.Join(game, "Data", "Game"), 0o755)
	copyFixture := func(name, dst string) {
		os.WriteFile(dst, testdata(t, name), 0o644)
	}
	copyFixture("Calligraphy.sip", filepath.Join(game, "Data", "Game", "Calligraphy.sip"))
	for _, s := range []string{"shown", "hidden"} {
		f := d.Types[d.Sinks[s].Type].File
		copyFixture(f, filepath.Join(cooked, f))
	}
	return &Engine{DB: d, GameDir: game, DataDir: t.TempDir(), GameRunning: func() bool { return false }, SkipRarity: true}
}

func axeClass(t *testing.T, e *Engine) uint64 {
	t.Helper()
	raw, _ := os.ReadFile(filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip"))
	p, err := sip.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	axe := item(t, e.DB, "Uru-Forged Battle Axe")
	b, _ := p.Read(axe.Protos[0].Path)
	fields := map[uint64]bool{}
	for _, f := range e.DB.UnrealClassFields {
		fields[f] = true
	}
	c, _ := sip.UnrealClass(b, fields)
	return c
}

func TestRetargetApplyAndRestore(t *testing.T) {
	e := setupRetarget(t, t.TempDir())
	axe := item(t, e.DB, "Uru-Forged Battle Axe")
	cal := filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip")
	shownPkg := filepath.Join(e.GameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole", e.DB.Types[e.DB.Sinks["shown"].Type].File)
	calOrig, sinkOrig := sha(t, cal), sha(t, shownPkg)
	r, err := e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(axe): {Hide: true}}})
	if err != nil || len(r.Warnings) != 0 || r.Changed != 2 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	if axeClass(t, e) != e.DB.Sinks["shown"].Asset || sha(t, shownPkg) == sinkOrig {
		t.Fatal("axe not retargeted or sink not patched")
	}
	if r, _ := e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(axe): {Hide: true}}}); r.Changed != 0 {
		t.Fatalf("re-apply changed %d files", r.Changed)
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, cal) != calOrig || sha(t, shownPkg) != sinkOrig {
		t.Fatalf("restore: %v", err)
	}
}

func TestRestoreCalligraphyAndSinks(t *testing.T) {
	e := setupRetarget(t, t.TempDir())
	axe := item(t, e.DB, "Uru-Forged Battle Axe")
	cal := filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip")
	calOrig := sha(t, cal)
	e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(axe): {Hide: true, Name: true}}})
	if r, err := e.Apply(Filter{}); err != nil || len(r.Warnings) != 0 || sha(t, cal) != calOrig {
		t.Fatalf("unhiding the last item should restore Calligraphy.sip: %v %+v", err, r)
	}
	for _, s := range []string{"shown", "hidden"} {
		f := e.DB.Types[e.DB.Sinks[s].Type].File
		orig := testdata(t, f)
		if sha(t, filepath.Join(e.GameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole", f)) != patch.Sha1Hex(orig) {
			t.Fatalf("sink %s not restored", f)
		}
	}
}

func TestCalligraphyChangedElsewhereIsSkipped(t *testing.T) {
	e := setupRetarget(t, t.TempDir())
	cal := filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip")
	b, _ := os.ReadFile(cal)
	p, _ := sip.Open(b)
	name := p.Names()[0] // not an item prototype
	data, _ := p.Read(name)
	data[len(data)-1] ^= 1
	p.Replace(name, data)
	os.WriteFile(cal, p.Bytes(), 0o644)
	axe := item(t, e.DB, "Uru-Forged Battle Axe")
	if r, _ := e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(axe): {Hide: true}}}); len(r.Warnings) == 0 {
		t.Fatal("a Calligraphy.sip changed by something else must be left alone")
	}
}

func TestChangeGameDirKeepsStatePerFolder(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	e := setupRetarget(t, a)
	setupRetarget(t, b)
	for _, dir := range []string{a, b} {
		exe := filepath.Join(dir, "UnrealEngine3", "Binaries", "Win64")
		os.MkdirAll(exe, 0o755)
		os.WriteFile(filepath.Join(exe, "MarvelHeroesOmega.exe"), []byte("x"), 0o644)
		mg := testdata(t, "MarvelGame.upk")
		os.WriteFile(filepath.Join(dir, "UnrealEngine3", "MarvelGame", "CookedPCConsole", "MarvelGame.upk"), mg, 0o644)
	}
	axe := item(t, e.DB, "Uru-Forged Battle Axe")
	calA, calB := filepath.Join(a, "Data", "Game", "Calligraphy.sip"), filepath.Join(b, "Data", "Game", "Calligraphy.sip")
	orig := sha(t, calA)
	hide := Filter{Items: map[string]ItemFlags{ItemKey(axe): {Hide: true}}}
	e.Apply(hide)
	if err := e.SetGameDir(b); err != nil {
		t.Fatal(err)
	}
	e.Apply(hide)
	if err := e.SetGameDir(a); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, calA) != orig {
		t.Fatalf("folder A not restored: %v", err)
	}
	if sha(t, calB) == orig {
		t.Fatal("restoring folder A must not touch folder B")
	}
	if err := e.SetGameDir(t.TempDir()); err == nil {
		t.Fatal("a folder without the game must be rejected")
	}
}

func TestVersionOneDataIsMigrated(t *testing.T) {
	e, file, key := setup(t)
	orig := sha(t, file)
	backup, _ := os.ReadFile(file)
	patched, _ := patch.Package(backup, e.DB.Types[key], patch.Flags{Model: true})
	os.WriteFile(file, patched, 0o644)
	os.MkdirAll(filepath.Join(e.DataDir, "backup"), 0o755)
	os.WriteFile(filepath.Join(e.DataDir, "backup", relic), backup, 0o644)
	os.WriteFile(filepath.Join(e.DataDir, "state.json"), []byte(`{"UnrealEngine3/MarvelGame/CookedPCConsole/`+relic+`":"`+patch.Sha1Hex(patched)+`"}`), 0o644)
	os.WriteFile(filepath.Join(e.DataDir, "filter.json"), []byte(`{"types":{"`+key+`":{"Glow":false,"Model":true,"Name":false}},"rarities":{}}`), 0o644)
	f, err := e.LoadFilter()
	if err != nil || !f.Looks[key].Model {
		t.Fatalf("version 1 type settings should load as look settings: %v %+v", err, f)
	}
	if r, err := e.RestoreAll(); err != nil || len(r.Warnings) != 0 || sha(t, file) != orig {
		t.Fatalf("files patched by version 1 must be restorable: %v %+v", err, r)
	}
}

func TestRestoreClearsTheFilter(t *testing.T) {
	e, _, key := setup(t)
	e.Apply(Filter{Looks: map[string]patch.Flags{key: {Model: true}}, Rarities: map[string]bool{"Common": true}})
	if _, err := e.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	f, err := e.LoadFilter()
	if err != nil || len(f.Looks) != 0 || len(f.Rarities) != 0 || len(f.Items) != 0 || len(f.Groups) != 0 {
		t.Fatalf("restore should empty the filter: %v %+v", err, f)
	}
}

// setupSound adds what the alert needs to a retarget game folder: the X-Men insignia package, the drop sound
// package and the asset package cache.
func setupSound(t *testing.T) (*Engine, string) {
	t.Helper()
	e := setupRetarget(t, t.TempDir())
	cooked := filepath.Join(e.GameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	for _, f := range []string{"UC__MarvelItem_Insignia_XMen_SF.upk", patch.SoundPackage, assetcache.File} {
		os.WriteFile(filepath.Join(cooked, f), testdata(t, f), 0o644)
	}
	return e, cooked
}

func TestSoundOnSharedItemApplyAndRestore(t *testing.T) {
	e, cooked := setupSound(t)
	var cyclops db.Item
	for _, it := range e.DB.Items {
		if it.Type == "marvelitem_insignia_xmen" {
			cyclops = it
			break
		}
	}
	cal := filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip")
	files := []string{cal, filepath.Join(cooked, patch.SoundPackage), filepath.Join(cooked, assetcache.File),
		filepath.Join(cooked, "UC__MarvelItem_Insignia_XMen_SF.upk")}
	before := map[string]string{}
	for _, f := range files {
		before[f] = sha(t, f)
	}

	f := Filter{Items: map[string]ItemFlags{ItemKey(cyclops): {Sound: true}}}
	r, err := e.Apply(f)
	if err != nil || len(r.Warnings) != 0 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	copies, _ := filepath.Glob(filepath.Join(cooked, "UC__MarvelItem_LF*_SF.upk"))
	if len(copies) != 1 {
		t.Fatalf("want one class copy, got %v", copies)
	}
	name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(copies[0]), "UC__"), "_SF.upk")
	id, _ := patch.CloneIDs(name)
	raw, _ := os.ReadFile(cal)
	p, _ := sip.Open(raw)
	fields := map[uint64]bool{}
	for _, f := range e.DB.UnrealClassFields {
		fields[f] = true
	}
	proto, _ := p.Read(cyclops.Protos[0].Path)
	if c, _ := sip.UnrealClass(proto, fields); c != id {
		t.Fatalf("item points at %d, want the copy %d", c, id)
	}
	list, _ := p.Read(sip.UnrealClassTypes)
	if !bytes.Contains(list, []byte(name)) {
		t.Fatal("copy not registered as a class")
	}
	cache, _ := os.ReadFile(filepath.Join(cooked, assetcache.File))
	if pk, err := assetcache.Packages(cache, "marvelgameitems."+name); err != nil || pk[len(pk)-1] != "uc__"+strings.ToLower(name)+"_sf" {
		t.Fatalf("copy not in the asset package cache: %v %v", pk, err)
	}
	if sha(t, filepath.Join(cooked, patch.SoundPackage)) == before[filepath.Join(cooked, patch.SoundPackage)] ||
		sha(t, filepath.Join(cooked, "UC__MarvelItem_Insignia_XMen_SF.upk")) != before[filepath.Join(cooked, "UC__MarvelItem_Insignia_XMen_SF.upk")] {
		t.Fatal("the sound package must change and the shared insignia package must not")
	}
	if r, _ := e.Apply(f); r.Changed != 0 {
		t.Fatalf("re-apply changed %d files", r.Changed)
	}

	if _, err := e.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if sha(t, f) != before[f] {
			t.Fatalf("%s not restored", filepath.Base(f))
		}
	}
	if _, err := os.Stat(copies[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("class copy not removed on restore")
	}
}

func TestRaritySoundOnlyTouchesTheSoundPackage(t *testing.T) {
	e, cooked := setupSound(t)
	pck := filepath.Join(cooked, patch.SoundPackage)
	orig := sha(t, pck)
	r, err := e.Apply(Filter{RaritySounds: map[string]bool{"Cosmic": true}})
	if err != nil || r.Changed != 1 || sha(t, pck) == orig {
		t.Fatalf("apply: %v %+v", err, r)
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, pck) != orig {
		t.Fatalf("restore: %v", err)
	}
}

func TestCustomAlertIsUsedAndCanBeReset(t *testing.T) {
	e, cooked := setupSound(t)
	if _, name := e.Alert(); name != "" {
		t.Fatal("built-in sound has no file name")
	}
	if err := e.SetAlert("big.mp3", make([]int16, MaxAlertSeconds*44100+1)); err == nil {
		t.Fatal("expected error for a sound that is too long")
	}
	samples := []int16{0, 1000, -2000, 500}
	if err := e.SetAlert("ding.wav", samples); err != nil {
		t.Fatal(err)
	}
	wem, name := e.Alert()
	if name != "ding.wav" || !bytes.Equal(wem, wwise.PCM(samples)) {
		t.Fatalf("custom alert not stored: %q", name)
	}
	if _, err := e.Apply(Filter{RaritySounds: map[string]bool{"Cosmic": true}}); err != nil {
		t.Fatal(err)
	}
	pck, _ := os.ReadFile(filepath.Join(cooked, patch.SoundPackage))
	bank, _ := wwise.Bank(pck, patch.ItemSoundBank)
	if !bytes.Contains(bank, wem) || bytes.Contains(bank, wwise.Alert[64:4096]) {
		t.Fatal("the custom alert must replace the built-in one in the game")
	}
	if err := e.SetAlert("", nil); err != nil {
		t.Fatal(err)
	}
	if wem, name := e.Alert(); name != "" || !bytes.Equal(wem, wwise.Alert) {
		t.Fatal("built-in alert not restored")
	}
}

func TestRetargetKeepsInheritingItemsOnTheirClass(t *testing.T) {
	e := setupRetarget(t, t.TempDir())
	flag := item(t, e.DB, "Flag of the Skrull Empire")
	var pin db.Pin
	for _, p := range flag.Protos {
		if len(p.Inheritors) > 0 {
			pin = p.Inheritors[0]
		}
	}
	if pin.Path == "" {
		t.Fatal("the Skrull flag has an item that inherits its class")
	}
	if _, err := e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(flag): {Hide: true}}}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip"))
	p, _ := sip.Open(raw)
	fields := map[uint64]bool{}
	for _, f := range e.DB.UnrealClassFields {
		fields[f] = true
	}
	child, _ := p.Read(pin.Path)
	if c, ok := sip.UnrealClass(child, fields); !ok || c != pin.Asset {
		t.Fatalf("%s now has class %d (set %v), want its own %d", pin.Path, c, ok, pin.Asset)
	}
	if r, err := e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(flag): {Hide: true}}}); err != nil || len(r.Warnings) != 0 || r.Changed != 0 {
		t.Fatalf("re-apply: %v %+v", err, r)
	}
}

func TestHiddenItemsBecomeUnclickable(t *testing.T) {
	e := setupRetarget(t, t.TempDir())
	qs, thor := item(t, e.DB, "Insignia of Quicksilver"), item(t, e.DB, "Insignia of Thor")
	cal := filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip")
	orig := sha(t, cal)
	if r, err := e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(qs): {Hide: true, Name: true}}}); err != nil || len(r.Warnings) != 0 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	raw, _ := os.ReadFile(cal)
	p, _ := sip.Open(raw)
	flag := le64(e.DB.Picking.FlagField)
	read := func(path string) []byte { b, _ := p.Read(path); return b }
	if !bytes.Contains(read(qs.Protos[0].Path), flag) || bytes.Contains(read(thor.Protos[0].Path), flag) {
		t.Fatal("only the hidden insignia may carry ComplexPickingOnly")
	}
	if r, _ := e.Apply(Filter{Items: map[string]ItemFlags{ItemKey(qs): {Hide: true, Name: true}}}); r.Changed != 0 {
		t.Fatalf("re-apply changed %d files", r.Changed)
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, cal) != orig {
		t.Fatalf("restore: %v", err)
	}
}

func le64(v uint64) []byte {
	b := make([]byte, 8)
	for i := range b {
		b[i] = byte(v >> (8 * i))
	}
	return b
}

func TestRarityHideRewritesMarvelGameAndExe(t *testing.T) {
	e, mgPath, exePath := setupRarity(t, true)
	for _, ty := range e.DB.Types { // the fixture's one item class stands in for the medallions
		ty.Category = "Medallions"
	}
	mgOrig, exeOrig := sha(t, mgPath), sha(t, exePath)
	r, err := e.Apply(Filter{RarityHide: map[string][]string{"Medallions": {"Common", "Rare"}}})
	if err != nil || len(r.Warnings) != 0 {
		t.Fatalf("apply: %v %+v", err, r)
	}
	mg, _ := os.ReadFile(mgPath)
	exe, _ := os.ReadFile(exePath)
	sum := sha1.Sum(mg)
	if sha(t, mgPath) == mgOrig || !bytes.Equal(exe[patch.ExeHashOffset:patch.ExeHashOffset+20], sum[:]) {
		t.Fatal("MarvelGame.upk and the hash in the exe must change together")
	}
	if r, _ := e.Apply(Filter{RarityHide: map[string][]string{"Medallions": {"Common", "Rare"}}}); r.Changed != 0 {
		t.Fatalf("re-apply changed %d files", r.Changed)
	}
	if _, err := e.RestoreAll(); err != nil || sha(t, mgPath) != mgOrig || sha(t, exePath) != exeOrig {
		t.Fatalf("restore: %v", err)
	}
}

func TestDangerRoomRowRuleMatchesTheScenarioCopy(t *testing.T) {
	e, cooked := setupSound(t)
	os.WriteFile(filepath.Join(cooked, "UC__MarvelItem_Loot_SF.upk"), testdata(t, "UC__MarvelItem_Loot_SF.upk"), 0o644)
	f := Filter{RarityHide: map[string][]string{DangerRoom: {"Common", "Rare"}}}
	plan := Resolve(e.DB, f)
	state := map[string]string{}
	clones, err := e.syncClones(plan, state, &Report{})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := clones[ScenarioClone]
	if !ok {
		t.Fatal("scenario copy not written")
	}
	rules := e.rarityRules(f, clones)
	if len(rules) != 1 || len(rules[0].Classes) != 1 || rules[0].Classes[0] != strings.ToLower(c.name) || len(rules[0].Rarities) != 2 {
		t.Fatalf("rule should match only the scenario copy: %+v", rules)
	}
}

func TestRarityRowsAreClickedByMeshOnlyWithTheRarityCode(t *testing.T) {
	for _, usable := range []bool{true, false} {
		e := setupRetarget(t, t.TempDir())
		addRarityFiles(t, e, usable)
		qs := item(t, e.DB, "Insignia of Quicksilver")
		if _, err := e.Apply(Filter{RarityHide: map[string][]string{"Insignias": {"Rare"}}}); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(e.GameDir, "Data", "Game", "Calligraphy.sip"))
		p, _ := sip.Open(raw)
		b, _ := p.Read(qs.Protos[0].Path)
		if bytes.Contains(b, le64(e.DB.Picking.FlagField)) != usable {
			t.Fatalf("exe usable %v: an insignia must take clicks on its mesh only when the rarity code is in", usable)
		}
	}
}
