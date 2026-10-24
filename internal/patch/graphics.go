package patch

import (
	"bytes"
	"strings"
)

// Setting is one key of the [SystemSettings] section. An empty Value stands for a key the file does not have.
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GraphicsPreset is a set of graphics settings: Menu are the ones the game's Advanced graphics Options have, Extra
// the ones it does not.
type GraphicsPreset struct{ Menu, Extra []Setting }

// GraphicsPresets by name. Every preset sets the same Menu keys, so the values saved before the first preset cover
// any later one. Left out on purpose: ScreenPercentage (clicks land off the cursor), TEXTUREGROUP sizes (black floors)
// and decal settings beyond the Options (they draw the warnings of boss attacks).
var GraphicsPresets = map[string]GraphicsPreset{
	"potato":   preset(0),
	"balanced": preset(1),
	"pretty":   preset(2),
}

// Each row: the key, then the potato, balanced and pretty values.
var (
	menuValues = [][4]string{
		{"AllowOtherPlayerFXOnPowers", "False", "True", "True"},
		{"AllowDecalsOnPowers", "False", "True", "True"},
		{"AllowHighQualityLights", "False", "True", "True"},
		{"bAllowHighQualityMaterials", "False", "True", "True"},
		{"AmbientOcclusion", "False", "False", "True"},
		{"bAllowLightShafts", "False", "False", "True"},
		{"Bloom", "False", "True", "True"},
		{"DepthOfField", "False", "False", "True"},
		{"MotionBlur", "False", "False", "True"},
		{"Distortion", "False", "True", "True"},
		{"DropParticleDistortion", "True", "False", "False"},
		{"DynamicShadows", "False", "True", "True"},
		{"FogVolumes", "False", "True", "True"},
		{"StaticDecals", "False", "True", "True"},
		{"DetailMode", "0", "2", "2"},
		{"SkeletalMeshLODBias", "1", "0", "0"},
		{"StaticMeshLODBias", "1", "1", "0"},
		{"MaxAnisotropy", "0", "4", "16"},
		{"PostProcessAAType", "0", "1", "6"},
		{"ShadowFilterQualityBias", "-1", "-1", "1"},
		{"MaxActiveDecals", "20", "50", "80"},
		{"MaxActiveEffects", "50", "200", "300"},
		{"MaxDeadBodies", "5", "15", "30"},
		{"MinTimeDLETick", "5000", "1000", "250"},
		{"ParticleLODBias", "2", "0", "0"},
		{"ParticleLODWithFramerate", "True", "True", "False"},
		{"ParticleScoreCutoff", "100", "350", "800"},
		{"MaxParticleVertexMemory", "33668", "66436", "131972"},
		{"PhysicsDetailLevel", "0", "1", "2"},
	}
	extraValues = [][4]string{
		{"LensFlares", "False", "True", "True"},
		{"LightEnvironmentShadows", "False", "True", "True"},
		{"SHSecondaryLighting", "False", "True", "True"},
		{"AllowRadialBlur", "False", "True", "True"},
		{"AllowImageReflections", "False", "True", "True"},
		{"AllowImageReflectionShadowing", "False", "True", "True"},
		{"bAllowFracturedDamage", "False", "True", "True"},
		{"MaxShadowResolution", "512", "1024", "2048"},
		{"MaxWholeSceneDominantShadowResolution", "1024", "2048", "4096"},
		{"MaxDrawDistanceScale", "0.8", "1", "1"},
	}
)

func preset(i int) GraphicsPreset {
	pick := func(rows [][4]string) []Setting {
		s := make([]Setting, len(rows))
		for j, r := range rows {
			s[j] = Setting{r[0], r[i+1]}
		}
		return s
	}
	return GraphicsPreset{Menu: pick(menuValues), Extra: pick(extraValues)}
}

// SetSettings returns ini with each setting written into its [SystemSettings] section, an empty value removing the
// key, and what the file had for each key before. The rest of the file is kept as it is.
func SetSettings(ini []byte, settings []Setting) ([]byte, []Setting) {
	lines := strings.SplitAfter(string(ini), "\n")
	start, end := -1, len(lines) // the section's lines, after its header
	for i, l := range lines {
		h := strings.TrimSpace(l)
		if start < 0 && strings.EqualFold(h, "[SystemSettings]") {
			start = i + 1
		} else if start >= 0 && strings.HasPrefix(h, "[") {
			end = i
			break
		}
	}
	if start < 0 {
		lines = append([]string{"[SystemSettings]\r\n"}, lines...)
		start, end = 1, 1
	}
	section := lines[start:end]
	for end > start && strings.TrimSpace(lines[end-1]) == "" { // add after the last key, before blank lines
		end--
	}
	old := make([]Setting, len(settings))
	var added []string
	for i, s := range settings {
		old[i] = Setting{Key: s.Key}
		found := false
		for j, l := range section {
			k, v, ok := strings.Cut(strings.TrimRight(l, "\r\n"), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(k), s.Key) {
				continue
			}
			if !found {
				old[i].Value = v
			}
			found = true
			if s.Value == "" {
				section[j] = ""
			} else {
				section[j] = s.Key + "=" + s.Value + "\r\n"
			}
		}
		if !found && s.Value != "" {
			added = append(added, s.Key+"="+s.Value+"\r\n")
		}
	}
	var b bytes.Buffer
	for i, l := range lines {
		if i == end {
			for _, a := range added {
				b.WriteString(a)
			}
		}
		b.WriteString(l)
	}
	if end == len(lines) {
		if b.Len() > 0 && !bytes.HasSuffix(b.Bytes(), []byte("\n")) && len(added) > 0 {
			b.WriteString("\r\n")
		}
		for _, a := range added {
			b.WriteString(a)
		}
	}
	return b.Bytes(), old
}
