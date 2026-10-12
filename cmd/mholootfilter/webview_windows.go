//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

// The window that hosts the WebView2 page. It finds the installed WebView2 runtime and loads it like any other
// DLL, instead of carrying Microsoft's loader DLL inside the exe and mapping it from memory, which antivirus
// machine learning reads as malware.

var (
	procRegisterClassEx    = user32.NewProc("RegisterClassExW")
	procCreateWindowEx     = user32.NewProc("CreateWindowExW")
	procDefWindowProc      = user32.NewProc("DefWindowProcW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procUpdateWindow       = user32.NewProc("UpdateWindow")
	procSetFocus           = user32.NewProc("SetFocus")
	procLoadImage          = user32.NewProc("LoadImageW")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procGetMessage         = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
	procIsDialogMessage    = user32.NewProc("IsDialogMessageW")
	procGetAncestor        = user32.NewProc("GetAncestor")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procPostThreadMessage  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentThreadId")
)

const (
	wmDestroy      = 0x0002
	wmMove         = 0x0003
	wmSize         = 0x0005
	wmActivate     = 0x0006
	wmQuit         = 0x0012
	wmNCLButtonDn  = 0x00A1
	wmMoving       = 0x0216
	wmApp          = 0x8000
	waInactive     = 0
	wsOverlappedWn = 0x00CF0000
	swShow         = 5
	gaRoot         = 2
	smCxScreen     = 0
	smCyScreen     = 1
	imageIcon      = 1
	lrDefaultSize  = 0x0040
	lrShared       = 0x8000
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type winMsg struct {
	hwnd    windows.HWND
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type webview struct {
	hwnd     uintptr
	ready    bool // the browser is in the window; messages before that must not reach it
	thread   uintptr
	browser  *edge.Chromium
	m        sync.Mutex
	queue    []func()
	bindings map[string]func(args []json.RawMessage) (any, error)
}

var errNoWebView2 = errors.New("WebView2 runtime not found")

// the one window, for wndProc
var mainWebview *webview

// newWebview opens the window with the page's browser in it. It fails when the WebView2 runtime is missing.
func newWebview(title string, width, height uint, dataPath string) (*webview, error) {
	if v, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(""); err != nil || v == "" {
		return nil, errNoWebView2
	}
	w := &webview{bindings: map[string]func([]json.RawMessage) (any, error){}}
	w.thread, _, _ = procGetCurrentThreadId.Call()
	w.browser = edge.NewChromium()
	w.browser.DataPath = dataPath
	w.browser.MessageCallback = func(msg string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		w.message(msg)
	}
	w.browser.SetErrorCallback(func(err error) {
		message("The filter window failed: "+err.Error(), windows.MB_ICONERROR)
		os.Exit(1) // some failures would otherwise leave the window waiting forever
	})
	w.browser.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)

	var instance windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &instance)
	icon, _, _ := procLoadImage.Call(uintptr(instance), 1, imageIcon, 0, 0, lrDefaultSize|lrShared)
	className := windows.StringToUTF16Ptr("MHOLootFilter")
	wc := wndClassEx{
		wndProc:   windows.NewCallback(wndProc),
		instance:  instance,
		icon:      windows.Handle(icon),
		iconSm:    windows.Handle(icon),
		className: className,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	mainWebview = w
	w.hwnd, _, _ = procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(title))), wsOverlappedWn,
		(sw-uintptr(width))/2, (sh-uintptr(height))/2, uintptr(width), uintptr(height), 0, 0, uintptr(instance), 0)
	if w.hwnd == 0 {
		return nil, errors.New("could not create the window")
	}
	procShowWindow.Call(w.hwnd, swShow)
	procUpdateWindow.Call(w.hwnd)
	procSetFocus.Call(w.hwnd)

	w.browser.Embed(w.hwnd)
	w.ready = true
	w.browser.Resize()
	settings, err := w.browser.GetSettings()
	if err != nil {
		return nil, err
	}
	if err := settings.PutAreDefaultContextMenusEnabled(false); err != nil {
		return nil, err
	}
	if err := settings.PutAreDevToolsEnabled(false); err != nil {
		return nil, err
	}
	return w, nil
}

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	w := mainWebview
	if w == nil || !w.ready || hwnd != w.hwnd {
		r, _, _ := procDefWindowProc.Call(hwnd, msg, wp, lp)
		return r
	}
	switch msg {
	case wmMove, wmMoving:
		_ = w.browser.NotifyParentWindowPositionChanged()
	case wmNCLButtonDn:
		procSetFocus.Call(hwnd)
		r, _, _ := procDefWindowProc.Call(hwnd, msg, wp, lp)
		return r
	case wmSize:
		w.browser.Resize()
	case wmActivate:
		if wp != waInactive {
			w.browser.Focus()
		}
	case wmDestroy:
		procPostQuitMessage.Call(0)
	default:
		r, _, _ := procDefWindowProc.Call(hwnd, msg, wp, lp)
		return r
	}
	return 0
}

