package web

import (
	"os"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/icons"
)

// TestAchievementArtworkLoadsFromTheGame checks, against a real game folder, that every achievement's artwork
// (MarvelUIIcons_Achievements textures) decodes, so the Achievement hunter never shows a blank badge.
func TestAchievementArtworkLoadsFromTheGame(t *testing.T) {
	game := os.Getenv("MHO_GAME")
	if _, err := os.Stat(game); game == "" || err != nil {
		t.Skip("set MHO_GAME to the game folder")
	}
	set := icons.New(game)
	if _, err := set.PNG("MarvelUIIcons_Achievements.Achievement_Boss_CosmicMandarin"); err != nil {
		t.Fatalf("Mandarin's artwork must load: %v", err)
	}
	d, err := db.LoadDrops()
	if err != nil {
		t.Fatal(err)
	}
	none := 0
	for _, a := range d.Achievements {
		if a.Icon == "" {
			none++
			continue
		}
		if _, err := set.PNG(a.Icon); err != nil {
			t.Errorf("%s: %s does not load: %v", a.Name, a.Icon, err)
		}
	}
	t.Logf("%d achievements, %d without artwork in the data", len(d.Achievements), none)
}
