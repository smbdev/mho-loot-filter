// Package engine makes the game folder match a filter, backing up every original file first.
package engine

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"mholootfilter/internal/assetcache"
	"mholootfilter/internal/db"
	"mholootfilter/internal/patch"
	"mholootfilter/internal/sip"
	"mholootfilter/internal/upk"
	"mholootfilter/internal/wwise"
)

type Report struct {
	Changed  int      `json:"changed"`
	Warnings []string `json:"warnings"`
}

type Engine struct {
	DB          *db.DB
	GameDir     string
	DataDir     string
	GameRunning func() bool
	SkipRarity  bool // tests only: fixture game folder has no MarvelGame.upk / exe

	mu sync.Mutex // one Apply or Restore at a time
}

var ErrGameRunning = errors.New("Marvel Heroes Omega is running - close the game first")

const (
	cookedRel      = "UnrealEngine3/MarvelGame/CookedPCConsole"
	exeRel         = "UnrealEngine3/Binaries/Win64/MarvelHeroesOmega.exe"
	calligraphyRel = "Data/Game/Calligraphy.sip"
	exeSize        = 48544080
)

// file is one game file the engine may change.
type file struct {
	rel     string
	origSha string
	// ours reports whether cur is the original with only this tool's changes applied.
	ours func(cur, orig []byte) bool
}

func (f file) name() string { return filepath.Base(f.rel) }

func (e *Engine) path(rel string) string { return filepath.Join(e.GameDir, filepath.FromSlash(rel)) }

// folderDir holds the state and backups of the current game folder, so switching folders never mixes them.
func (e *Engine) folderDir() string {
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(e.GameDir))))
	return filepath.Join(e.DataDir, "games", hex.EncodeToString(sum[:6]))
}

func (e *Engine) backup(name string) string { return filepath.Join(e.folderDir(), "backup", name) }

// ValidGameDir reports whether dir holds a Marvel Heroes Omega install.
func ValidGameDir(dir string) error {
	for _, rel := range []string{cookedRel + "/MarvelGame.upk", exeRel} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("%s is not a Marvel Heroes Omega folder (%s not found)", dir, filepath.Base(rel))
		}
	}
	return nil
}

// SetGameDir switches to another game folder after checking it holds the game.
func (e *Engine) SetGameDir(dir string) error {
	if err := ValidGameDir(dir); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.GameDir = dir
	return nil
}

// Dir returns the current game folder.
func (e *Engine) Dir() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.GameDir
}

func (e *Engine) hasBackup(name string) bool {
	_, err := os.Stat(e.backup(name))
	return err == nil
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func saveJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(v, "", " ")
	return writeAtomic(path, b)
}

func (e *Engine) statePath() string { return filepath.Join(e.folderDir(), "state.json") }

func (e *Engine) LoadFilter() (Filter, error) {
	var stored struct {
		Filter
		Types map[string]patch.Flags `json:"types"` // version 1 kept per-type settings here
	}
	err := loadJSON(filepath.Join(e.DataDir, "filter.json"), &stored)
	f := stored.Filter
	if len(f.Looks) == 0 && len(stored.Types) > 0 {
		f.Looks = stored.Types
	}
	if f.Items == nil {
		f.Items = map[string]ItemFlags{}
	}
	if f.Looks == nil {
		f.Looks = map[string]patch.Flags{}
	}
	if f.Groups == nil {
		f.Groups = map[string]ItemFlags{}
	}
	if f.Rarities == nil {
		f.Rarities = map[string]bool{}
	}
	if f.RaritySounds == nil {
		f.RaritySounds = map[string]bool{}
	}
	return f, err
}