// Run handles the window's messages until it closes.
func (w *webview) Run() {
	var msg winMsg
	for {
		procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch msg.message {
		case wmApp:
			w.m.Lock()
			q := w.queue
			w.queue = nil
			w.m.Unlock()
			for _, f := range q {
				f()
			}
		case wmQuit:
			return
		}
		root, _, _ := procGetAncestor.Call(uintptr(msg.hwnd), gaRoot)
		if r, _, _ := procIsDialogMessage.Call(root, uintptr(unsafe.Pointer(&msg))); r != 0 {
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Dispatch runs f on the window's thread.
func (w *webview) Dispatch(f func()) {
	w.m.Lock()
	w.queue = append(w.queue, f)
	w.m.Unlock()
	procPostThreadMessage.Call(w.thread, wmApp, 0, 0)
}

func (w *webview) Eval(js string)      { w.browser.Eval(js) }
func (w *webview) SetHtml(html string) { w.browser.NavigateToString(html) }

// Bind makes window.<name>(...args) in the page call f on the window's thread; it returns a promise of f's result.
func (w *webview) Bind(name string, f func(args []json.RawMessage) (any, error)) {
	w.m.Lock()
	w.bindings[name] = f
	w.m.Unlock()
	w.browser.Init("(function() { var name = " + jsonString(name) + ";" + `
		var RPC = window._rpc = (window._rpc || {nextSeq: 1});
		window[name] = function() {
			var seq = RPC.nextSeq++;
			var promise = new Promise(function(resolve, reject) { RPC[seq] = {resolve: resolve, reject: reject}; });
			window.external.invoke(JSON.stringify({id: seq, method: name, params: Array.prototype.slice.call(arguments)}));
			return promise;
		};
	})()`)
}

// message answers a call from a bound function.
func (w *webview) message(msg string) {
	var call struct {
		ID     int               `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.Unmarshal([]byte(msg), &call) != nil {
		return
	}
	w.m.Lock()
	f := w.bindings[call.Method]
	w.m.Unlock()
	if f == nil {
		return
	}
	settle := func(how, value string) {
		w.Dispatch(func() {
			w.Eval(fmt.Sprintf("window._rpc[%d].%s(%s); window._rpc[%d] = undefined", call.ID, how, value, call.ID))
		})
	}
	res, err := f(call.Params)
	if err == nil {
		var b []byte
		if b, err = json.Marshal(res); err == nil {
			settle("resolve", string(b))
			return
		}
	}
	settle("reject", jsonString(err.Error()))
}

// args decodes a bound function's arguments into ptrs.
func args(params []json.RawMessage, ptrs ...any) error {
	if len(params) != len(ptrs) {
		return errors.New("function arguments mismatch")
	}
	for i, p := range ptrs {
		if err := json.Unmarshal(params[i], p); err != nil {
			return err
		}
	}
	return nil
}
