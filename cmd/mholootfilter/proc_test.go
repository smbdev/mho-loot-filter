package main

import (
	"errors"
	"testing"
)

func TestGameRunningFailsSafe(t *testing.T) {
	list := func(names []string, err error) func() ([]string, error) {
		return func() ([]string, error) { return names, err }
	}
	if !isRunning(list([]string{"explorer.exe", "MarvelHeroesOmega.exe"}, nil)) {
		t.Fatal("running game not detected")
	}
	if !isRunning(list([]string{"MARVELHEROESOMEGA.EXE"}, nil)) {
		t.Fatal("process names are case-insensitive on Windows")
	}
	if isRunning(list([]string{"explorer.exe"}, nil)) {
		t.Fatal("game reported running when it is not")
	}
	if !isRunning(list(nil, errors.New("access denied"))) {
		t.Fatal("when the process list cannot be read, the game must be treated as running")
	}
}