// migrateV1 moves the backups and state that version 1 kept directly in DataDir to the current game folder's
// directory, so files patched by version 1 can still be restored.
func (e *Engine) migrateV1() error {
	dir := e.folderDir()
	for _, name := range []string{"backup", "state.json"} {
		old, dst := filepath.Join(e.DataDir, name), filepath.Join(dir, name)
		if _, err := os.Stat(old); err != nil {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.Rename(old, dst); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) SaveFilter(f Filter) error {
	return saveJSON(filepath.Join(e.DataDir, "filter.json"), f)
}

// writeAtomic writes data to a uniquely named temporary file, flushes it to disk, then renames it over target,
// so a crash or power cut leaves either the old or the new file, never a partial one.
func writeAtomic(target string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".*.lootfilter.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), target)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// onlyDiffersAt reports whether a and b are the same length and differ only inside the given byte ranges.
func onlyDiffersAt(a, b []byte, offsets []int, width int) bool {
	if len(a) != len(b) {
		return false
	}
	start := 0
	for _, o := range sortedCopy(offsets) {
		if o < start || o+width > len(a) {
			return false
		}
		if !bytes.Equal(a[start:o], b[start:o]) {
			return false
		}
		start = o + width
	}
	return bytes.Equal(a[start:], b[start:])
}

func sortedCopy(v []int) []int {
	out := append([]int{}, v...)
	sort.Ints(out)
	return out
}

// unpackedDiffersAt compares two compressed packages after decompression.
func unpackedDiffersAt(offsets []int) func(cur, orig []byte) bool {
	return func(cur, orig []byte) bool {
		a, err := upk.Unpack(cur)
		if err != nil {
			return false
		}
		b, err := upk.Unpack(orig)
		return err == nil && onlyDiffersAt(a, b, offsets, 4)
	}
}

// resolve reads a game file and finds its original. It never writes the game file; it only creates the backup
// the first time it sees the untouched original. A non-empty warning means the file must be left alone.
func (e *Engine) resolve(f file, state map[string]string) (cur, orig []byte, warning string, err error) {
	cur, err = os.ReadFile(e.path(f.rel))
	if err != nil {
		return nil, nil, fmt.Sprintf("%s: missing from the game folder - skipped", f.name()), nil
	}
	curSha := patch.Sha1Hex(cur)
	if curSha == f.origSha {
		if !e.hasBackup(f.name()) {
			if err := os.MkdirAll(filepath.Dir(e.backup(f.name())), 0o755); err != nil {
				return nil, nil, "", err
			}
			if err := writeAtomic(e.backup(f.name()), cur); err != nil {
				return nil, nil, "", err
			}
		}
		return cur, cur, "", nil
	}
	backup, berr := os.ReadFile(e.backup(f.name()))
	if berr != nil || patch.Sha1Hex(backup) != f.origSha {
		if state[f.rel] != "" {
			return nil, nil, fmt.Sprintf("%s: the backup of the original is missing - run Steam \"Verify integrity of game files\" to repair it", f.name()), nil
		}
		return nil, nil, fmt.Sprintf("%s: changed outside the filter (Steam verify or game update?) - skipped", f.name()), nil
	}
	if curSha != state[f.rel] && !f.ours(cur, backup) {
		return nil, nil, fmt.Sprintf("%s: changed outside the filter (Steam verify or game update?) - skipped", f.name()), nil
	}
	return cur, backup, "", nil
}

// commit writes data over the game file (unless it already holds it) and records the result in state.
func (e *Engine) commit(f file, cur, data []byte, state map[string]string, r *Report) error {
	newSha := patch.Sha1Hex(data)
	if !bytes.Equal(cur, data) {
		if err := writeAtomic(e.path(f.rel), data); err != nil {
			return fmt.Errorf("%s: %w (close the game if it is running, or run the filter as administrator)", f.name(), err)
		}
		r.Changed++
	}
	if newSha == f.origSha {
		delete(state, f.rel)
	} else {
		state[f.rel] = newSha
	}
	return saveJSON(e.statePath(), state)
}

func (e *Engine) run(f Filter) (Report, error) {
	r := Report{Warnings: []string{}}
	if e.GameRunning != nil && e.GameRunning() {
		return r, ErrGameRunning
	}
	if err := e.migrateV1(); err != nil {
		return r, err
	}
	state := map[string]string{}
	if err := loadJSON(e.statePath(), &state); err != nil {
		return r, err
	}
	plan := Resolve(e.DB, f)
	keys := make([]string, 0, len(e.DB.Types))
	for k := range e.DB.Types {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t := e.DB.Types[k]
		flags := plan.Types[k]
		pf := file{rel: cookedRel + "/" + t.File, origSha: t.OrigSha1, ours: typeOurs(t)}
		if flags.None() && state[pf.rel] == "" && !e.hasBackup(t.File) {
			continue // never touched: don't even read it
		}
		cur, orig, warning, err := e.resolve(pf, state)
		if err != nil {
			return r, err
		}
		if warning != "" {
			r.Warnings = append(r.Warnings, warning)
			continue
		}
		data := orig
		if !flags.None() {
			if data, err = patch.Package(orig, t, flags); err != nil {
				return r, fmt.Errorf("%s: %w", t.File, err)
			}
		}
		if err := e.commit(pf, cur, data, state, &r); err != nil {
			return r, err
		}
	}
	if err := e.syncSound(f, plan, state, &r); err != nil {
		return r, err
	}
	clones, err := e.syncClones(plan, state, &r)
	if err != nil {
		return r, err
	}
	if err := e.syncCalligraphy(plan, clones, state, &r); err != nil {
		return r, err
	}
	if err := e.removeStaleClones(clones, state, &r); err != nil {
		return r, err
	}
	if e.SkipRarity {
		return r, nil
	}
	return r, e.syncRarity(f, state, &r)
}

// typeOurs reports whether an item package is the original with one of the filter's patches applied.
// Only used when the recorded state is lost, so rebuilding every combination is affordable.
func typeOurs(t *db.Type) func(cur, orig []byte) bool {
	return func(cur, orig []byte) bool {
		a, err := upk.Unpack(cur)
		if err != nil {
			return false
		}
		for i := 1; i < 16; i++ {
			f := patch.Flags{Glow: i&1 != 0, Model: i&2 != 0, Name: i&4 != 0, Sound: i&8 != 0}
			if b, err := patch.Package(orig, t, f); err == nil {
				if c, err := upk.Unpack(b); err == nil && bytes.Equal(a, c) {
					return true
				}
			}
		}
		return false
	}
}

// syncSound puts the alert into the drop sound bank when any item or rarity plays it.
func (e *Engine) syncSound(f Filter, plan Plan, state map[string]string, r *Report) error {
	var sounds []uint32
	used := len(plan.Clones) > 0
	for _, fl := range plan.Types {
		used = used || fl.Sound
	}
	if used {
		sounds = append(sounds, patch.TokenSound)
	}
	for _, rarity := range []string{"Cosmic", "Unique"} {
		if f.RaritySounds[rarity] {
			sounds = append(sounds, patch.RaritySounds[rarity])
		}
	}
	pck := file{rel: cookedRel + "/" + patch.SoundPackage, origSha: e.DB.SoundPackageSha1,
		ours: func(cur, orig []byte) bool { return wwise.Extends(cur, orig, patch.ItemSoundBank) }}
	if len(sounds) == 0 && state[pck.rel] == "" && !e.hasBackup(pck.name()) {
		return nil
	}
	cur, orig, warning, err := e.resolve(pck, state)
	if err != nil {
		return err
	}
	if warning != "" {
		r.Warnings = append(r.Warnings, warning+" - drop sounds unchanged")
		return nil
	}
	data := orig
	if len(sounds) > 0 {
		if data, err = patch.Sounds(orig, sounds, e.alert()); err != nil {
			return fmt.Errorf("%s: %w", patch.SoundPackage, err)
		}
	}
	return e.commit(pck, cur, data, state, r)
}

// MaxAlertSeconds limits a custom alert: the whole sound is stored in the game's sound bank.
const MaxAlertSeconds = 10

func (e *Engine) alertPath() string     { return filepath.Join(e.DataDir, "alert.wem") }
func (e *Engine) alertNamePath() string { return filepath.Join(e.DataDir, "alert.json") }

// alert returns the sound played for alerted drops: the user's own, or the built-in one.
func (e *Engine) alert() []byte {
	if b, err := os.ReadFile(e.alertPath()); err == nil {
		return b
	}
	return wwise.Alert
}

// Alert returns the alert sound and the name of the file it came from, empty for the built-in sound.
func (e *Engine) Alert() (wem []byte, name string) {
	var info struct{ Name string }
	if _, err := os.Stat(e.alertPath()); err == nil {
		loadJSON(e.alertNamePath(), &info)
		if info.Name == "" {
			info.Name = "Custom sound"
		}
	}
	return e.alert(), info.Name
}

// SetAlert makes a mono 44.1 kHz sound, named after the file it came from, the alert. Nil samples bring back
// the built-in sound. It takes effect at the next Apply.
func (e *Engine) SetAlert(name string, samples []int16) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if samples == nil {
		for _, p := range []string{e.alertPath(), e.alertNamePath()} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	if len(samples) == 0 || len(samples) > MaxAlertSeconds*wwise.Rate {
		return fmt.Errorf("the sound must be between 0 and %d seconds long", MaxAlertSeconds)
	}
	if err := os.MkdirAll(e.DataDir, 0o755); err != nil {
		return err
	}
	if err := writeAtomic(e.alertPath(), wwise.PCM(samples)); err != nil {
		return err
	}
	return saveJSON(e.alertNamePath(), struct{ Name string }{name})
}

// clone is a copy of an item class that plays the alert.
type clone struct {
	name      string // class and Calligraphy asset name
	id, guid  uint64 // Calligraphy asset id and GUID
	sourceKey string
}

func (c clone) rel() string { return cookedRel + "/" + patch.CloneFile(c.name) }

// syncClones writes the class copies the plan needs and registers them in AssetPackageCache.bin, without which the
// game never loads them. It returns the copies that are ready, by item type; items of any other copy keep their
// own class.
func (e *Engine) syncClones(plan Plan, state map[string]string, r *Report) (map[string]clone, error) {
	ready := map[string]clone{}
	cache := file{rel: cookedRel + "/" + assetcache.File, origSha: e.DB.AssetPackageCacheSha1, ours: assetcache.Extends}
	if len(plan.Clones) == 0 && state[cache.rel] == "" && !e.hasBackup(cache.name()) {
		return ready, nil
	}
	cur, orig, warning, err := e.resolve(cache, state)
	if err != nil {
		return nil, err
	}
	if warning != "" {
		if len(plan.Clones) > 0 {
			r.Warnings = append(r.Warnings, warning+" - items that share a look play no alert")
		}
		return ready, nil
	}
	keys := make([]string, 0, len(e.DB.Types))
	for k := range e.DB.Types {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data := orig
	for n, key := range keys {
		flags, ok := plan.Clones[key]
		if !ok {
			continue
		}
		t := e.DB.Types[key]
		c := clone{sourceKey: key}
		if c.name, err = patch.CloneName(key, n); err != nil {
			r.Warnings = append(r.Warnings, err.Error())
			continue
		}
		c.id, c.guid = patch.CloneIDs(c.name)
		packages, err := assetcache.Packages(orig, "marvelgameitems."+key)
		if err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v - its items play no alert", t.File, err))
			continue
		}
		for i, p := range packages {
			if p == "uc__"+key+"_sf" {
				packages[i] = "uc__" + strings.ToLower(c.name) + "_sf"
			}
		}
		_, src, warning, err := e.resolve(file{rel: cookedRel + "/" + t.File, origSha: t.OrigSha1, ours: typeOurs(t)}, state)
		if err != nil {
			return nil, err
		}
		if warning != "" {
			r.Warnings = append(r.Warnings, warning)
			continue
		}
		pkg, err := patch.Clone(src, t, key, c.name, flags)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.File, err)
		}
		if err := e.writeClone(c, pkg, state, r); err != nil {
			return nil, err
		}
		if data, err = assetcache.AddClass(data, c.guid, "marvelgameitems."+c.name, packages); err != nil {
			return nil, fmt.Errorf("%s: %w", assetcache.File, err)
		}
		ready[key] = c
	}
	return ready, e.commit(cache, cur, data, state, r)
}

// writeClone writes a class copy, a file the game does not ship, and records it so it can be removed again.
func (e *Engine) writeClone(c clone, pkg []byte, state map[string]string, r *Report) error {
	path := e.path(c.rel())
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, pkg) {
		state[c.rel()] = patch.Sha1Hex(pkg)
		return nil
	}
	if err := writeAtomic(path, pkg); err != nil {
		return fmt.Errorf("%s: %w (close the game if it is running, or run the filter as administrator)", filepath.Base(path), err)
	}
	r.Changed++
	state[c.rel()] = patch.Sha1Hex(pkg)
	return saveJSON(e.statePath(), state)
}

