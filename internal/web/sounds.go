package web

import (
	"os"
	"strings"

	"mholootfilter/internal/db"
	"mholootfilter/internal/wwise"
)

// readPreview reads a sound's audio from a file package and converts it for the page.
func readPreview(path string, p *db.Preview) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	wem := make([]byte, p.Size)
	if _, err := f.ReadAt(wem, p.Offset); err != nil {
		return nil, err
	}
	return wwise.WemToOgg(wem)
}

// soundGroups are the groups of the Sounds page, in page order, with the event name parts that put a sound in each.
var soundGroups = []struct {
	label string
	parts []string // the words after play_ (and vox_iga_ for voices)
}{
	{"Hero voices", []string{"vox_iga_ply"}},
	{"Team-up voices", []string{"vox_iga_tup"}},
	{"Boss voices", []string{"vox_iga_bos"}},
	{"Enemy voices", []string{"vox_iga_mob"}},
	{"Other voices", []string{"vox"}},
	{"Powers", []string{"sfx_pwr"}},
	{"Status effects", []string{"sfx_cfx"}},
	{"Creatures", []string{"sfx_cvo", "sfx_cre", "sfx_fol", "sfx_fly", "sfx_hov", "sfx_afx", "sfx_mov"}},
	{"Objects", []string{"sfx_exp", "sfx_knc", "sfx_int"}},
	{"Surroundings", []string{"sfx_bg", "sfx_env"}},
	{"Interface", []string{"sfx_ui", "sfx_loot"}},
	{"Emotes", []string{"sfx_emo"}},
	{"Music", []string{"music", "muisc"}},
	{"Other sounds", []string{""}},
}

// describeSound puts a sound in a group and turns the rest of its event name into words, e.g.
// play_vox_iga_ply_warmachine_interplay_captainamerica -> Hero voices, "warmachine interplay captainamerica".
func describeSound(s db.Sound) (group, label string) {
	rest := strings.TrimPrefix(s.Name, "play_")
	for _, g := range soundGroups {
		for _, p := range g.parts {
			if p == "" || rest == p || strings.HasPrefix(rest, p+"_") {
				words := rest
				if p != "" {
					words = strings.TrimPrefix(rest, p+"_")
				}
				if p == "vox" {
					words = strings.TrimPrefix(words, "iga_")
				}
				return g.label, strings.ReplaceAll(words, "_", " ")
			}
		}
	}
	return "", rest
}
