package library

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Video is one streamable file found under the shared root.
type Video struct {
	RelPath string // slash-separated, relative to root, e.g. "Show/S01E01.mkv"
	Size    int64
	ModTime time.Time
}

// Scan walks root recursively and returns its videos in NaturalLess order of RelPath.
// Symlinked folders and files are followed only when their real path is inside the real root;
// hidden/system entries and unreadable subfolders are skipped. It returns an error only when
// root itself can't be read.
func Scan(root string) ([]Video, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
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
	s := scanner{realRoot: realRoot, ancestors: map[string]bool{realRoot: true}}
	if err := s.walk(absRoot, "", realRoot, true); err != nil {
		return nil, err
	}
	slices.SortFunc(s.videos, func(a, b Video) int {
		switch {
		case NaturalLess(a.RelPath, b.RelPath):
			return -1
		case NaturalLess(b.RelPath, a.RelPath):
			return 1
		}
		return 0
	})
	return s.videos, nil
}

type scanner struct {
	realRoot  string
	ancestors map[string]bool // real paths of the folders on the current descent path
	videos    []Video
}

func (s *scanner) walk(dir, relDir, realDir string, top bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if top {
			return err
		}
		return nil // unreadable subfolder: skip it, keep scanning the rest
	}
	for _, e := range entries {
		name := e.Name()
		if skipName(name) {
			continue
		}
		p := filepath.Join(dir, name)
		rel := path.Join(relDir, name)
		realPath := filepath.Join(realDir, name)
		var info os.FileInfo
		if e.Type()&os.ModeSymlink != 0 {
			if realPath, err = filepath.EvalSymlinks(p); err != nil || !within(s.realRoot, realPath) {
				continue
			}
			if info, err = os.Stat(realPath); err != nil {
				continue
			}
		} else if info, err = e.Info(); err != nil {
			continue
		}
		switch {
		case info.IsDir():
			if s.ancestors[realPath] {
				continue // symlink loop
			}
			s.ancestors[realPath] = true
			s.walk(p, rel, realPath, false)
			delete(s.ancestors, realPath)
		case info.Mode().IsRegular() && IsVideo(name):
			s.videos = append(s.videos, Video{RelPath: rel, Size: info.Size(), ModTime: info.ModTime()})
		}
	}
	return nil
}

// within reports whether p is root itself or somewhere below it. Both must be cleaned, real paths.
func within(root, p string) bool {
	if p == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}
	return strings.HasPrefix(p, prefix)
}
