package main

import "testing"

func TestWindowSizeFitsTheScreen(t *testing.T) {
	cases := []struct{ dpi, workW, workH, w, h int }{
		{96, 1920, 1040, 1240, 820},   // full HD at 100%
		{96, 1366, 728, 1240, 688},    // 1366x768 laptop: clamp to the work area
		{192, 3840, 2080, 2480, 1640}, // 4K at 200%: scale up
		{144, 1920, 1040, 1860, 1000}, // 1080p at 150%: scaled size would not fit
	}
	for _, c := range cases {
		w, h := windowSize(c.dpi, c.workW, c.workH)
		if w != c.w || h != c.h {
			t.Errorf("dpi %d work %dx%d: got %dx%d, want %dx%d", c.dpi, c.workW, c.workH, w, h, c.w, c.h)
		}
	}
}
