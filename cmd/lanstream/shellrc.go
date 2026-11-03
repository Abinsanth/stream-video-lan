package main

import (
	"os"
	"path/filepath"
	"strings"
)

// pathMarker is the comment install.sh writes above the PATH line it adds to a shell startup
// file; uninstall removes exactly that comment and the line after it, nothing else.
const pathMarker = "# Added by lanstream installer"

// fishFile is the file install.sh creates for fish users; it holds only our two lines.
func fishFile(home string) string {
	return filepath.Join(home, ".config", "fish", "conf.d", "lanstream.fish")
}

// rcFilesWithPath lists the shell startup files that contain the installer's PATH lines.
func rcFilesWithPath(home string) []string {
	var out []string
	for _, name := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile"} {
		p := filepath.Join(home, name)
		if b, err := os.ReadFile(p); err == nil && hasMarker(string(b)) {
			out = append(out, p)
		}
	}
	if b, err := os.ReadFile(fishFile(home)); err == nil && hasMarker(string(b)) {
		out = append(out, fishFile(home))
	}
	return out
}

func hasMarker(s string) bool {
	for _, l := range strings.Split(s, "\n") {
		if l == pathMarker {
			return true
		}
	}
	return false
}

// removePathLines deletes the marker and the line after it from p (keeping its permissions),
// or deletes p entirely when it is the fish file the installer created.
func removePathLines(p string, home string) error {
	if p == fishFile(home) {
		return os.Remove(p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	kept := lines[:0]
	for i := 0; i < len(lines); i++ {
		if lines[i] == pathMarker {
			i++ // skip the PATH line written with it
			continue
		}
		kept = append(kept, lines[i])
	}
	fi, err := os.Stat(p)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".lanstream-rc-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	if _, err := tmp.WriteString(strings.Join(kept, "\n")); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(fi.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
