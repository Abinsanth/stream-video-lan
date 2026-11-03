//go:build darwin

package library

import (
	"os"
	"path/filepath"
	"strings"
)

// canonicalCase respells an absolute path with the letter case stored on disk, so that a folder
// typed as ~/movies compares equal to symlink targets that resolve to ~/Movies. macOS volumes
// are case-insensitive by default, which is why the typed spelling opens at all. On any error,
// or for a component that can't be found, the input is returned unchanged.
func canonicalCase(p string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	cur := string(filepath.Separator)
	for _, comp := range strings.Split(strings.Trim(p, string(filepath.Separator)), string(filepath.Separator)) {
		if comp == "" {
			continue
		}
		name, ok := onDiskName(cur, comp)
		if !ok {
			return p
		}
		cur = filepath.Join(cur, name)
	}
	return cur
}

func onDiskName(dir, comp string) (string, bool) {
	if _, err := os.Lstat(filepath.Join(dir, comp)); err != nil {
		return "", false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return comp, true // can't list (e.g. no read permission): keep the typed spelling
	}
	fold := ""
	for _, e := range entries {
		if e.Name() == comp {
			return comp, true
		}
		if fold == "" && strings.EqualFold(e.Name(), comp) {
			fold = e.Name()
		}
	}
	if fold != "" {
		return fold, true
	}
	return comp, true
}
