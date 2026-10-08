package main

import (
	"os"
	"path/filepath"
	"testing"
)

const vdf = `"libraryfolders"
{
	"0"
	{
		"path"		"C:\\Program Files (x86)\\Steam"
		"apps"		{ "228980"		"0" }
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
	}
}`

func TestLibraryPaths(t *testing.T) {
	got := libraryPaths(vdf)
	if len(got) != 2 || got[0] != `C:\Program Files (x86)\Steam` || got[1] != `D:\SteamLibrary` {
		t.Fatalf("got %q", got)
	}
}

func TestFindInstalls(t *testing.T) {
	lib := t.TempDir()
	game := filepath.Join(lib, "steamapps", "common", "Marvel Heroes")
	for _, rel := range []string{"UnrealEngine3/MarvelGame/CookedPCConsole/MarvelGame.upk", "UnrealEngine3/Binaries/Win64/MarvelHeroesOmega.exe"} {
		p := filepath.Join(game, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	os.MkdirAll(filepath.Join(lib, "steamapps", "common", "Other Game"), 0o755)
	got := findInstalls([]string{lib, filepath.Join(lib, "missing")}, []string{game})
	if len(got) != 1 || got[0] != game {
		t.Fatalf("got %q (duplicates must be removed)", got)
	}
}

func makeGame(t *testing.T, dir string) {
	t.Helper()
	for _, rel := range []string{"UnrealEngine3/MarvelGame/CookedPCConsole/MarvelGame.upk", "UnrealEngine3/Binaries/Win64/MarvelHeroesOmega.exe"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
}

func TestScanFindsArchiveCopiesWithinDepth(t *testing.T) {
	root := t.TempDir()
	shallow := filepath.Join(root, "marvel-heroes-1.52.0.1700-omega-2.16a-steam", "Marvel Heroes Omega 2.16a Steam")
	deep := filepath.Join(root, "a", "b", "c", "d", "Marvel Heroes")
	makeGame(t, shallow)
	makeGame(t, deep)
	got := scanForInstalls([]string{root, filepath.Join(root, "missing")}, 3)
	if len(got) != 1 || got[0] != shallow {
		t.Fatalf("got %q, want only the copy within 3 levels", got)
	}
}

func TestSteamDepotDownloadsAreFound(t *testing.T) {
	lib := t.TempDir()
	depot := filepath.Join(lib, "steamapps", "content", "app_226320", "depot_226323")
	makeGame(t, depot)
	if got := findInstalls([]string{lib}, nil); len(got) != 1 || got[0] != depot {
		t.Fatalf("got %q", got)
	}
}
