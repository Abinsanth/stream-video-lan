package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFound(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "Show/ep 1#?.mkv")
	lib := must(New(root))
	f, v, err := lib.Open("Show/ep 1#?.mkv")
	if err != nil || v.RelPath != "Show/ep 1#?.mkv" {
		t.Fatal(v, err)
	}
	f.Close()
}

func TestOpenRejectsTraversalAndUnknown(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "a.mp4", "notes.txt")
	lib := must(New(root))
	for _, rel := range []string{"../a.mp4", "/etc/passwd", "x/../a.mp4", "%2e%2e/a.mp4",
		"a.mp4\x00", "", "notes.txt", "missing.mp4", "./a.mp4", "/a.mp4"} {
		if f, _, err := lib.Open(rel); err == nil {
			f.Close()
			t.Errorf("Open(%q) succeeded", rel)
		}
	}
}

func TestOpenRejectsSymlinkSwappedAfterScan(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	mk(t, root, "a.mp4")
	mk(t, outside, "secret.mp4")
	lib := must(New(root))
	if err := os.Remove(filepath.Join(root, "a.mp4")); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(outside, "secret.mp4"), filepath.Join(root, "a.mp4"))
	if f, _, err := lib.Open("a.mp4"); err == nil {
		f.Close()
		t.Fatal("followed swapped symlink outside root")
	}
}

func TestOpenRootBehindSymlink(t *testing.T) {
	real := t.TempDir()
	mk(t, real, "a.mp4")
	link := filepath.Join(t.TempDir(), "link")
	symlink(t, real, link)
	lib := must(New(link)) // also covers macOS /var → /private/var
	if f, _, err := lib.Open("a.mp4"); err != nil {
		t.Fatal("root behind symlink rejected its own file")
	} else {
		f.Close()
	}
}

func TestOpenSymlinkInsideRootServed(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "real/in.mp4")
	symlink(t, filepath.Join(root, "real"), filepath.Join(root, "alias"))
	lib := must(New(root))
	if f, _, err := lib.Open("alias/in.mp4"); err != nil {
		t.Fatal("symlink inside root rejected", err)
	} else {
		f.Close()
	}
}

func TestOpenAbsoluteSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "real/in.mp4")
	symlink(t, filepath.Join(root, "real"), filepath.Join(root, "absalias")) // absolute target, inside
	lib := must(New(root))
	f, _, err := lib.Open("absalias/in.mp4")
	if err != nil {
		t.Fatal("absolute in-root symlink rejected:", err)
	}
	f.Close()
}
