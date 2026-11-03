package library

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestScanFindsVideosRecursivelyInNaturalOrder(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "Show/S01E10.mkv", "Show/S01E2.mkv", "movie.MP4", "notes.txt", "sub.srt")
	got := relPaths(must(Scan(root)))
	want := []string{"movie.MP4", "Show/S01E2.mkv", "Show/S01E10.mkv"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestScanReportsSizeAndModTime(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	vs := must(Scan(root))
	if len(vs) != 1 || vs[0].Size != 5 || vs[0].ModTime.IsZero() {
		t.Fatalf("got %+v", vs)
	}
}

func TestScanSkipsHiddenAndSystem(t *testing.T) {
	root := t.TempDir()
	mk(t, root, ".hidden/a.mp4", "._b.mp4", "$RECYCLE.BIN/c.mp4", "System Volume Information/d.mp4", "ok.mp4")
	if got := relPaths(must(Scan(root))); !slices.Equal(got, []string{"ok.mp4"}) {
		t.Fatal(got)
	}
}

func TestScanSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	mk(t, root, "real/in.mp4")
	mk(t, outside, "secret.mp4")
	symlink(t, filepath.Join(root, "real"), filepath.Join(root, "alias"))             // inside → followed
	symlink(t, outside, filepath.Join(root, "escape"))                                // outside → skipped
	symlink(t, filepath.Join(outside, "secret.mp4"), filepath.Join(root, "leak.mp4")) // outside file → skipped
	symlink(t, root, filepath.Join(root, "real", "loop"))                             // loop → no hang
	got := relPaths(must(Scan(root)))
	if !slices.Equal(got, []string{"alias/in.mp4", "real/in.mp4"}) {
		t.Fatal(got)
	}
}

func TestScanSkipsUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	root := t.TempDir()
	mk(t, root, "locked/a.mp4", "open/b.mp4")
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	if got := relPaths(must(Scan(root))); !slices.Equal(got, []string{"open/b.mp4"}) {
		t.Fatal(got)
	}
}

func TestScanMissingRootErrors(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error")
	}
}
