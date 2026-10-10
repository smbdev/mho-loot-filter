// Command soundindex writes internal/db/sounds.json: every sound the game plays by name, and where its event sits
// in the Wwise sound banks, so the filter can mute it.
//
// Usage: go run ./tools/soundindex "<game folder>"
//
// The game posts sounds through Wwise events named by its packages' AkEvent objects (Play_sfx_ui_LevelUp...).
// An event's ID is the FNV-1 hash of its lower-case name. Banks hold many more events that nothing names; the
// game never plays those, so only named events are listed. For previews it also records where the audio of each
// event's first sound sits, following the event into other banks where needed.
package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mholootfilter/internal/db"
	"mholootfilter/internal/upk"
	"mholootfilter/internal/wwise"
)

var le = binary.LittleEndian

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/soundindex \"<game folder>\"")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type event struct {
	at []db.SoundAt
}

// origin is the bank a hierarchy node came from.
type origin struct {
	file string
	bank uint32
}

// language reports whether a file package holds a translation; the English one is preferred for previews.
func language(file string) bool {
	return strings.HasSuffix(file, "_DEU.pck") || strings.HasSuffix(file, "_FRA.pck")
}

func run(gameDir string) error {
	cooked := filepath.Join(gameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole")
	ents, err := os.ReadDir(cooked)
	if err != nil {
		return err
	}

	// every event object in every bank of every file package
	events := map[uint32]*event{}
	ids := map[uint32]bool{} // every object ID, so a muted ID can be chosen that clashes with none
	files := map[string]string{}
	// the hierarchy of every bank, alone and all together, and where each bank's audio sits
	local := map[origin]map[uint32]wwise.Node{}
	localOrder := map[origin][]uint32{}
	all := map[uint32]wwise.Node{}
	var allOrder []uint32
	from := map[uint32]origin{}
	embedded := map[origin]map[uint32][2]int{} // media -> absolute offset and size
	streamed := map[uint32]db.Preview{}
	for _, de := range ents {
		name := de.Name()
		if !strings.HasSuffix(name, ".pck") {
			continue
		}
		pck, err := os.ReadFile(filepath.Join(cooked, name))
		if err != nil {
			return err
		}
		banks, err := wwise.Banks(pck)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		streamedHere, err := wwise.StreamedFiles(pck)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for id, f := range streamedHere {
			if _, ok := streamed[id]; !ok || !language(name) {
				streamed[id] = db.Preview{File: name, Offset: f.Offset, Size: int(f.Size)}
			}
		}
		found := false
		for _, bankID := range banks {
			bank, err := wwise.Bank(pck, bankID)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			o := origin{name, bankID}
			nodes, order, err := wwise.Nodes(bank)
			if err != nil {
				return fmt.Errorf("%s bank %d: %w", name, bankID, err)
			}
			for id, n := range nodes {
				n.Body = bytes.Clone(n.Body) // keep the hierarchy, not the whole package
				nodes[id] = n
				if _, ok := all[id]; !ok || (language(from[id].file) && !language(name)) {
					if !ok {
						allOrder = append(allOrder, id)
					}
					all[id], from[id] = n, o
				}
			}
			local[o], localOrder[o] = nodes, order
			start, err := wwise.BankAt(pck, bankID)
			if err != nil {
				return err
			}
			media, err := wwise.EmbeddedMedia(bank)
			if err != nil {
				return fmt.Errorf("%s bank %d: %w", name, bankID, err)
			}
			for m, at := range media {
				media[m] = [2]int{start + at[0], at[1]}
			}
			embedded[o] = media
			objs, err := wwise.Objects(bank)
			if err != nil {
				return fmt.Errorf("%s bank %d: %w", name, bankID, err)
			}
			for _, obj := range objs {
				ids[obj.ID] = true
				if obj.Type != wwise.HircEvent {
					continue
				}
				if events[obj.ID] == nil {
					events[obj.ID] = &event{}
				}
				events[obj.ID].at = append(events[obj.ID].at, db.SoundAt{File: name, Bank: bankID, Offset: obj.IDOffset})
				found = true
			}
		}
		if found {
			sum := sha1.Sum(pck)
			files[name] = hex.EncodeToString(sum[:])
		}
	}

	// names of events the game posts: Play_ names in any package's name table
	named := map[uint32]string{}
	for _, de := range ents {
		if !strings.HasSuffix(de.Name(), ".upk") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(cooked, de.Name()))
		if err != nil {
			return err
		}
		flat, err := upk.Unpack(raw)
		if err != nil {
			flat = raw // not compressed
		}
		names, err := upk.Names(flat)
		if err != nil {
			continue
		}
		for _, n := range names {
			low := strings.ToLower(n)
			if !strings.HasPrefix(low, "play_") {
				continue
			}
			if id := wwise.ShortID(low); events[id] != nil {
				named[id] = low
			}
		}
	}

	var xor uint32 = 0x4D484F00 // "MHO\0"; muting flips an event ID with it
	for ; ; xor++ {
		clash := false
		for id := range named {
			if ids[id^xor] {
				clash = true
				break
			}
		}
		if !clash {
			break
		}
	}

	// preview: the first sound the event plays, from its own bank or, failing that, from any bank
	preview := func(id uint32) *db.Preview {
		at := events[id].at[0]
		for _, a := range events[id].at {
			if !language(a.File) {
				at = a
			}
		}
		o := origin{at.File, at.Bank}
		plays, err := wwise.Plays(local[o], localOrder[o], id)
		nodes := local[o]
		if err != nil || len(plays) == 0 {
			plays, _ = wwise.Plays(all, allOrder, id)
			nodes = all
		}
		for _, n := range plays {
			home := o
			if _, ok := local[o][n]; !ok {
				home = from[n]
			}
			for _, s := range nodes[n].Sources() {
				if s.Embedded {
					if m, ok := embedded[home][s.Media]; ok {
						return &db.Preview{File: home.file, Offset: int64(m[0]), Size: m[1]}
					}
				} else if f, ok := streamed[s.Media]; ok {
					return &f
				}
			}
		}
		return nil
	}

	out := db.Sounds{MuteXor: xor, Files: map[string]string{}}
	previews := 0
	for id, name := range named {
		at := events[id].at
		sort.Slice(at, func(i, j int) bool { return at[i].File < at[j].File })
		p := preview(id)
		if p != nil {
			previews++
		}
		out.Sounds = append(out.Sounds, db.Sound{Name: name, At: at, Preview: p})
		for _, a := range at {
			out.Files[a.File] = files[a.File]
		}
	}
	sort.Slice(out.Sounds, func(i, j int) bool { return out.Sounds[i].Name < out.Sounds[j].Name })
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	dst := filepath.Join("internal", "db", "sounds.json")
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %d sounds (%d with a preview) in %d files to %s\n", len(out.Sounds), previews, len(out.Files), dst)
	return nil
}
