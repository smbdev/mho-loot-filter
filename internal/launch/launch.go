// Package launch finds the ways the player starts the game: a Bifrost launcher in the game folder, or Steam.
package launch

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Way is one way to start the game. Either URL is opened by the shell, or Exe runs with Args in Dir.
type Way struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	URL  string   `json:"-"`
	Exe  string   `json:"-"`
	Args []string `json:"-"`
	Dir  string   `json:"-"`
}

// SteamURL starts Marvel Heroes Omega through Steam, which adds the player's launch options.
const SteamURL = "steam://rungameid/226320"

// Find returns the ways to start the game in gameDir: every Bifrost in the folder or one level below it, then Steam
// when the game is a Steam install.
func Find(gameDir string) []Way {
	var ways []Way
	dirs := []string{gameDir}
	if ents, err := os.ReadDir(gameDir); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(gameDir, e.Name()))
			}
		}
	}
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "Bifrost.exe")); err == nil {
			ways = append(ways, bifrost(gameDir, dir))
		}
	}
	if strings.Contains(strings.ToLower(filepath.ToSlash(gameDir)), "/steamapps/") {
		ways = append(ways, Way{ID: "steam", Name: "Steam", URL: SteamURL})
	}
	return ways
}

type server struct{ Name, SiteConfigUrl string }

// config is Bifrost.LaunchConfig.json. Bifrost writes every field, so missing ones need no defaults.
type config struct {
	ServerIndex                                       int
	NoStartupMovies, NoSplash, ForceCustomResolution  bool
	CustomResolutionX, CustomResolutionY              int
	NoHomeDirectory, EnableLocaleOverride             bool
	LocaleOverride                                    string
	EnableAutoLogin                                   bool
	AutoLoginEmailAddress, AutoLoginPassword          string
	CustomArguments                                   string
	EnableLogging, OverrideLoggingLevel               bool
	LoggingLevel                                      int
	LoggingChannelStateDict                           map[string]int
	Downloader                                        int
	Force32Bit                                        bool
	NoSound, NoAccount, NoOptions, NoStore, NoCatalog bool
	NoNews, NoLogout                                  bool
}

var loggingLevels = []string{"NONE", "CRITICAL", "FATAL", "ERROR", "WARNING", "INFORMATION", "VERBOSE", "EXTRA_VERBOSE", "DEBUG"}

var loggingChannels = []string{"ALL", "ERROR", "CORE", "CORE_NET", "CORE_JOBS_TP", "GAME", "PEER_CONNECTOR", "DATASTORE", "PROFILE",
	"GAME_NETWORK", "PAKFILE_SYSTEM", "LOOT_MANAGER", "GROUPING_SYSTEM", "PROTOBUF_DUMPER", "GAME_DATABASE", "TRANSITION", "AI",
	"INVENTORY", "MEMORY", "MISSIONS", "PATCHER", "GENERATION", "RESPAWN", "SAVELOAD", "FRONTEND", "COMMUNITY", "ACHIEVEMENTS",
	"METRICS_HTTP_UPLOAD", "CURRENCY_CONVERSION", "MOBILE", "UI", "LEADERBOARD"}

// bifrost starts the game with the server and settings chosen in the Bifrost in dir, as Bifrost's own Play does
// (Bifrost.Core LaunchConfig.ToLaunchArguments). A Bifrost that has not saved its settings yet is opened instead.
func bifrost(gameDir, dir string) Way {
	id := "bifrost:" + filepath.Base(dir)
	if dir == gameDir {
		id = "bifrost:"
	}
	var servers []server
	var c config
	if !readJSON(filepath.Join(dir, "Bifrost.ServerList.json"), &servers) || len(servers) == 0 ||
		!readJSON(filepath.Join(dir, "Bifrost.LaunchConfig.json"), &c) {
		name := "Bifrost"
		if dir != gameDir {
			name += " (" + filepath.Base(dir) + " folder)"
		}
		return Way{ID: id, Name: name, Exe: filepath.Join(dir, "Bifrost.exe"), Dir: dir}
	}
	s := servers[0]
	if c.ServerIndex >= 0 && c.ServerIndex < len(servers) {
		s = servers[c.ServerIndex]
	}
	var args []string
	add := func(on bool, a ...string) {
		if on {
			args = append(args, a...)
		}
	}
	switch c.Downloader {
	case 0:
		args = append(args, "-robocopy", "-nosolidstate", "-nosteam")
	case 1:
		args = append(args, "-solidstate", "-nosteam")
	case 2:
		args = append(args, "-steam", "-nosolidstate")
	}
	args = append(args, "-siteconfigurl="+s.SiteConfigUrl)
	add(c.NoStartupMovies, "-nostartupmovies")
	add(c.NoSplash, "-nosplash")
	add(c.ForceCustomResolution, "-ResX="+strconv.Itoa(c.CustomResolutionX), "-ResY="+strconv.Itoa(c.CustomResolutionY))
	add(c.NoHomeDirectory, "-nohomedir")
	add(c.EnableLocaleOverride, "-locale="+c.LocaleOverride)
	add(c.EnableAutoLogin, "-emailaddress="+c.AutoLoginEmailAddress, "-password="+c.AutoLoginPassword)
	args = append(args, strings.Fields(c.CustomArguments)...)
	if c.EnableLogging {
		args = append(args, "-log")
		if c.OverrideLoggingLevel && c.LoggingLevel >= 0 && c.LoggingLevel < len(loggingLevels) {
			args = append(args, "-LoggingLevel="+loggingLevels[c.LoggingLevel])
		}
		var filter []string
		for _, ch := range loggingChannels {
			if v, ok := c.LoggingChannelStateDict[ch]; ok && v >= 0 {
				filter = append(filter, map[bool]string{true: "+", false: "-"}[v > 0]+ch)
			}
		}
		add(len(filter) > 0, "-LoggingChannels="+strings.Join(filter, ","))
	}
	for _, f := range []struct {
		on  bool
		arg string
	}{{c.NoSound, "-nosound"}, {c.NoAccount, "-noaccount"}, {c.NoOptions, "-nooptions"}, {c.NoStore, "-nostore"},
		{c.NoCatalog, "-nocatalog"}, {c.NoNews, "-nonews"}, {c.NoLogout, "-nologout"}} {
		add(f.on, f.arg)
	}
	bin := "Win64"
	if c.Force32Bit {
		bin = "Win32"
	}
	exeDir := filepath.Join(gameDir, "UnrealEngine3", "Binaries", bin)
	return Way{ID: id, Name: s.Name, Exe: filepath.Join(exeDir, "MarvelHeroesOmega.exe"), Args: args, Dir: exeDir}
}

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) // .NET may write a byte order mark
	return json.Unmarshal(b, v) == nil
}
