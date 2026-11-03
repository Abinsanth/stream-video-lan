package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Abinsanth/stream-video-lan/internal/config"
)

func TestPickerListsRecentAndShortcuts(t *testing.T) {
	m, home := pickerModel(t, []string{"Movies/Anime"})
	v := view(m)
	for _, s := range []string{"Recent", "~/Movies/Anime", "Shortcuts", "~/Movies", "~/Downloads", "Browse…", "Type a path…"} {
		if !strings.Contains(v, s) {
			t.Errorf("missing %q in\n%s", s, v)
		}
	}
	if strings.Contains(v, "~/Desktop") {
		t.Error("non-existent shortcut shown")
	}
	m = update(m, countMsg{dir: filepath.Join(home, "Movies/Anime"), n: 1})
	if !strings.Contains(view(m), "1 video") {
		t.Error("count not shown")
	}
}

func TestPickerHidesGoneRecentAndEmptySections(t *testing.T) {
	m, _ := pickerModel(t, []string{"Gone"})
	if v := view(m); strings.Contains(v, "Recent") || strings.Contains(v, "Gone") {
		t.Fatal(v)
	}
}

func TestPickerCountCmdsCoverRecent(t *testing.T) {
	m, home := pickerModel(t, []string{"Movies/Anime"})
	cmd := m.picker.countCmds()
	if cmd == nil {
		t.Fatal("no count command")
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		msg = b[0]()
	}
	if c, ok := msg.(countMsg); !ok || c.dir != filepath.Join(home, "Movies/Anime") || c.n != 1 {
		t.Fatal(msg)
	}
}

func TestPickerEnterStartsShareAndSavesRecent(t *testing.T) {
	m, home := pickerModel(t, nil)
	var started string
	var saved config.Config
	m = withStart(m, func(d string) (Share, error) { started = d; return &fakeShare{root: d}, nil })
	m = withSave(m, func(c config.Config) error { saved = c; return nil })
	m = press(m, "enter") // first selectable row = ~/Movies (no recents)
	if started != filepath.Join(home, "Movies") || m.screen != screenServing || len(saved.Recent) == 0 || saved.Recent[0] != started {
		t.Fatal(started, m.screen, saved)
	}
}

func TestPickerStartErrorStays(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = withStart(m, func(string) (Share, error) { return nil, errors.New("ports 8080-8099 are all busy") })
	m = press(m, "enter")
	if m.screen != screenPicker || !strings.Contains(view(m), "ports 8080-8099 are all busy") {
		t.Fatal(view(m))
	}
}

func TestPickerQQuits(t *testing.T) {
	m, _ := pickerModel(t, nil)
	if _, cmd := updateCmd(m, "q"); !isQuit(cmd) {
		t.Fatal("q did not quit")
	}
}

func TestBrowseNavigateAndShare(t *testing.T) {
	m, home := pickerModel(t, nil)
	var started string
	m = withStart(m, func(d string) (Share, error) { started = d; return &fakeShare{root: d}, nil })
	m = selectRow(m, "Browse…")
	m = press(m, "enter")
	m = selectRow(m, "Movies")
	m = press(m, "enter")
	if !strings.Contains(view(m), "Anime") {
		t.Fatal(view(m))
	}
	m = press(m, "s")
	if started != filepath.Join(home, "Movies") {
		t.Fatal(started)
	}
}

func TestBrowseBackspaceGoesUp(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = selectRow(m, "Browse…")
	m = press(m, "enter")
	m = selectRow(m, "Movies")
	m = press(m, "enter", "backspace")
	if v := view(m); !strings.Contains(v, "Downloads") || strings.Contains(v, "Anime") {
		t.Fatal(v)
	}
}

func TestBrowseEscReturnsToList(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = selectRow(m, "Browse…")
	m = press(m, "enter", "esc")
	if !strings.Contains(view(m), "Type a path…") {
		t.Fatal(view(m))
	}
}

func TestBrowseUnreadableDirStays(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	m, home := pickerModel(t, nil)
	locked := filepath.Join(home, "Locked")
	os.Mkdir(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	m = selectRow(m, "Browse…")
	m = press(m, "enter")
	m = selectRow(m, "Locked")
	m = press(m, "enter")
	if v := view(m); !strings.Contains(v, "Can't open") || !strings.Contains(v, "Movies") {
		t.Fatal(v)
	}
}

func TestTypePathTabCompletes(t *testing.T) {
	m, home := pickerModel(t, nil)
	m = selectRow(m, "Type a path…")
	m = press(m, "enter")
	m = typeText(m, filepath.Join(home, "Mov"))
	m = press(m, "tab")
	if got := m.picker.input.Value(); got != filepath.Join(home, "Movies") {
		t.Fatal(got)
	}
}

func TestTypePathPasteDraggedFolder(t *testing.T) {
	m, home := pickerModel(t, nil)
	var started string
	m = withStart(m, func(d string) (Share, error) { started = d; return &fakeShare{root: d}, nil })
	m = selectRow(m, "Type a path…")
	m = press(m, "enter")
	m = update(m, tea.PasteMsg{Content: "'" + filepath.Join(home, "Movies", "Anime") + "' "})
	m = press(m, "enter")
	if started != filepath.Join(home, "Movies", "Anime") {
		t.Fatal(started)
	}
}

func TestTypePathQIsText(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = selectRow(m, "Type a path…")
	m = press(m, "enter")
	if _, cmd := updateCmd(m, "q"); isQuit(cmd) {
		t.Fatal("q quit while typing a path")
	}
}

func TestBrowseDoesNotCountHomeOrAbove(t *testing.T) {
	m, home := pickerModel(t, nil)
	if _, cmd := m.enterDir(home); cmd != nil {
		t.Fatal("entering home started a recursive count")
	}
	if _, cmd := m.enterDir("/"); cmd != nil {
		t.Fatal("entering / started a recursive count")
	}
	if _, cmd := m.enterDir(filepath.Dir(home)); cmd != nil {
		t.Fatal("entering a folder above home started a recursive count")
	}
	os.MkdirAll(filepath.Join(home, "Library", "Caches"), 0o755)
	if _, cmd := m.enterDir(filepath.Join(home, "Library", "Caches")); cmd != nil {
		t.Fatal("entering ~/Library started a recursive count")
	}
	if _, cmd := m.enterDir(filepath.Join(home, "Movies")); cmd == nil {
		t.Fatal("entering ~/Movies should count its videos")
	}
}