// removeStaleClones deletes class copies the filter wrote earlier that the current filter no longer uses.
// It runs after Calligraphy.sip stops pointing items at them.
func (e *Engine) removeStaleClones(ready map[string]clone, state map[string]string, r *Report) error {
	keep := map[string]bool{}
	for _, c := range ready {
		keep[c.rel()] = true
	}
	for rel := range state {
		if !patch.IsClone(filepath.Base(rel)) || keep[rel] {
			continue
		}
		if err := os.Remove(e.path(rel)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: %w (close the game if it is running, or run the filter as administrator)", filepath.Base(rel), err)
		}
		r.Changed++
		delete(state, rel)
		if err := saveJSON(e.statePath(), state); err != nil {
			return err
		}
	}
	return nil
}

// syncCalligraphy registers the class copies and points every item in plan.Retargets at its sink class or copy,
// starting from the original archive.
func (e *Engine) syncCalligraphy(plan Plan, clones map[string]clone, state map[string]string, r *Report) error {
	cal := file{rel: calligraphyRel, origSha: e.DB.CalligraphySha1, ours: e.calligraphyOurs}
	if len(plan.Retargets) == 0 && state[cal.rel] == "" && !e.hasBackup(cal.name()) {
		return nil
	}
	cur, orig, warning, err := e.resolve(cal, state)
	if err != nil {
		return err
	}
	if warning != "" {
		r.Warnings = append(r.Warnings, warning)
		return nil
	}
	data := orig
	if len(plan.Retargets) > 0 {
		pak, err := sip.Open(orig)
		if err != nil {
			return fmt.Errorf("Calligraphy.sip: %w", err)
		}
		protos := e.protoIndex()
		fields := map[uint64]bool{}
		for _, f := range e.DB.UnrealClassFields {
			fields[f] = true
		}
		if len(clones) > 0 {
			list, err := pak.Read(sip.UnrealClassTypes)
			if err != nil {
				return fmt.Errorf("Calligraphy.sip: %w", err)
			}
			for _, key := range sortedKeys(clones) {
				c := clones[key]
				if list, err = sip.AddAsset(list, c.id, c.guid, c.name); err != nil {
					return fmt.Errorf("Calligraphy.sip: %w", err)
				}
			}
			if err := pak.Replace(sip.UnrealClassTypes, list); err != nil {
				return fmt.Errorf("Calligraphy.sip: %w", err)
			}
		}
		for path, target := range plan.Retargets {
			slot, ok := protos[path]
			if !ok {
				continue
			}
			asset := e.DB.Sinks[target].Asset
			if _, isSink := e.DB.Sinks[target]; !isSink {
				c, ready := clones[target]
				if !ready {
					continue // its copy could not be made: the item keeps its own class
				}
				asset = c.id
			}
			proto, err := pak.Read(path)
			if err != nil {
				return fmt.Errorf("Calligraphy.sip: %w", err)
			}
			changed, err := sip.Retarget(proto, asset, slot, fields)
			if err != nil {
				return fmt.Errorf("Calligraphy.sip %s: %w", path, err)
			}
			if err := pak.Replace(path, changed); err != nil {
				return fmt.Errorf("Calligraphy.sip: %w", err)
			}
		}
		// Prototypes of other items that inherit a retargeted class keep their own.
		for path := range plan.Retargets {
			for _, pin := range protos[path].Inheritors {
				if _, own := plan.Retargets[pin.Path]; own {
					continue
				}
				proto, err := pak.Read(pin.Path)
				if err != nil {
					return fmt.Errorf("Calligraphy.sip: %w", err)
				}
				changed, err := sip.Retarget(proto, pin.Asset, pin.Slot(), fields)
				if err != nil {
					return fmt.Errorf("Calligraphy.sip %s: %w", pin.Path, err)
				}
				if err := pak.Replace(pin.Path, changed); err != nil {
					return fmt.Errorf("Calligraphy.sip: %w", err)
				}
			}
		}
		data = pak.Bytes()
	}
	return e.commit(cal, cur, data, state, r)
}

