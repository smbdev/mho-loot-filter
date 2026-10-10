package patch

import (
	"strings"
	"testing"
)

func TestTweaksAreAddedAndRemovedCleanly(t *testing.T) {
	orig := []byte("[Engine.Engine]\r\nMaxSmoothedFrameRate=200\r\n")
	if got := Tweak(orig, EngineIni, Tweaks{}); string(got) != string(orig) {
		t.Fatalf("no tweaks changed the file: %q", got)
	}
	got := Tweak(orig, EngineIni, Tweaks{FPS: 144, SkipIntro: true, TextureMB: 1024, LowLag: true})
	for _, want := range []string{"MaxSmoothedFrameRate=144\r\n", "[FullScreenMovie]\r\n!StartupMovies=ClearArray\r\n", "[TextureStreaming]\r\nPoolSize=1024\r\n"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	again := Tweak(got, EngineIni, Tweaks{FPS: NoFPSLimit})
	if !strings.HasPrefix(string(again), string(orig)) || strings.Contains(string(again), "PoolSize") || !strings.Contains(string(again), "bSmoothFrameRate=FALSE") {
		t.Errorf("retweak kept old tweaks or lost the original: %q", again)
	}
	if string(Untweak(again)) != string(orig) {
		t.Errorf("untweak did not give back the original: %q", Untweak(again))
	}
	if Tweak([]byte("x"), EngineIni, Tweaks{FPS: 60})[1] != '\r' {
		t.Error("tweaks must start on a line of their own")
	}
	if strings.Contains(string(got), "OneFrameThreadLag") {
		t.Error("system settings in the engine file")
	}
	sys := string(Tweak(orig, SystemSettingsIni, Tweaks{LowLag: true, FPS: 60}))
	for _, want := range []string{"[SystemSettings]\r\nOneFrameThreadLag=False\r\n"} {
		if !strings.Contains(sys, want) {
			t.Errorf("missing %q in %q", want, sys)
		}
	}
	if added := sys[len(orig):]; strings.Contains(added, "MaxSmoothedFrameRate") || strings.Contains(added, "TEXTUREGROUP") {
		t.Errorf("wrong lines in the system settings: %q", sys)
	}
	for _, bad := range []Tweaks{{FPS: 10}, {FPS: 5000}, {TextureMB: 100}, {TextureMB: 99999}} {
		if bad.Validate() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if (Tweaks{FPS: NoFPSLimit, TextureMB: 2048}).Validate() != nil {
		t.Error("valid tweaks rejected")
	}
}
