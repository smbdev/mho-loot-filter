package main

import "sync"

// detector searches for game installs in the background, so a slow drive never holds up the window.
type detector struct {
	mu     sync.Mutex
	found  []string
	done   bool
	notify []func([]string)
}

func newDetector(scan func() []string) *detector {
	d := &detector{}
	go func() {
		found := scan()
		d.mu.Lock()
		d.found, d.done = found, true
		callbacks := d.notify
		d.notify = nil
		d.mu.Unlock()
		for _, fn := range callbacks {
			fn(found)
		}
	}()
	return d
}

// Results returns what has been found so far and whether the search has finished.
func (d *detector) Results() ([]string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string{}, d.found...), d.done
}

// OnDone calls fn with the results once the search finishes (straight away if it already has).
func (d *detector) OnDone(fn func([]string)) {
	d.mu.Lock()
	if !d.done {
		d.notify = append(d.notify, fn)
		d.mu.Unlock()
		return
	}
	found := d.found
	d.mu.Unlock()
	fn(found)
}
