//go:build darwin

package library

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWrongCaseRootKeepsAbsoluteInRootSymlinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "Movies")
	mk(t, root, "real/in.mp4")
	symlink(t, filepath.Join(root, "real"), filepath.Join(root, "absalias"))
	lower := filepath.Join(base, "movies")
	if _, err := os.Stat(lower); err != nil {
		t.Skip("case-sensitive volume")
	}
	lib := must(New(lower))
	if got := relPaths(lib.Videos()); !slices.Equal(got, []string{"absalias/in.mp4", "real/in.mp4"}) {
		t.Fatal(got)
	}
	f, _, err := lib.Open("absalias/in.mp4")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestWrongCaseRootServesPlainFilesAndRelativeSymlinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "Movies")
	mk(t, root, "a.mp4", "real/in.mp4")
	if err := os.Symlink("real", filepath.Join(root, "rel")); err != nil { // relative, inside
		t.Fatal(err)
	}
	lower := filepath.Join(base, "movies")
	if _, err := os.Stat(lower); err != nil {
		t.Skip("case-sensitive volume")
	}
	lib := must(New(lower))
	if got := relPaths(lib.Videos()); !slices.Equal(got, []string{"a.mp4", "real/in.mp4", "rel/in.mp4"}) {
		t.Fatal(got)
	}
	for _, rel := range []string{"a.mp4", "rel/in.mp4"} {
		f, _, err := lib.Open(rel)
		if err != nil {
			t.Fatalf("Open(%q): %v", rel, err)
		}
		f.Close()
	}
	if lib.Root() != root {
		t.Fatalf("Root() = %q, want the on-disk spelling %q", lib.Root(), root)
	}
}
