package main

import (
	"os"
	"strings"
	"testing"
)

func TestVersionMatchesWinres(t *testing.T) {
	raw, err := os.ReadFile("../../winres/winres.json")
	if err != nil {
		t.Fatal(err)
	}
	four := version // the exe's file version has four parts: 0.2.8 -> 0.2.8.0
	for strings.Count(four, ".") < 3 {
		four += ".0"
	}
	for _, want := range []string{`"ProductVersion": "` + version + `"`, `"FileVersion": "` + version + `"`, `"` + four + `"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("winres/winres.json does not contain %s", want)
		}
	}
}
