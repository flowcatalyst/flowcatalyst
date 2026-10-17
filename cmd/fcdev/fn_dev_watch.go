package main

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Directories never watched: dependencies, VCS state and build outputs. A
// build writing into one of these must not trigger the next build.
var devIgnoredDirs = map[string]bool{
	"node_modules": true, "dist": true, "target": true, ".git": true, "bin": true,
	"vendor": true, "build": true, "out": true,
}

// devIgnoredFile reports a file name that is a build output or editor
// scratch, never a source: the Go artifact, logs, swap and backup files.
func devIgnoredFile(name string) bool {
	switch {
	case strings.HasPrefix(name, "."):
		return true
	case strings.HasSuffix(name, ".wasm"), strings.HasSuffix(name, ".log"),
		strings.HasSuffix(name, ".swp"), strings.HasSuffix(name, "~"), strings.HasSuffix(name, ".tmp"):
		return true
	}
	return false
}

type fileStamp struct {
	mod  time.Time
	size int64
}

// pollWatcher notices source changes by comparing (mtime, size) of every
// source file under root on a timer. Polling keeps fcdev free of a
// file-notification dependency and behaves the same on every platform.
type pollWatcher struct {
	root     string
	interval time.Duration
	debounce time.Duration
	now      func() time.Time // injectable clock

	prev    map[string]fileStamp
	pending map[string]bool
	last    time.Time // when the newest pending change was seen
}

func newPollWatcher(root string, interval, debounce time.Duration) *pollWatcher {
	return &pollWatcher{root: root, interval: interval, debounce: debounce, now: time.Now}
}

// snapshot stamps every watched file under root.
func (w *pollWatcher) snapshot() map[string]fileStamp {
	out := map[string]fileStamp{}
	_ = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // vanished mid-walk; the next poll sees the truth
		}
		name := d.Name()
		if d.IsDir() {
			if path != w.root && (devIgnoredDirs[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if devIgnoredFile(name) || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(w.root, path)
		out[rel] = fileStamp{info.ModTime(), info.Size()}
		return nil
	})
	return out
}

// poll takes one snapshot. It returns the changed paths (sorted) once changes
// have been quiet for the debounce period, otherwise nil: a burst of saves is
// reported once, after it settles.
func (w *pollWatcher) poll() []string {
	cur := w.snapshot()
	if w.prev == nil {
		w.prev = cur
		return nil
	}
	if w.pending == nil {
		w.pending = map[string]bool{}
	}
	for p, st := range cur {
		if old, ok := w.prev[p]; !ok || old != st {
			w.pending[p] = true
			w.last = w.now()
		}
	}
	for p := range w.prev {
		if _, ok := cur[p]; !ok {
			w.pending[p] = true
			w.last = w.now()
		}
	}
	w.prev = cur
	if len(w.pending) == 0 || w.now().Sub(w.last) < w.debounce {
		return nil
	}
	out := make([]string, 0, len(w.pending))
	for p := range w.pending {
		out = append(out, p)
	}
	slices.Sort(out)
	w.pending = nil
	return out
}

// run polls until ctx ends, calling onChange with each settled batch.
func (w *pollWatcher) run(ctx context.Context, onChange func([]string)) {
	w.poll() // baseline
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if paths := w.poll(); paths != nil {
			onChange(paths)
		}
	}
}
