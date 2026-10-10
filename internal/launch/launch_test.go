package launch

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBifrostIsStartedTheWayBifrostWould(t *testing.T) {
	game := t.TempDir()
	coa := filepath.Join(game, "Council of Ancients")
	write(t, filepath.Join(coa, "Bifrost.exe"), "")
	write(t, filepath.Join(coa, "Bifrost.ServerList.json"), `[
  {"Name": "Council of Ancients", "SiteConfigUrl": "mho.councilofancients.com/SiteConfig.xml"},
  {"Name": "Tahiti", "SiteConfigUrl": "mhtahiti.com/SiteConfig.xml"}]`)
	write(t, filepath.Join(coa, "Bifrost.LaunchConfig.json"), `{"ServerIndex": 1, "NoStartupMovies": true, "NoSplash": true,
  "ForceCustomResolution": false, "CustomArguments": "-a  -b", "EnableLogging": true, "OverrideLoggingLevel": true, "LoggingLevel": 3,
  "LoggingChannelStateDict": {"ALL": -1, "GAME": 1, "AI": 0}, "Downloader": 0, "Force32Bit": false, "NoNews": true}`)
	// a Bifrost that was never started has no settings yet: it is opened instead
	write(t, filepath.Join(game, "Tahiti", "Bifrost.exe"), "")

	ways := Find(game)
	if len(ways) != 2 {
		t.Fatalf("want 2 ways, got %+v", ways)
	}
	w := ways[0]
	wantArgs := []string{"-robocopy", "-nosolidstate", "-nosteam", "-siteconfigurl=mhtahiti.com/SiteConfig.xml", "-nostartupmovies",
		"-nosplash", "-a", "-b", "-log", "-LoggingLevel=ERROR", "-LoggingChannels=+GAME,-AI", "-nonews"}
	if w.Name != "Tahiti" || !slices.Equal(w.Args, wantArgs) {
		t.Errorf("got %q %q", w.Name, w.Args)
	}
	if w.Exe != filepath.Join(game, "UnrealEngine3", "Binaries", "Win64", "MarvelHeroesOmega.exe") || w.Dir != filepath.Dir(w.Exe) {
		t.Errorf("exe %q dir %q", w.Exe, w.Dir)
	}
	if o := ways[1]; o.Name != "Bifrost (Tahiti folder)" || o.Exe != filepath.Join(game, "Tahiti", "Bifrost.exe") || len(o.Args) != 0 {
		t.Errorf("unstarted Bifrost: %+v", o)
	}
	if ways[0].ID == ways[1].ID {
		t.Error("ways share an id")
	}
}

func TestSteamInstallsStartThroughSteam(t *testing.T) {
	game := filepath.Join(t.TempDir(), "SteamApps", "common", "Marvel Heroes")
	write(t, filepath.Join(game, "Bifrost.exe"), "")
	write(t, filepath.Join(game, "Bifrost.ServerList.json"), `[{"Name": "Council of Ancients", "SiteConfigUrl": "x/SiteConfig.xml"}]`)
	write(t, filepath.Join(game, "Bifrost.LaunchConfig.json"), `{"ServerIndex": 5, "Force32Bit": true, "Downloader": 2}`)
	ways := Find(game)
	if len(ways) != 2 || ways[1].Name != "Steam" || ways[1].URL != "steam://rungameid/226320" {
		t.Fatalf("got %+v", ways)
	}
	w := ways[0]
	if w.Name != "Council of Ancients" || !slices.Equal(w.Args, []string{"-steam", "-nosolidstate", "-siteconfigurl=x/SiteConfig.xml"}) ||
		filepath.Base(filepath.Dir(w.Exe)) != "Win32" {
		t.Errorf("out of range server index or 32-bit: %+v", w)
	}
	if ways := Find(t.TempDir()); len(ways) != 0 {
		t.Errorf("no launcher should find nothing: %+v", ways)
	}
}
