package web

import (
	"regexp"
	"strings"
	"testing"
)

func TestInlinePageIsSelfContained(t *testing.T) {
	page, err := InlinePage()
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{`href="style.css"`, `src="app.js"`, `src="logo.png"`, `url("oswald.woff2")`} {
		if strings.Contains(page, ref) {
			t.Errorf("page still references %s", ref)
		}
	}
	for _, want := range []string{"<style>", "data:font/woff2;base64,", "data:image/png;base64,", "function showPage"} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %s", want)
		}
	}
}

// A second function with a name already in use silently replaces the first one for every caller.
func TestAppDeclaresEachFunctionOnce(t *testing.T) {
	js, err := static.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^(?:async )?function (\w+)`).FindAllStringSubmatch(string(js), -1) {
		if seen[m[1]] {
			t.Errorf("function %s is declared twice", m[1])
		}
		seen[m[1]] = true
	}
}
