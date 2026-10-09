package icons

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mholootfilter/internal/upk"
)

// Set serves item icons from one game folder. Icon packages are read the first time one of their icons is asked
// for, and every icon is decoded once.
type Set struct {
	cooked string

	mu    sync.Mutex
	pkgs  map[string]*pkg   // package name (lower case) -> contents, nil when it could not be read
	cache map[string][]byte // icon -> PNG, nil when it could not be made
}

type pkg struct {
	flat    []byte
	names   []string
	exports map[string]upk.Export // texture name (lower case) -> export
}

// New returns the icons of the game in gameDir.
func New(gameDir string) *Set {
	return &Set{cooked: filepath.Join(gameDir, "UnrealEngine3", "MarvelGame", "CookedPCConsole"),
		pkgs: map[string]*pkg{}, cache: map[string][]byte{}}
}

// PNG returns an icon, named as in the game data ("MarvelUIIcons.Item_Unique335"), as a PNG image.
func (s *Set) PNG(icon string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.cache[icon]; ok {
		if b == nil {
			return nil, errors.New("no icon")
		}
		return b, nil
	}
	b, err := s.load(icon)
	s.cache[icon] = b
	return b, err
}

func (s *Set) load(icon string) ([]byte, error) {
	pkgName, texName, ok := strings.Cut(strings.ToLower(icon), ".")
	if !ok { // a few items name the texture alone; the item icons live in MarvelUIIcons
		pkgName, texName = "marveluiicons", pkgName
	}
	p := s.pkg(pkgName)
	if p == nil {
		return nil, errors.New("icon package not found")
	}
	e, ok := p.exports[texName]
	if !ok || e.Offset+e.Size > len(p.flat) {
		return nil, errors.New("icon not found")
	}
	return texture(p.flat[e.Offset:e.Offset+e.Size], p.names)
}

func (s *Set) pkg(name string) *pkg {
	if p, ok := s.pkgs[name]; ok {
		return p
	}
	s.pkgs[name] = nil
	matches, _ := filepath.Glob(filepath.Join(s.cooked, "ICO__*_SF.upk"))
	for _, m := range matches {
		base := strings.ToLower(filepath.Base(m))
		if base != "ico__"+name+"_sf.upk" {
			continue
		}
		raw, err := os.ReadFile(m)
		if err != nil {
			return nil
		}
		flat, err := upk.Unpack(raw)
		if err != nil {
			return nil
		}
		names, err1 := upk.Names(flat)
		exports, err2 := upk.Exports(flat)
		if err1 != nil || err2 != nil {
			return nil
		}
		p := &pkg{flat: flat, names: names, exports: map[string]upk.Export{}}
		for _, e := range exports {
			if e.Outer == 0 || exports[e.Outer-1].Outer == 0 { // textures sit at the top of the icon package
				p.exports[e.Name] = e
			}
		}
		s.pkgs[name] = p
		return p
	}
	return nil
}
