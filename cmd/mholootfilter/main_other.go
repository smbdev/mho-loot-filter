//go:build !windows

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"mholootfilter/internal/web"
)

// defaultAddr is fixed so a second launch finds the copy already running instead of patching alongside it.
const defaultAddr = "127.0.0.1:47816"

// listen opens addr for the filter. If another copy of the filter already serves it, running is true.
func listen(addr string) (ln net.Listener, running bool, err error) {
	ln, err = net.Listen("tcp", addr)
	if err == nil {
		return ln, false, nil
	}
	client := http.Client{Timeout: 2 * time.Second}
	res, gerr := client.Get("http://" + addr + "/api/status")
	if gerr == nil {
		defer res.Body.Close()
		var status struct {
			GameDir *string `json:"gameDir"`
		}
		if json.NewDecoder(res.Body).Decode(&status) == nil && status.GameDir != nil {
			return nil, true, nil
		}
	}
	return nil, false, fmt.Errorf("port %s is used by another program: %w", addr, err)
}

// main serves the UI over HTTP on non-Windows systems, for development and automated UI tests.
func main() {
	game := flag.String("game", "", "game folder (contains UnrealEngine3)")
	data := flag.String("data", defaultDataDir(), "settings and backup folder")
	addr := flag.String("addr", defaultAddr, "local address to serve the filter on")
	flag.Bool("no-browser", true, "kept for compatibility")
	flag.Parse()
	d, e, opts, err := newApp(*game, *data, newDetector(func() []string { return findInstalls(nil, []string{*game}) }))
	if err != nil {
		fmt.Println("item database is damaged:", err)
		os.Exit(1)
	}
	ln, running, err := listen(*addr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if running {
		fmt.Println("MHO Loot Filter is already running: http://" + *addr + "/")
		return
	}
	fmt.Println("MHO Loot Filter is open in your browser: http://" + ln.Addr().String() + "/")
	if err := http.Serve(ln, web.New(d, e, opts)); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
