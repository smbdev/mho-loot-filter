package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"mholootfilter/internal/db"
	"mholootfilter/internal/engine"
	"mholootfilter/internal/web"
)

const defaultSteamGame = `C:\Program Files (x86)\Steam\steamapps\common\Marvel Heroes`

func defaultDataDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "MHOLootFilter")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".mholootfilter")
}

type settings struct {
	GameDir string `json:"gameDir"`
}

func loadSettings(dataDir string) settings {
	var s settings
	if b, err := os.ReadFile(filepath.Join(dataDir, "settings.json")); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func saveSettings(dataDir string, s settings) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", " ")
	return os.WriteFile(filepath.Join(dataDir, "settings.json"), b, 0o644)
}

// newApp loads the database and picks the game folder: the -game flag, then the saved folder, then the default
// Steam folder. If none of those holds the game, the first install the background search finds is used.
// The folder in use is saved, so the filter keeps working on the same install between launches.
func newApp(gameFlag, dataDir string, det *detector) (*db.DB, *engine.Engine, web.Options, error) {
	d, err := db.Load()
	if err != nil {
		return nil, nil, web.Options{}, err
	}
	saved := loadSettings(dataDir).GameDir
	game := gameFlag
	if game == "" {
		game = saved
	}
	if game == "" {
		game = defaultSteamGame
	}
	save := func(dir string) error { return saveSettings(dataDir, settings{GameDir: dir}) }
	if gameFlag == "" && saved == "" && engine.ValidGameDir(game) == nil {
		save(game)
	}
	e := &engine.Engine{DB: d, GameDir: game, DataDir: dataDir, GameRunning: gameRunning}
	det.OnDone(func(found []string) {
		if len(found) > 0 && engine.ValidGameDir(e.Dir()) != nil && e.SetGameDir(found[0]) == nil {
			save(found[0])
		}
	})
	opts := web.Options{
		Searching:  func() bool { _, done := det.Results(); return !done },
		Detect:     func() []string { found, _ := det.Results(); return found },
		SaveFolder: save,
	}
	return d, e, opts, nil
}
