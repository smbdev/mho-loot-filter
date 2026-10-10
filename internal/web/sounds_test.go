package web

import (
	"testing"

	"mholootfilter/internal/db"
)

func TestSoundsGetAGroupAndWords(t *testing.T) {
	for name, want := range map[string][2]string{
		"play_vox_iga_ply_warmachine_interplay_captainamerica": {"Hero voices", "warmachine interplay captainamerica"},
		"play_vox_iga_npc_bishop_death_01":                     {"Other voices", "npc bishop death 01"},
		"play_sfx_ui_levelup":                                  {"Interface", "levelup"},
		"play_muisc_ageofultron":                               {"Music", "ageofultron"},
		"play_footstepgroundtypevsfootmaterial":                {"Other sounds", "footstepgroundtypevsfootmaterial"},
		"play_sfx_knc_glass_large":                             {"Objects", "glass large"},
	} {
		g, l := describeSound(db.Sound{Name: name})
		if g != want[0] || l != want[1] {
			t.Errorf("%s: got %q %q", name, g, l)
		}
	}
}
