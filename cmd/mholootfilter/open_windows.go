//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"mholootfilter/internal/launch"

	"golang.org/x/sys/windows"
)

var procShellExecute = shell32.NewProc("ShellExecuteW")

// openURL opens a web page in the default browser. The filter runs as administrator, so the page is handed to
// Explorer, which opens it in the user's normal browser instead of an elevated one.
func openURL(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString("explorer.exe")
	args, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	const showNormal = 1
	r, _, _ := procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)), 0, showNormal)
	if r <= 32 { // ShellExecute returns a value above 32 on success
		return fmt.Errorf("could not open the browser (error %d)", r)
	}
	return nil
}

// launchGame starts the game the chosen way: Steam through its link, anything else as a program with arguments.
func launchGame(w launch.Way) error {
	if w.URL != "" {
		return openURL(w.URL)
	}
	quoted := make([]string, len(w.Args))
	for i, a := range w.Args {
		quoted[i] = windows.EscapeArg(a)
	}
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(w.Exe)
	if err != nil {
		return err
	}
	args, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return err
	}
	dir, err := windows.UTF16PtrFromString(w.Dir)
	if err != nil {
		return err
	}
	const showNormal = 1
	r, _, _ := procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(dir)), showNormal)
	if r <= 32 {
		return fmt.Errorf("could not start %s (error %d)", filepath.Base(w.Exe), r)
	}
	return nil
}
