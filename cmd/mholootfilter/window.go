package main

// windowSize returns the window size in physical pixels: 1240x1040 at 100% scaling, tall enough for the whole
// sidebar (938 px of content) and the Game tweaks page under the title bar, scaled by the display's DPI and kept
// inside the work area (the screen minus the taskbar) with a small margin. On a full HD screen that leaves
// 1240x1000, which the sidebar still fits.
func windowSize(dpi, workW, workH int) (w, h int) {
	const margin = 40
	w, h = 1240*dpi/96, 1040*dpi/96
	if w > workW-margin {
		w = workW - margin
	}
	if h > workH-margin {
		h = workH - margin
	}
	return w, h
}
