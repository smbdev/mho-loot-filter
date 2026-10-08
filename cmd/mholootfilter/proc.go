package main

import "strings"

const gameExe = "MarvelHeroesOmega.exe"

// isRunning reports whether the game is among the running processes. If the list cannot be read it answers
// yes, so the filter never patches files the game might have open.
func isRunning(list func() ([]string, error)) bool {
	names, err := list()
	if err != nil {
		return true
	}
	for _, n := range names {
		if strings.EqualFold(n, gameExe) {
			return true
		}
	}
	return false
}
