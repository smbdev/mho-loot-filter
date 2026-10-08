package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mholootfilter/internal/engine"
)

var vdfPath = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)*)"`)

// libraryPaths returns the Steam library folders listed in a libraryfolders.vdf file.
func libraryPaths(vdf string) []string {
	var out []string
	for _, m := range vdfPath.FindAllStringSubmatch(vdf, -1) {
		out = append(out, strings.ReplaceAll(m[1], `\\`, `\`))
	}
	return out
}

// findInstalls returns every game folder found in the given Steam libraries, plus any extra candidates that
// hold the game, without duplicates.
func findInstalls(libraries, candidates []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		key := strings.ToLower(filepath.Clean(dir))
		if !seen[key] && engine.ValidGameDir(dir) == nil {
			seen[key] = true
			out = append(out, dir)
		}
	}
	for _, c := range candidates {
		add(c)
	}
	for _, lib := range libraries {
		// steamapps/common holds normal installs; steamapps/content/app_226320 holds depot downloads.
		for _, sub := range []string{filepath.Join("steamapps", "common"), filepath.Join("steamapps", "content", "app_226320")} {
			entries, err := os.ReadDir(filepath.Join(lib, sub))
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					add(filepath.Join(lib, sub, e.Name()))
				}
			}
		}
	}
	return out
}

// scanForInstalls looks for game folders under each root, at most depth folders down. It does not look inside
// a game folder it has found, and skips hidden and system folders.
func scanForInstalls(roots []string, depth int) []string {
	var out []string
	var walk func(dir string, left int)
	walk = func(dir string, left int) {
		if engine.ValidGameDir(dir) == nil {
			out = append(out, dir)
			return
		}
		if left == 0 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") || skipFolders[strings.ToLower(name)] {
				continue
			}
			walk(filepath.Join(dir, name), left-1)
		}
	}
	for _, root := range roots {
		walk(root, depth)
	}
	return out
}

var skipFolders = map[string]bool{
	"windows": true, "programdata": true, "appdata": true, "system volume information": true,
	"recovery": true, "node_modules": true, "msocache": true,
}
