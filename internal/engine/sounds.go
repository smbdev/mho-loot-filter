package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"mholootfilter/internal/patch"
	"mholootfilter/internal/wwise"
)

func (e *Engine) mutesPath() string { return filepath.Join(e.DataDir, "mutes.json") }

// Mutes returns the names of the muted sounds. Like the tweaks they belong to the game, not to a profile.
func (e *Engine) Mutes() ([]string, error) {
	var m []string
	if err := loadJSON(e.mutesPath(), &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = []string{}
	}
	return m, nil
}

// SetMutes stores the muted sounds. They reach the game at the next Apply.
func (e *Engine) SetMutes(names []string) error {
	known := map[string]bool{}
	if e.Sounds != nil {
		for _, s := range e.Sounds.Sounds {
			known[s.Name] = true
		}
	}
	for _, n := range names {
		if !known[n] {
			return fmt.Errorf("Unknown sound %q", n)
		}
	}
	names = slices.Clone(names)
	slices.Sort(names)
	e.mu.Lock()
	defer e.mu.Unlock()
	return saveJSON(e.mutesPath(), slices.Compact(names))
}

// muteSpot is one copy of a sound's event in a file package.
type muteSpot struct {
	bank   uint32
	offset int
	id     uint32 // the event's own ID
	muted  bool   // whether the filter should mute it
}

// muteSpots lists, per file package, every event the filter can mute there and whether it should be muted.
func (e *Engine) muteSpots(muted []string) map[string][]muteSpot {
	want := map[string]bool{}
	for _, n := range muted {
		want[n] = true
	}
	out := map[string][]muteSpot{}
	if e.Sounds == nil {
		return out
	}
	for _, s := range e.Sounds.Sounds {
		id := wwise.ShortID(s.Name)
		for _, a := range s.At {
			out[a.File] = append(out[a.File], muteSpot{a.Bank, a.Offset, id, want[s.Name]})
		}
	}
	return out
}

// setMutes mutes and unmutes the events in spots, in place. It reports false when an event's ID is neither its own
// nor its muted one: the file is not the one the sound list was made from.
func (e *Engine) setMutes(pck []byte, spots []muteSpot, onlyUnmute bool) (bool, error) {
	for _, s := range spots {
		from, to := s.id^e.Sounds.MuteXor, s.id
		if s.muted && !onlyUnmute {
			from, to = to, from
		}
		ok, err := wwise.SetEventID(pck, s.bank, s.offset, from, to)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// syncMutes mutes the chosen sounds in every file package except the one syncSound looks after. Muting changes a
// few bytes in place and undoing it gives the exact original back, so these large files get no backup: the file
// is checked against the game's own copy once every event is unmuted.
func (e *Engine) syncMutes(muted []string, state map[string]string, r *Report) error {
	spots := e.muteSpots(muted)
	var files []string
	for f := range spots {
		if f != patch.SoundPackage {
			files = append(files, f)
		}
	}
	slices.Sort(files)
	for _, name := range files {
		rel := cookedRel + "/" + name
		wanted := slices.ContainsFunc(spots[name], func(s muteSpot) bool { return s.muted })
		if !wanted && state[rel] == "" {
			continue
		}
		pck, err := os.ReadFile(e.path(rel))
		if err != nil {
			r.Warnings = append(r.Warnings, name+": missing from the game folder - sounds in it not muted")
			continue
		}
		before := patch.Sha1Hex(pck)
		ok, err := e.setMutes(pck, spots[name], true)
		if err != nil || !ok || patch.Sha1Hex(pck) != e.Sounds.Files[name] {
			r.Warnings = append(r.Warnings, name+": changed outside the filter (Steam verify or game update?) - sounds in it left alone")
			continue
		}
		if _, err := e.setMutes(pck, spots[name], false); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		f := file{rel: rel, origSha: e.Sounds.Files[name]}
		cur := pck
		if patch.Sha1Hex(pck) != before {
			cur = nil // differs from data, so commit writes it
		}
		if err := e.commit(f, cur, pck, state, r); err != nil {
			return err
		}
	}
	return nil
}

// muteSoundPackage mutes the chosen sounds in syncSound's file package, whose original is backed up. It works on
// a copy: data may be that backup.
func (e *Engine) muteSoundPackage(data []byte, muted []string) ([]byte, error) {
	spots := e.muteSpots(muted)[patch.SoundPackage]
	if !slices.ContainsFunc(spots, func(s muteSpot) bool { return s.muted }) {
		return data, nil
	}
	data = bytes.Clone(data)
	ok, err := e.setMutes(data, spots, false)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s is not the game's own copy", patch.SoundPackage)
	}
	return data, nil
}
