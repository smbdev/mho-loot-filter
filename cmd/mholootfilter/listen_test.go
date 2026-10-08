//go:build !windows

package main

import (
	"net"
	"net/http"
	"testing"
)

func TestListenReusesRunningCopy(t *testing.T) {
	first, running, err := listen("127.0.0.1:0")
	if err != nil || running {
		t.Fatalf("first listen: running=%v err=%v", running, err)
	}
	defer first.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"gameDir":""}`)) })
	go http.Serve(first, mux)

	second, running, err := listen(first.Addr().String())
	if err != nil || !running || second != nil {
		t.Fatalf("second listen should find the running copy: running=%v err=%v", running, err)
	}
}

func TestListenReportsPortTakenByAnotherProgram(t *testing.T) {
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, running, err := listen(other.Addr().String()); err == nil || running {
		t.Fatalf("expected an error for a port used by another program, got running=%v err=%v", running, err)
	}
}
