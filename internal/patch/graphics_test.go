package patch

import (
	"slices"
	"strings"
	"testing"
)

func TestPresetsSetTheSameMenuKeys(t *testing.T) {
	keys := func(s []Setting) (k []string) {
		for _, x := range s {
			k = append(k, x.Key)
		}
		return k
	}
	want := keys(GraphicsPresets["potato"].Menu)
	for name, p := range GraphicsPresets {
		if !slices.Equal(keys(p.Menu), want) {
			t.Errorf("%s sets other menu keys", name)
		}
		for _, s := range append(p.Menu, p.Extra...) {
			if s.Value == "" {
				t.Errorf("%s: %s has no value", name, s.Key)
			}
		}
	}
}

func TestSetSettingsChangesOnlyTheKeysItSets(t *testing.T) {
	ini := "[SystemSettings]\r\nResX=2560\r\nbloom=True\r\nMotionBlur=False\r\n\r\n[AppCompat]\r\nUserGraphicsLevel=0\r\n"
	got, old := SetSettings([]byte(ini), []Setting{{"Bloom", "False"}, {"DepthOfField", "True"}, {"MotionBlur", ""}})
	want := "[SystemSettings]\r\nResX=2560\r\nBloom=False\r\nDepthOfField=True\r\n\r\n[AppCompat]\r\nUserGraphicsLevel=0\r\n"
	if string(got) != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if !slices.Equal(old, []Setting{{"Bloom", "True"}, {"DepthOfField", ""}, {"MotionBlur", "False"}}) {
		t.Fatalf("old values %v", old)
	}
	back, _ := SetSettings(got, old)
	if string(back) != strings.Replace(ini, "bloom=", "Bloom=", 1) {
		t.Errorf("putting the old values back gave %q", back)
	}
	if got, _ := SetSettings([]byte("[AppCompat]\r\nX=1"), []Setting{{"Bloom", "False"}}); string(got) != "[SystemSettings]\r\nBloom=False\r\n[AppCompat]\r\nX=1" {
		t.Errorf("no section: %q", got)
	}
	if got, _ := SetSettings([]byte("[SystemSettings]\r\nResX=1"), []Setting{{"Bloom", "False"}}); string(got) != "[SystemSettings]\r\nResX=1\r\nBloom=False\r\n" {
		t.Errorf("last line without newline: %q", got)
	}
}

func TestPresetExtrasGoIntoSystemSettings(t *testing.T) {
	sys := string(Tweak(nil, SystemSettingsIni, Tweaks{Graphics: "potato", LowLag: true}))
	if strings.Count(sys, "[SystemSettings]") != 1 || !strings.Contains(sys, "OneFrameThreadLag=False\r\nLensFlares=False\r\n") {
		t.Errorf("system settings: %q", sys)
	}
	if strings.Contains(sys, "Bloom=") {
		t.Error("menu settings belong in the Options file")
	}
	if eng := string(Tweak(nil, EngineIni, Tweaks{Graphics: "potato"})); eng != "" {
		t.Errorf("preset in the engine file: %q", eng)
	}
	if (Tweaks{Graphics: "ultra"}).Validate() == nil {
		t.Error("unknown preset accepted")
	}
}
