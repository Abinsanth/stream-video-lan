package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x", "config.json")
	c := Config{Recent: []string{"/a"}, TipShown: true}
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	if got := Load(p); !reflect.DeepEqual(got, c) {
		t.Fatal(got)
	}
	if fi, err := os.Stat(filepath.Dir(p)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatal("config dir not private", fi.Mode(), err)
	}
}

func TestLoadMissingOrCorrupt(t *testing.T) {
	dir := t.TempDir()
	if got := Load(filepath.Join(dir, "none.json")); !reflect.DeepEqual(got, Config{}) {
		t.Fatal(got)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(bad); !reflect.DeepEqual(got, Config{}) {
		t.Fatal(got)
	}
}

func TestAddRecent(t *testing.T) {
	var c Config
	for _, d := range []string{"/1", "/2", "/3", "/4", "/5", "/6", "/3"} {
		c.AddRecent(d)
	}
	if want := []string{"/3", "/6", "/5", "/4", "/2"}; !slices.Equal(c.Recent, want) {
		t.Fatal(c.Recent)
	}
}

func TestExistingRecent(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "file")
	os.WriteFile(f, []byte("x"), 0o644)
	c := Config{Recent: []string{d, filepath.Join(d, "gone"), f}}
	if got := c.ExistingRecent(); !slices.Equal(got, []string{d}) {
		t.Fatal(got)
	}
}

func TestDirHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if d, _ := Dir(); d != "/tmp/xdg/lanstream" {
		t.Fatal(d)
	}
	if p, _ := Path(); p != "/tmp/xdg/lanstream/config.json" {
		t.Fatal(p)
	}
}

func TestDirDefaultsToDotConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/u")
	if d, _ := Dir(); d != "/home/u/.config/lanstream" {
		t.Fatal(d)
	}
}
