package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBridgeRoutesIntoHandler(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/echo", func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "127.0.0.1" {
			http.Error(w, `{"error":"bad host"}`, 403)
			return
		}
		var v map[string]any
		json.NewDecoder(r.Body).Decode(&v)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	})
	call := bridge(mux)
	var res struct {
		Status int            `json:"status"`
		Body   map[string]any `json:"body"`
	}
	if err := json.Unmarshal([]byte(call("PUT", "/api/echo", `{"a":1}`)), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Body["a"].(float64) != 1 {
		t.Fatalf("got %+v", res)
	}
	json.Unmarshal([]byte(call("GET", "/api/missing", "")), &res)
	if res.Status != 404 {
		t.Fatalf("missing route: %+v", res)
	}
}
