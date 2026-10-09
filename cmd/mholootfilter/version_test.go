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
	for _, want := range []string{`"ProductVersion": "` + version + `"`, `"FileVersion": "` + version + `"`, `"` + version + `.0.0"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("winres/winres.json does not contain %s", want)
		}
	}
}
