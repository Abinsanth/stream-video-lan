package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Abinsanth/stream-video-lan/internal/awake"
	"github.com/Abinsanth/stream-video-lan/internal/config"
	"github.com/Abinsanth/stream-video-lan/internal/library"
	"github.com/Abinsanth/stream-video-lan/internal/viewers"
)

const testBase = "http://192.168.1.5:8080"

type fakeShare struct {
	root    string
	videos  []library.Video
	scanErr error
	urls    []string
	stops   int
}

func (f *fakeShare) Root() string            { return f.root }
func (f *fakeShare) Videos() []library.Video { return f.videos }
func (f *fakeShare) ScanErr() error          { return f.scanErr }
func (f *fakeShare) PlaylistURL() string     { return f.PlaylistURLs()[0] }
func (f *fakeShare) PlaylistURLs() []string {
	if len(f.urls) == 0 {
		return []string{testBase + "/playlist.m3u8"}
	}
	return f.urls
}
func (f *fakeShare) VideoURL(rel string) string { return testBase + "/v/" + rel }
func (f *fakeShare) Stop()                      { f.stops++ }

type fakeInh struct{ acquires, releases int }

func (f *fakeInh) Acquire() error { f.acquires++; return nil }
func (f *fakeInh) Release() error { f.releases++; return nil }

// testClock is a settable clock shared by a model's deps.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

// Tests run sequentially, so these hold the current test's clock and the command returned by
// the most recent update; production code knows nothing about them.
var (
	currentClock *testClock
	lastCmd      tea.Cmd
)

func newTestModel(t *testing.T, mutate func(*Deps)) Model {
	t.Helper()
	clk := &testClock{t: time.Unix(1_700_000_000, 0)}
	home := t.TempDir()
	d := Deps{
		Start:      func(dir string) (Share, error) { return &fakeShare{root: dir}, nil },
		SaveConfig: func(config.Config) error { return nil },
		CopyText:   func(string) error { return nil },
		Tracker:    viewers.New(clk.now),
		Keeper:     awake.NewKeeper(&fakeInh{}, clk.now),
		Now:        clk.now,
		Home:       home,
		GOOS:       "darwin",
		Version:    "dev",
	}
	if mutate != nil {
		mutate(&d)
	}
	currentClock = clk
	return update(NewModel(d), tea.WindowSizeMsg{Width: 80, Height: 20})
}

// pickerModel returns a model on the picker. Its home holds Movies/, Downloads/, Movies/Anime/
// and Movies/Anime/ep1.mkv; recentRel are recent folders relative to home.
func pickerModel(t *testing.T, recentRel []string) (Model, string) {
	t.Helper()
	var home string
	m := newTestModel(t, func(d *Deps) {
		home = d.Home
		for _, dir := range []string{"Movies/Anime", "Downloads"} {
			os.MkdirAll(filepath.Join(home, dir), 0o755)
		}
		os.WriteFile(filepath.Join(home, "Movies/Anime/ep1.mkv"), []byte("x"), 0o644)
		for _, r := range recentRel {
			d.Config.Recent = append(d.Config.Recent, filepath.Join(home, r))
		}
	})
	return m, home
}

func vid(rel string, size int64) library.Video { return library.Video{RelPath: rel, Size: size} }

// servingModel returns a model already serving a fakeShare with vids; *copied records CopyText.
func servingModel(t *testing.T, vids ...library.Video) (Model, *string) {
	return servingModelWithConfig(t, config.Config{TipShown: true}, vids...)
}

func servingModelWithConfig(t *testing.T, c config.Config, vids ...library.Video) (Model, *string) {
	t.Helper()
	copied := new(string)
	fs := &fakeShare{root: "/videos", videos: vids}
	m := newTestModel(t, func(d *Deps) {
		d.Config = c
		d.CopyText = func(s string) error { *copied = s; return nil }
		d.Start = func(string) (Share, error) { return fs, nil }
		d.InitialFolder = "/videos"
	})
	m = update(m, startReq{dir: "/videos"})
	if m.screen != screenServing {
		t.Fatal("not serving")
	}
	return m, copied
}

func withStart(m Model, f func(string) (Share, error)) Model { m.deps.Start = f; return m }
func withSave(m Model, f func(config.Config) error) Model    { m.deps.SaveConfig = f; return m }

func update(m Model, msg tea.Msg) Model {
	nm, cmd := m.Update(msg)
	lastCmd = cmd
	return nm.(Model)
}

func keyMsg(k string) tea.KeyPressMsg {
	named := map[string]tea.KeyPressMsg{
		"enter": {Code: tea.KeyEnter}, "tab": {Code: tea.KeyTab}, "shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
		"esc": {Code: tea.KeyEscape}, "backspace": {Code: tea.KeyBackspace}, "up": {Code: tea.KeyUp},
		"down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft}, "right": {Code: tea.KeyRight},
		"home": {Code: tea.KeyHome}, "end": {Code: tea.KeyEnd}, "pgup": {Code: tea.KeyPgUp},
		"pgdown": {Code: tea.KeyPgDown}, "ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	}
	if k, ok := named[k]; ok {
		return k
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		m = update(m, keyMsg(k))
	}
	return m
}

// updateCmd presses keys and returns the command from the last one.
func updateCmd(m Model, keys ...string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var nm tea.Model
		nm, cmd = m.Update(keyMsg(k))
		m = nm.(Model)
		lastCmd = cmd
	}
	return m, cmd
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func click(m Model, x, y int) Model {
	return update(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func advance(m Model, d time.Duration) Model {
	currentClock.t = currentClock.t.Add(d)
	return m
}

// selectRow moves the cursor until the selected row's label contains label.
func selectRow(m Model, label string) Model {
	for i := 0; i < 60; i++ {
		if strings.Contains(m.selectedLabel(), label) {
			return m
		}
		m = press(m, "down")
	}
	panic("no row containing " + label + "; at " + m.selectedLabel())
}

// runCmds runs the command returned by the most recent update, and whatever it leads to, for up
// to 10 rounds, feeding messages back in.
func runCmds(m Model) Model {
	cmds := []tea.Cmd{lastCmd}
	for round := 0; round < 10 && len(cmds) > 0; round++ {
		var next []tea.Cmd
		for _, c := range cmds {
			if c == nil {
				continue
			}
			msg := c()
			if b, ok := msg.(tea.BatchMsg); ok {
				next = append(next, b...)
				continue
			}
			if _, isTick := msg.(tickMsg); isTick {
				continue // don't start the refresh loop inside tests
			}
			nm, nc := m.Update(msg)
			m = nm.(Model)
			next = append(next, nc)
		}
		cmds = next
	}
	return m
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if isQuit(c) {
				return true
			}
		}
	}
	return false
}

func view(m Model) string { return m.View().Content }
