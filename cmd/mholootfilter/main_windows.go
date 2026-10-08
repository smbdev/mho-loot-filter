//go:build windows

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"mholootfilter/internal/web"
)

const webView2Download = "https://developer.microsoft.com/microsoft-edge/webview2/"

func message(text string, icon uint32) {
	windows.MessageBox(0, windows.StringToUTF16Ptr(text), windows.StringToUTF16Ptr("MHO Loot Filter"), icon)
}

// steamLibraries returns the Steam library folders listed by the local Steam install.
func steamLibraries() []string {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	steam, _, err := key.GetStringValue("SteamPath")
	if err != nil {
		return nil
	}
	steam = filepath.FromSlash(steam)
	libs := []string{steam}
	for _, rel := range []string{`steamapps\libraryfolders.vdf`, `config\libraryfolders.vdf`} {
		if b, err := os.ReadFile(filepath.Join(steam, rel)); err == nil {
			libs = append(libs, libraryPaths(string(b))...)
		}
	}
	return libs
}

// runningGameDir returns the game folder of a running MarvelHeroesOmega.exe, or "".
func runningGameDir() string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "MarvelHeroesOmega.exe") {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
		windows.CloseHandle(h)
		if err == nil {
			// <game>\UnrealEngine3\Binaries\Win64\MarvelHeroesOmega.exe
			return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(windows.UTF16ToString(buf[:size])))))
		}
	}
	return ""
}

// scanRoots are the likely places for a copy of the game that did not come from Steam, such as an archived
// client extracted to Downloads, with how many folders deep to look in each.
func scanRoots() (user []string, drives []string) {
	if home, err := os.UserHomeDir(); err == nil {
		for _, sub := range []string{"Downloads", "Desktop", "Documents", "Games"} {
			user = append(user, filepath.Join(home, sub))
		}
	}
	for letter := 'C'; letter <= 'Z'; letter++ {
		root := string(letter) + `:\`
		// Only local fixed disks: network, removable and optical drives can be slow or ask for media.
		if windows.GetDriveType(windows.StringToUTF16Ptr(root)) == windows.DRIVE_FIXED {
			drives = append(drives, root)
		}
	}
	return user, drives
}

// detectInstalls finds the game in Steam libraries, the running game, and common folders.
func detectInstalls() []string {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	candidates := []string{runningGameDir(), dir, filepath.Dir(dir), defaultSteamGame}
	user, drives := scanRoots()
	candidates = append(candidates, scanForInstalls(user, 3)...)
	candidates = append(candidates, scanForInstalls(drives, 2)...)
	return findInstalls(steamLibraries(), candidates)
}

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
)

// screenWindowSize sizes the window for the main display's scaling and work area.
func screenWindowSize() (uint, uint) {
	dpi := 96
	if procGetDpiForSystem.Find() == nil {
		if r, _, _ := procGetDpiForSystem.Call(); r != 0 {
			dpi = int(r)
		}
	}
	var area windows.Rect
	const spiGetWorkArea = 0x0030
	if r, _, _ := procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&area)), 0); r == 0 {
		area = windows.Rect{Right: 1366, Bottom: 728}
	}
	w, h := windowSize(dpi, int(area.Right-area.Left), int(area.Bottom-area.Top))
	return uint(w), uint(h)
}

func main() {
	game := flag.String("game", "", "game folder (contains UnrealEngine3)")
	data := flag.String("data", defaultDataDir(), "settings and backup folder")
	flag.Parse()

	mutex, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Local\MHOLootFilter`))
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		message("MHO Loot Filter is already open.", windows.MB_ICONINFORMATION)
		return
	}
	defer windows.CloseHandle(mutex)

	d, e, opts, err := newApp(*game, *data, newDetector(detectInstalls))
	if err != nil {
		message("The item database is damaged: "+err.Error(), windows.MB_ICONERROR)
		return
	}
	page, err := web.InlinePage()
	if err != nil {
		message("The app files are damaged: "+err.Error(), windows.MB_ICONERROR)
		return
	}

	width, height := screenWindowSize()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(*data, "webview"),
		WindowOptions: webview2.WindowOptions{
			Title:  "MHO Loot Filter",
			Width:  width,
			Height: height,
			IconId: 1,
			Center: true,
		},
	})
	if w == nil {
		message("MHO Loot Filter needs the Microsoft Edge WebView2 Runtime, which is part of Windows 11.\n\n"+
			"Install it from "+webView2Download+" and start the filter again.", windows.MB_ICONERROR)
		return
	}
	defer w.Destroy()
	opts.PickFolder = func() (string, error) { return pickFolder(uintptr(w.Window())) }
	call := bridge(web.New(d, e, opts))
	// lfCall runs on the window's thread: used for the folder dialog, which must be owned by the window.
	// lfStart runs everything else in the background and posts the answer back, so the window never freezes.
	err = w.Bind("lfCall", func(method, path, body string) string { return call(strings.ToUpper(method), path, body) })
	if err == nil {
		err = w.Bind("lfStart", func(id int, method, path, body string) {
			go func() {
				result := call(strings.ToUpper(method), path, body)
				w.Dispatch(func() { w.Eval(fmt.Sprintf("window.lfDone(%d, %s)", id, jsonString(result))) })
			}()
		})
	}
	if err != nil {
		message("Could not start the filter window: "+err.Error(), windows.MB_ICONERROR)
		return
	}
	w.SetHtml(page)
	w.Run()
}
