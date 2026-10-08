package web

import (
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
