//go:build windows

package main

import "os/exec"

// openURL opens a web page in the default browser. The filter runs as administrator; handing the page to the
// running Explorer opens it in the user's normal browser instead of an elevated one.
func openURL(url string) error {
	return exec.Command("explorer.exe", url).Start()
}
