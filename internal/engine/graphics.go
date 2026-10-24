package engine

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"mholootfilter/internal/patch"
)

// graphicsState is the preset last written into the Options file, and what that file had for its keys before.
type graphicsState struct {
	Preset string          `json:"preset"`
	Saved  []patch.Setting `json:"saved"`
}

func (e *Engine) graphicsPath() string { return filepath.Join(e.DataDir, "graphics.json") }

// syncGraphics writes the Options settings of preset into the game's Options file when the preset changes, so the
// game's own Options show them and can still change them afterwards. No preset puts back what the file had before.
func (e *Engine) syncGraphics(preset string, r *Report) error {
	var g graphicsState
	if err := loadJSON(e.graphicsPath(), &g); err != nil {
		return err
	}
	if g.Preset == preset {
		return nil
	}
	cur, err := os.ReadFile(e.OptionsFile)
	if e.OptionsFile == "" || errors.Is(err, os.ErrNotExist) {
		if preset != "" {
			r.Warnings = append(r.Warnings, "The game's Options file was not found - start the game once, then apply again to use the graphics preset")
		}
		return nil
	}
	if err != nil {
		return err
	}
	var data []byte
	if preset == "" {
		data, _ = patch.SetSettings(cur, g.Saved)
		g = graphicsState{}
	} else {
		var old []patch.Setting
		data, old = patch.SetSettings(cur, patch.GraphicsPresets[preset].Menu)
		if g.Preset == "" {
			g.Saved = old
		}
		g.Preset = preset
	}
	if !bytes.Equal(cur, data) {
		if err := writeAtomic(e.OptionsFile, data); err != nil {
			return err
		}
		r.Changed++
	}
	return saveJSON(e.graphicsPath(), g)
}
