package main

// windowSize returns the window size in physical pixels: 1240x820 at 100% scaling, scaled by the display's DPI
// and kept inside the work area (the screen minus the taskbar) with a small margin.
func windowSize(dpi, workW, workH int) (w, h int) {
	const margin = 40
	w, h = 1240*dpi/96, 820*dpi/96
	if w > workW-margin {
		w = workW - margin
	}
	if h > workH-margin {
		h = workH - margin
	}
	return w, h
}
