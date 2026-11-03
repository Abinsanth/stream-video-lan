package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Library is the cached scan of one shared folder. It is safe for concurrent use.
type Library struct {
	root     string // absolute, as the user gave it (for display and joining)
	realRoot string // symlink-resolved root, for containment checks

	mu     sync.RWMutex
	videos []Video
	byPath map[string]Video
	err    error
}

// New opens root (which must be a folder) and runs the first scan.
func New(root string) (*Library, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(realRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", root)
	}
	l := &Library{root: abs, realRoot: realRoot}
	if err := l.Rescan(); err != nil {
		return nil, err
	}
	return l, nil
}

// Root returns the shared folder's absolute path as the user gave it.
func (l *Library) Root() string { return l.root }

// Videos returns a copy of the current list, in natural order.
func (l *Library) Videos() []Video {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.Clone(l.videos)
}

// Rescan replaces the list with a fresh scan. On error the list becomes empty and Err returns it.
func (l *Library) Rescan() error {
	vs, err := Scan(l.root)
	byPath := make(map[string]Video, len(vs))
	for _, v := range vs {
		byPath[v.RelPath] = v
	}
	l.mu.Lock()
	l.videos, l.byPath, l.err = vs, byPath, err
	l.mu.Unlock()
	return err
}

// Err returns the error from the latest scan, if any.
func (l *Library) Err() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.err
}

// Run rescans every interval until ctx is done.
func (l *Library) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.Rescan()
		}
	}
}
