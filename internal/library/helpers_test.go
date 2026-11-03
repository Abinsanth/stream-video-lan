package library

import (
	"os"
	"path/filepath"
	"testing"
)

// mk creates each relPath under root (with parent folders) as a 1-byte file.
func mk(t *testing.T, root string, relPaths ...string) {
	t.Helper()
	for _, rel := range relPaths {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func relPaths(vs []Video) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.RelPath
	}
	return out
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
