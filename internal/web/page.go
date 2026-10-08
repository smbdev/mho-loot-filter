package web

import (
	"encoding/base64"
	"strings"
)

// InlinePage returns the UI as one HTML document with its stylesheet, script, font and logo embedded,
// for showing in a window that does not load files from a server.
func InlinePage() (string, error) {
	read := func(name string) (string, error) {
		b, err := static.ReadFile("static/" + name)
		return string(b), err
	}
	dataURL := func(name, mime string) (string, error) {
		b, err := static.ReadFile("static/" + name)
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), err
	}
	page, err := read("index.html")
	if err != nil {
		return "", err
	}
	css, err := read("style.css")
	if err != nil {
		return "", err
	}
	js, err := read("app.js")
	if err != nil {
		return "", err
	}
	font, err := dataURL("oswald.woff2", "font/woff2")
	if err != nil {
		return "", err
	}
	logo, err := dataURL("logo.png", "image/png")
	if err != nil {
		return "", err
	}
	css = strings.ReplaceAll(css, `url("oswald.woff2")`, `url("`+font+`")`)
	page = strings.Replace(page, `<link rel="stylesheet" href="style.css">`, "<style>"+css+"</style>", 1)
	page = strings.Replace(page, `<script src="app.js"></script>`, "<script>"+js+"</script>", 1)
	page = strings.ReplaceAll(page, `src="logo.png"`, `src="`+logo+`"`)
	return page, nil
}
