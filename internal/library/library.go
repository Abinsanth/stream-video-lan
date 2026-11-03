package library

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Library is the cached scan of one shared folder. It is safe for concurrent use.
type Library struct {
	root     string // absolute, as the user gave it (for display and joining)
	realRoot string // symlink-resolved root, for containment checks

	scanMu    sync.Mutex // serialises scans; see Rescan
	lastStart time.Time  // when the most recent scan began (guarded by scanMu)

	mu     sync.RWMutex
	videos []Video
	byPath map[string]Video
	err    error
}

// New opens root (which must be a folder) and runs the first scan.
func New(root string) (*Library, error) {
	abs, real, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}
	l := &Library{root: abs, realRoot: real}
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
//
// Concurrent calls coalesce: a caller that waited while another scan ran, where that scan began
// after this call was made, reuses its result instead of scanning again. So a burst of playlist
// requests plus the background ticker costs one or two scans, and a slow disk can't pile them up.
func (l *Library) Rescan() error {
	asked := time.Now()
	l.scanMu.Lock()
	defer l.scanMu.Unlock()
	if l.lastStart.After(asked) {
		return l.Err()
	}
	l.lastStart = time.Now()
	vs, err := scanFn(l.root, l.realRoot)
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
