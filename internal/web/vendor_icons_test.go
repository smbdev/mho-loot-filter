package web

import (
	"os"
	"strings"
	"testing"

	"mholootfilter/internal/db"
	"mholootfilter/internal/icons"
)

// TestVendorStockIconsLoadFromTheGame checks, against a real game folder, that every item and service a vendor
// stocks has an icon, and that a crafter's service shows its crafting panel icon (RecipeIconPath, first in its
// chain) rather than falling back to the plain recipe scroll.
func TestVendorStockIconsLoadFromTheGame(t *testing.T) {
	game := os.Getenv("MHO_GAME")
	if _, err := os.Stat(game); game == "" || err != nil {
		t.Skip("set MHO_GAME to the game folder")
	}
	set := icons.New(game)
	if _, err := set.PNG("MarvelUIIcons.NoSuchTexture"); err == nil {
		t.Fatal("a missing texture must fail, or this test proves nothing")
	}
	if _, err := set.PNG("MarvelUIIcons.Item_Recipe_Generic"); err != nil {
		t.Fatalf("the plain recipe scroll must load: %v", err)
	}
	d, err := db.LoadDrops()
	if err != nil {
		t.Fatal(err)
	}
	checked, fellBack := map[int]bool{}, 0
	for _, s := range d.Sources {
		if s.Kind != "vendor" {
			continue
		}
		for _, row := range d.Tables[s.Table].Rows {
			it := d.Items[row.Item]
			if it.Random || checked[row.Item] || !strings.Contains(it.Path, "/") {
				continue
			}
			checked[row.Item] = true
			chain := strings.Split(it.Icon, "|")
			if _, err := firstIcon(it.Icon, set.PNG); err != nil {
				t.Errorf("%s (%s, at %s): no icon loads from %q", it.Name, it.Path, s.Name, it.Icon)
				continue
			}
			if _, err := set.PNG(chain[0]); err != nil {
				if len(it.Service) > 0 {
					t.Errorf("service %s (%s): its crafting panel icon %s is missing, it would show %q", it.Name, it.Path, chain[0], chain[1:])
				}
				fellBack++
				t.Logf("falls back: %s (%s): %s missing", it.Name, it.Path, chain[0])
			}
		}
	}
	t.Logf("%d vendor items and services checked, %d use a later icon in their chain", len(checked), fellBack)
}
