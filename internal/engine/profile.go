package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Profile is one saved filter, with its own alert sound.
type Profile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Profiles lists the saved filters and which one is in use.
type Profiles struct {
	Active int       `json:"active"`
	List   []Profile `json:"list"`
}

// profileFiles are what a profile keeps in its folder.
var profileFiles = []string{"filter.json", "alert.wem", "alert.json"}

const maxProfileName = 40

func (e *Engine) profilesPath() string { return filepath.Join(e.DataDir, "profiles.json") }

// dirOf is the folder of a profile. Profile 0 keeps its files straight in DataDir, where versions before profiles
// kept the filter, so that filter becomes the first profile without moving anything.
func (e *Engine) dirOf(id int) string {
	if id == 0 {
		return e.DataDir
	}
	return filepath.Join(e.DataDir, "profiles", strconv.Itoa(id))
}

func (e *Engine) loadProfiles() (Profiles, error) {
	var p Profiles
	if err := loadJSON(e.profilesPath(), &p); err != nil {
		return p, fmt.Errorf("profiles.json: %w", err)
	}
	if len(p.List) == 0 {
		p = Profiles{List: []Profile{{ID: 0, Name: "My filter"}}}
	}
	if p.find(p.Active) < 0 {
		p.Active = p.List[0].ID
	}
	return p, nil
}

func (p Profiles) find(id int) int {
	return slices.IndexFunc(p.List, func(pr Profile) bool { return pr.ID == id })
}

// profileDir is the folder of the profile in use.
func (e *Engine) profileDir() (string, error) {
	p, err := e.loadProfiles()
	return e.dirOf(p.Active), err
}

// Profiles returns the saved filters.
func (e *Engine) Profiles() (Profiles, error) { return e.loadProfiles() }

// change runs edit on the profiles under the engine lock, so an Apply never sees a half-switched profile, and
// stores the result.
func (e *Engine) change(edit func(p *Profiles) error) (Profiles, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.loadProfiles()
	if err != nil {
		return p, err
	}
	if err := edit(&p); err != nil {
		return p, err
	}
	return p, saveJSON(e.profilesPath(), p)
}

// UseProfile switches to another saved filter. It reaches the game at the next Apply.
func (e *Engine) UseProfile(id int) (Profiles, error) {
	return e.change(func(p *Profiles) error {
		if p.find(id) < 0 {
			return errors.New("That profile no longer exists")
		}
		p.Active = id
		return nil
	})
}

// AddProfile adds a profile and switches to it: empty, or a copy of the active one with its sound.
func (e *Engine) AddProfile(copyActive bool) (Profiles, error) {
	return e.change(func(p *Profiles) error {
		id := 0
		for _, pr := range p.List {
			id = max(id, pr.ID+1)
		}
		name := fmt.Sprintf("Profile %d", len(p.List)+1)
		for n := len(p.List) + 2; p.taken(name, -1); n++ {
			name = fmt.Sprintf("Profile %d", n)
		}
		dir := e.dirOf(id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if copyActive {
			name = p.List[p.find(p.Active)].Name + " copy"
			for n := 2; p.taken(name, -1); n++ {
				name = fmt.Sprintf("%s copy %d", p.List[p.find(p.Active)].Name, n)
			}
			for _, f := range profileFiles {
				b, err := os.ReadFile(filepath.Join(e.dirOf(p.Active), f))
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return err
				}
				if err := writeAtomic(filepath.Join(dir, f), b); err != nil {
					return err
				}
			}
		}
		p.List = append(p.List, Profile{ID: id, Name: name})
		p.Active = id
		return nil
	})
}

func (p Profiles) taken(name string, except int) bool {
	return slices.ContainsFunc(p.List, func(pr Profile) bool { return pr.ID != except && strings.EqualFold(pr.Name, name) })
}

// RenameProfile gives a profile a new name, which no other profile has.
func (e *Engine) RenameProfile(id int, name string) (Profiles, error) {
	name = strings.TrimSpace(name)
	return e.change(func(p *Profiles) error {
		i := p.find(id)
		switch {
		case i < 0:
			return errors.New("That profile no longer exists")
		case name == "":
			return errors.New("Give the profile a name")
		case len([]rune(name)) > maxProfileName:
			return fmt.Errorf("Profile names can be at most %d letters long", maxProfileName)
		case p.taken(name, id):
			return fmt.Errorf("There is already a profile called %s", name)
		}
		p.List[i].Name = name
		return nil
	})
}

// DeleteProfile removes a profile and its files. Deleting the one in use switches to the first one left.
func (e *Engine) DeleteProfile(id int) (Profiles, error) {
	return e.change(func(p *Profiles) error {
		i := p.find(id)
		if i < 0 {
			return errors.New("That profile no longer exists")
		}
		if len(p.List) == 1 {
			return errors.New("The last profile cannot be deleted")
		}
		dir := e.dirOf(id)
		for _, f := range profileFiles {
			if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if id != 0 {
			if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		p.List = slices.Delete(p.List, i, i+1)
		if p.Active == id {
			p.Active = p.List[0].ID
		}
		return nil
	})
}
