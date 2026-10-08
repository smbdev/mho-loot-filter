package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
)

// bridge lets the window's JavaScript call the API in-process: it runs the request through the handler and
// returns {"status": code, "body": json} as a string.
func bridge(h http.Handler) func(method, path, body string) string {
	return func(method, path, body string) string {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = "127.0.0.1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		payload := bytes.TrimSpace(rec.Body.Bytes())
		if !json.Valid(payload) {
			payload, _ = json.Marshal(map[string]string{"error": strings.TrimSpace(rec.Body.String())})
		}
		out, _ := json.Marshal(struct {
			Status int             `json:"status"`
			Body   json.RawMessage `json:"body"`
		}{rec.Code, payload})
		return string(out)
	}
}

// jsonString quotes s as a JSON string, which is also a valid JavaScript string literal.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
