package wwise

import (
	"bytes"
	"math"
	"testing"
)

// The item drop sounds bank and the drop sound of the unused AITToken audio type in 2.16a.
const (
	itemsBank  = 1382876039
	tokenSound = 934977114
)

type media struct {
	id   uint32
	data []byte
}

func embeddedMedia(t *testing.T, bank []byte) []media {
	t.Helper()
	cs, err := chunks(bank)
	if err != nil {
		t.Fatal(err)
	}
	var didx, data []byte
	for _, c := range cs {
		switch c.tag {
		case "DIDX":
			didx = c.body
		case "DATA":
			data = c.body
		}
	}
	var out []media
	for i := 0; i+12 <= len(didx); i += 12 {
		off, size := le.Uint32(didx[i+4:]), le.Uint32(didx[i+8:])
		if off%dataAlign != 0 {
			t.Fatalf("media at %d not aligned", off)
		}
		out = append(out, media{le.Uint32(didx[i:]), data[off : off+size]})
	}
	return out
}

func TestReplaceSoundSwapsOnlyItsMedia(t *testing.T) {
	bank, err := Bank(testdata(t, "SFX_Shared_INT.pck"), itemsBank)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ReplaceSound(bank, tokenSound, Alert, 7)
	if err != nil {
		t.Fatal(err)
	}
	before, after := embeddedMedia(t, bank), embeddedMedia(t, out)
	if len(before) != len(after) {
		t.Fatal("media count changed")
	}
	const tokenMedia = 0x0cf18f65
	for i := range before {
		want := before[i].data
		if before[i].id == tokenMedia {
			want = Alert
		}
		if after[i].id != before[i].id || !bytes.Equal(after[i].data, want) {
			t.Fatalf("media %d wrong after replace", before[i].id)
		}
	}
	// The token sound is turned down 8 dB in the game's bank; the alert plays at the volume asked for.
	hirc := out[bytes.Index(out, []byte("HIRC")):]
	body := hirc[bytes.Index(hirc, le.AppendUint32(le.AppendUint32(nil, tokenSound), pluginPCM))+4:]
	if body[26] != 1 || body[27] != propVolume || math.Float32frombits(le.Uint32(body[28:])) != 7 {
		t.Fatalf("volume not cleared: % x", body[24:32])
	}
	if again, _ := ReplaceSound(out, tokenSound, Alert, 7); !bytes.Equal(again, out) {
		t.Fatal("replacing twice is not stable")
	}
	if _, err := ReplaceSound(bank, 12345, Alert, 7); err == nil {
		t.Fatal("expected error for a missing sound")
	}
}

func TestReplaceBankAppendsAndRepoints(t *testing.T) {
	pck := testdata(t, "SFX_Shared_INT.pck")
	nb := []byte("new bank contents")
	out, err := ReplaceBank(pck, itemsBank, nb)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Bank(out, itemsBank); err != nil || !bytes.Equal(got, nb) {
		t.Fatalf("bank not replaced: %v", err)
	}
	if !bytes.Equal(out[len(pck)-1000:len(pck)], pck[len(pck)-1000:]) {
		t.Fatal("existing data moved")
	}
	if !Extends(out, pck, itemsBank) || Extends(pck[:len(pck)-1], pck, itemsBank) {
		t.Fatal("Extends does not recognise a replaced bank")
	}
	tampered := append([]byte{}, out...)
	tampered[100]++
	if Extends(tampered, pck, itemsBank) {
		t.Fatal("Extends accepted a change to the original data")
	}
	if _, err := ReplaceBank([]byte("RIFF"), itemsBank, nb); err == nil {
		t.Fatal("expected error for a non-package")
	}
}

func TestPCMMatchesTheShippedAlertLayout(t *testing.T) {
	wav, err := WAV(Alert)
	if err != nil {
		t.Fatal(err)
	}
	data := wav[44:]
	samples := make([]int16, len(data)/2)
	for i := range samples {
		samples[i] = int16(le.Uint16(data[2*i:]))
	}
	if got := PCM(samples); !bytes.Equal(got[:64], Alert[:64]) || len(got) != len(Alert) {
		t.Fatalf("header differs from the built-in alert:\n% x\n% x", got[:64], Alert[:64])
	}
	quiet := PCM([]int16{0, 100, -50})
	if s := int16(le.Uint16(quiet[len(quiet)-4:])); s < 31000 {
		t.Fatalf("quiet sound not raised: peak %d", s)
	}
	if _, err := WAV([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
}
