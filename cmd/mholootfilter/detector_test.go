package main

import (
	"testing"
	"time"
)

func TestDetectorRunsInBackgroundAndReportsWhenDone(t *testing.T) {
	release := make(chan struct{})
	det := newDetector(func() []string { <-release; return []string{`D:\Games\Marvel Heroes`} })
	if found, done := det.Results(); done || len(found) != 0 {
		t.Fatal("results must not block while the search runs")
	}
	got := make(chan string, 1)
	det.OnDone(func(found []string) { got <- found[0] })
	close(release)
	select {
	case dir := <-got:
		if dir != `D:\Games\Marvel Heroes` {
			t.Fatalf("got %s", dir)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnDone never called")
	}
	if found, done := det.Results(); !done || len(found) != 1 {
		t.Fatalf("after the search: %v %v", found, done)
	}
}
