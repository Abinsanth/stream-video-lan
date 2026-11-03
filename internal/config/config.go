// Package config stores lanstream's small amount of state: recent folders and whether the
// first-run tip was shown.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

const maxRecent = 5

// Config is everything lanstream remembers between runs.
type Config struct {
	Recent   []string `json:"recent"`
	TipShown bool     `json:"tip_shown"`
}

// Dir is lanstream's config folder: $XDG_CONFIG_HOME/lanstream, else ~/.config/lanstream
// (the same on macOS and Linux).
func Dir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "lanstream"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lanstream"), nil
}

// Path is the config file inside Dir.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load reads path. A missing or unreadable file gives an empty Config, never an error.
func Load(path string) Config {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &c) != nil {
		return Config{}
	}
	return c
}

// Save writes c to path atomically (temp file in the same folder, then rename).
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op after a successful rename
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// AddRecent puts dir first in the recent list, removing any older copy and keeping five.
func (c *Config) AddRecent(dir string) {
	c.Recent = slices.DeleteFunc(c.Recent, func(d string) bool { return d == dir })
	c.Recent = append([]string{dir}, c.Recent...)
	if len(c.Recent) > maxRecent {
		c.Recent = c.Recent[:maxRecent]
	}
}

// ExistingRecent returns the recent folders that still exist.
func (c Config) ExistingRecent() []string {
	var out []string
	for _, d := range c.Recent {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	return out
}
