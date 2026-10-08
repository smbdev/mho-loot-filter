//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	ole32                   = windows.NewLazySystemDLL("ole32.dll")
	procSHBrowseForFolder   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
)

// browseInfo is the Windows BROWSEINFOW structure.
type browseInfo struct {
	owner       uintptr
	root        uintptr
	displayName *uint16
	title       *uint16
	flags       uint32
	callback    uintptr
	param       uintptr
	image       int32
}

const (
	bifReturnOnlyFSDirs = 0x0001
	bifEditBox          = 0x0010
	bifNewDialogStyle   = 0x0040
)

// pickFolder shows the Windows folder browser owned by the given window. It returns "" when cancelled.
func pickFolder(owner uintptr) (string, error) {
	name := make([]uint16, windows.MAX_PATH)
	info := browseInfo{
		owner:       owner,
		displayName: &name[0],
		title:       windows.StringToUTF16Ptr("Choose the Marvel Heroes Omega folder (the one that contains UnrealEngine3 and Data)."),
		flags:       bifReturnOnlyFSDirs | bifEditBox | bifNewDialogStyle,
	}
	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&info)))
	if pidl == 0 {
		return "", nil
	}
	defer procCoTaskMemFree.Call(pidl)
	path := make([]uint16, windows.MAX_LONG_PATH)
	if ok, _, err := procSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&path[0]))); ok == 0 {
		return "", err
	}
	return windows.UTF16ToString(path), nil
}
