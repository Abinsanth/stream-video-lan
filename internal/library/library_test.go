package library

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestRescanPicksUpNewFiles(t *testing.T) {
	root := t.TempDir()
	lib := must(New(root))
	if len(lib.Videos()) != 0 {
		t.Fatal("want empty")
	}
	mk(t, root, "new.mkv")
	if err := lib.Rescan(); err != nil {
		t.Fatal(err)
	}
	if got := relPaths(lib.Videos()); !slices.Equal(got, []string{"new.mkv"}) {
		t.Fatal(got)
	}
}

func TestRescanRootRemoved(t *testing.T) {
	root := filepath.Join(t.TempDir(), "drive")
	mk(t, root, "a.mp4")
	lib := must(New(root))
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := lib.Rescan(); err == nil {
		t.Fatal("want error")
	}
	if len(lib.Videos()) != 0 || lib.Err() == nil {
		t.Fatal("want empty list and Err set")
	}
}

func TestNewRejectsFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.mp4")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(f); err == nil {
		t.Fatal("want error for non-directory")
	}
}

func TestRootIsAbsoluteAsGiven(t *testing.T) {
	root := t.TempDir()
	lib := must(New(root))
	if lib.Root() != root {
		t.Fatalf("Root() = %q, want %q", lib.Root(), root)
	}
}

func TestVideosReturnsCopy(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "a.mp4")
	lib := must(New(root))
	vs := lib.Videos()
	vs[0].RelPath = "changed"
	if lib.Videos()[0].RelPath != "a.mp4" {
		t.Fatal("Videos() exposed internal slice")
	}
}

func TestRunRescansPeriodically(t *testing.T) {
	root := t.TempDir()
	lib := must(New(root))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go lib.Run(ctx, 10*time.Millisecond)
	mk(t, root, "late.mp4")
	deadline := time.Now().Add(2 * time.Second)
	for len(lib.Videos()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("Run did not pick up the new file")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
