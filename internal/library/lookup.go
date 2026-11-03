package library

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrNotFound means the requested path is not a servable video in the shared folder.
var ErrNotFound = errors.New("not found")

// Open maps a URL path (already unescaped) to a scanned video and opens it for reading.
//
// It succeeds only if rel is exactly a RelPath in the current scan, its symlink-resolved path is
// inside the real root, and the file is a regular file. The open itself goes through an os.Root
// for the real root, so a file swapped for an outside symlink between the check and the open is
// refused by the kernel-level walk rather than followed. It opens non-blocking, so a FIFO swapped
// in can't hang the request. The caller closes the file.
func (l *Library) Open(rel string) (*os.File, Video, error) {
	if rel == "" || strings.ContainsRune(rel, 0) {
		return nil, Video{}, ErrNotFound
	}
	l.mu.RLock()
	v, ok := l.byPath[rel]
	l.mu.RUnlock()
	if !ok {
		return nil, Video{}, ErrNotFound
	}
	real, err := filepath.EvalSymlinks(filepath.Join(l.root, filepath.FromSlash(rel)))
	if err != nil || !within(l.realRoot, real) {
		return nil, Video{}, ErrNotFound
	}
	inRoot, err := filepath.Rel(l.realRoot, real)
	if err != nil {
		return nil, Video{}, ErrNotFound
	}
	root, err := os.OpenRoot(l.realRoot)
	if err != nil {
		return nil, Video{}, ErrNotFound
	}
	defer root.Close()
	f, err := root.OpenFile(inRoot, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, Video{}, ErrNotFound
	}
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, Video{}, ErrNotFound
	}
	return f, v, nil
}
