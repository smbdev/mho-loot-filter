package db

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func gz(t *testing.T, s string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

const tiny = `{"version":1,"tiers":["Green","Red","Cosmic"],"labels":["event: X"],
 "items":[{"path":"A","name":"Alpha"},{"path":"B","name":"Beta"}],"pools":[],
 "places":[{"id":"p1","name":"Place One","group":"Terminals"}],
 "tables":[{"rows":[{"i":0,"pool":-1,"l":[],"p":[0.5,0.5,0.5],"n":[0.5,0.5,0.5],"s":0}]},
           {"rows":[{"i":0,"pool":-1,"l":[0],"p":[0,0,1],"n":[0,0,1],"s":1},{"i":1,"pool":-1,"l":[],"p":[1,1,1],"n":[1,1,1],"s":0}]}],
 "sources":[{"id":"S1","kind":"boss","name":"Zed","table":0,"place":0},{"id":"S2","kind":"boss","name":"Abe","table":1,"place":-1},
            {"id":"S3","kind":"box","name":"Box","table":0,"place":-1}]}`

func TestParseDropsIndexes(t *testing.T) {
	d, err := ParseDrops(gz(t, tiny))
	if err != nil {
		t.Fatal(err)
	}
	if d.ItemByPath["B"] != 1 || len(d.ItemSources[0]) != 3 || len(d.ItemSources[1]) != 1 {
		t.Fatalf("indexes: %v %v", d.ItemByPath, d.ItemSources)
	}
	if len(d.BossNames) != 2 || d.BossNames[0] != "Abe" || d.BossNames[1] != "Zed" {
		t.Fatalf("boss names: %v", d.BossNames)
	}
	if d.PlaceByID["p1"] != 0 || len(d.PlaceSources[0]) != 1 || d.PlaceSources[0][0] != 0 {
		t.Fatalf("place indexes: %v %v", d.PlaceByID, d.PlaceSources)
	}
}

func TestLoadDropsEmbedded(t *testing.T) {
	d, err := LoadDrops()
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != 1 || len(d.Sources) < 100 || len(d.BossNames) < 50 {
		t.Fatalf("embedded drops look empty: %d sources, %d bosses", len(d.Sources), len(d.BossNames))
	}
	if _, ok := d.ItemByPath["Entity/Items/Medals/MedalBlueprints/Endgame/CosmicMedallions/CosmicDoctorDoomEGM.prototype"]; !ok {
		t.Fatal("Cosmic Doctor Doom Medallion missing")
	}
}
