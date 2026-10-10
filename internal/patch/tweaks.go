package patch

import (
	"bytes"
	"fmt"

	"mholootfilter/internal/cursor"
)

// TweakFiles are the game settings files the tweaks go into. The game builds its settings from them at the next start.
var TweakFiles = []string{EngineIni, SystemSettingsIni}

const (
	EngineIni         = "UnrealEngine3/MarvelGame/Config/DefaultEngine.ini"
	SystemSettingsIni = "UnrealEngine3/MarvelGame/Config/DefaultSystemSettings.ini"
)

// Tweaks are game settings the in-game Options do not offer. Zero values leave the game's own setting.
type Tweaks struct {
	FPS       int  `json:"fps"`       // frame rate limit; NoFPSLimit removes it (the game's is 200)
	SkipIntro bool `json:"skipIntro"` // no logo videos at start
	TextureMB int  `json:"textureMB"` // texture memory in MB (the game's is 160)
	LowLag    bool `json:"lowLag"`    // no extra frame between input and screen

	Pointer cursor.Style `json:"pointer"` // drawn into MarvelGame.upk, not a settings file
}

const NoFPSLimit = -1

// MinFPS, MaxFPS, MinTextureMB and MaxTextureMB bound the tweak values.
const MinFPS, MaxFPS, MinTextureMB, MaxTextureMB = 30, 1000, 160, 8192

func (t Tweaks) Validate() error {
	if t.FPS != 0 && t.FPS != NoFPSLimit && (t.FPS < MinFPS || t.FPS > MaxFPS) {
		return fmt.Errorf("The frame rate limit must be from %d to %d", MinFPS, MaxFPS)
	}
	if t.TextureMB != 0 && (t.TextureMB < MinTextureMB || t.TextureMB > MaxTextureMB) {
		return fmt.Errorf("Texture memory must be from %d to %d MB", MinTextureMB, MaxTextureMB)
	}
	return t.Pointer.Validate()
}

// tweaksMark starts the block the filter adds at the end of a settings file. Later lines override earlier ones.
const tweaksMark = "; MHO Loot Filter game tweaks - the filter removes everything from here to the end"

// Untweak returns ini without the filter's tweaks.
func Untweak(ini []byte) []byte {
	if i := bytes.Index(ini, []byte(tweaksMark)); i >= 0 {
		return ini[:i]
	}
	return ini
}

// Tweak returns settings file rel (one of TweakFiles) with t in place of any tweaks it has.
func Tweak(ini []byte, rel string, t Tweaks) []byte {
	ini = Untweak(ini)
	var b bytes.Buffer
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\r\n", a...) }
	switch rel {
	case EngineIni:
		if t.FPS == NoFPSLimit {
			line("[Engine.Engine]")
			line("bSmoothFrameRate=FALSE")
		} else if t.FPS != 0 {
			line("[Engine.Engine]")
			line("MaxSmoothedFrameRate=%d", t.FPS)
		}
		if t.SkipIntro {
			line("[FullScreenMovie]")
			line("!StartupMovies=ClearArray")
		}
		if t.TextureMB != 0 {
			line("[TextureStreaming]")
			line("PoolSize=%d", t.TextureMB)
		}
	case SystemSettingsIni:
		if t.LowLag {
			line("[SystemSettings]")
			line("OneFrameThreadLag=False")
		}
	}
	if b.Len() == 0 {
		return ini
	}
	out := append([]byte{}, ini...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, "\r\n"...)
	}
	return append(append(out, tweaksMark+"\r\n"...), b.Bytes()...)
}
