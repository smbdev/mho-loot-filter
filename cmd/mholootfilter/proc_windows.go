//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func processNames() ([]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var names []string
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		names = append(names, windows.UTF16ToString(entry.ExeFile[:]))
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return nil, err
	}
	return names, nil
}

func gameRunning() bool { return isRunning(processNames) }