func (e *Engine) protoIndex() map[string]db.Proto {
	out := map[string]db.Proto{}
	for _, it := range e.DB.Items {
		for _, p := range it.Protos {
			out[p.Path] = p
		}
	}
	for _, it := range e.DB.Items {
		for _, p := range it.Protos {
			for _, pin := range p.Inheritors {
				if _, ok := out[pin.Path]; !ok {
					out[pin.Path] = pin.Slot() // changed when pinned, so a Calligraphy.sip with it is still ours
				}
			}
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// calligraphyOurs reports whether every entry that differs from the original is an item prototype or the list
// of item classes.
func (e *Engine) calligraphyOurs(cur, orig []byte) bool {
	a, err := sip.Open(cur)
	if err != nil {
		return false
	}
	b, err := sip.Open(orig)
	if err != nil || len(a.Names()) != len(b.Names()) {
		return false
	}
	protos := e.protoIndex()
	for _, name := range b.Names() {
		if !bytes.Equal(a.Blob(name), b.Blob(name)) {
			if _, ok := protos[name]; !ok && name != sip.UnrealClassTypes {
				return false
			}
		}
	}
	return true
}

// syncRarity changes MarvelGame.upk and the SHA1 the exe stores for it together: the game refuses to start
// when they disagree, so either both are written or neither is.
func (e *Engine) syncRarity(f Filter, state map[string]string, r *Report) error {
	hide := false
	for _, h := range f.Rarities {
		hide = hide || h
	}
	var rarityOffsets []int
	for _, o := range e.DB.Rarities {
		rarityOffsets = append(rarityOffsets, o...)
	}
	mg := file{rel: cookedRel + "/MarvelGame.upk", origSha: e.DB.MarvelGameSha1, ours: unpackedDiffersAt(rarityOffsets)}
	exe := file{rel: exeRel, ours: func(cur, orig []byte) bool {
		return onlyDiffersAt(cur, orig, []int{patch.ExeHashOffset}, 20)
	}}
	touched := state[mg.rel] != "" || state[exe.rel] != "" || e.hasBackup(mg.name()) || e.hasBackup(exe.name())
	if !hide && !touched {
		return nil
	}

	exeSha, err := e.exeOriginalSha()
	if err != nil {
		r.Warnings = append(r.Warnings, err.Error()+" - rarity glow left unchanged")
		return nil
	}
	exe.origSha = exeSha
	mgCur, mgOrig, mgWarning, err := e.resolve(mg, state)
	if err != nil {
		return err
	}
	exeCur, exeOrig, exeWarning, err := e.resolve(exe, state)
	if err != nil {
		return err
	}
	if mgWarning != "" || exeWarning != "" {
		for _, w := range []string{mgWarning, exeWarning} {
			if w != "" {
				r.Warnings = append(r.Warnings, w)
			}
		}
		r.Warnings = append(r.Warnings, "rarity glow left unchanged: MarvelGame.upk and MarvelHeroesOmega.exe must change together")
		return nil
	}

	mgData, exeData := mgOrig, exeOrig
	if hide {
		if mgData, err = patch.MarvelGame(mgOrig, e.DB.Rarities, f.Rarities); err != nil {
			return fmt.Errorf("MarvelGame.upk: %w", err)
		}
		if exeData, err = patch.ExeHash(exeOrig, mgData); err != nil {
			return fmt.Errorf("%s: %w", exe.name(), err)
		}
	}
	if err := e.commit(mg, mgCur, mgData, state, r); err != nil {
		return err
	}
	if err := e.commit(exe, exeCur, exeData, state, r); err != nil {
		if rerr := writeAtomic(e.path(mg.rel), mgCur); rerr == nil {
			e.commit(mg, mgData, mgCur, state, &Report{})
		}
		return err
	}
	return nil
}

// exeOriginalSha returns the exe's SHA1 from before the filter first touched it (its backup),
// or its current SHA1 if it is the 2.16a exe and still holds MarvelGame's original hash.
func (e *Engine) exeOriginalSha() (string, error) {
	if b, err := os.ReadFile(e.backup(filepath.Base(exeRel))); err == nil {
		return patch.Sha1Hex(b), nil
	}
	cur, err := os.ReadFile(e.path(exeRel))
	if err != nil {
		return "", errors.New("MarvelHeroesOmega.exe not found")
	}
	want, _ := hex.DecodeString(e.DB.MarvelGameSha1)
	if len(cur) != exeSize || !bytes.Equal(cur[patch.ExeHashOffset:patch.ExeHashOffset+20], want) {
		return "", errors.New("MarvelHeroesOmega.exe is not the original 2.16a version")
	}
	return patch.Sha1Hex(cur), nil
}

func (e *Engine) Apply(f Filter) (Report, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.GameRunning != nil && e.GameRunning() {
		return Report{Warnings: []string{}}, ErrGameRunning
	}
	if err := e.SaveFilter(f); err != nil {
		return Report{Warnings: []string{}}, err
	}
	return e.run(f)
}

// RestoreAll puts every changed game file back to its original and empties the filter.
func (e *Engine) RestoreAll() (Report, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, err := e.run(Filter{})
	if err != nil {
		return r, err
	}
	return r, e.SaveFilter(Filter{})
}
