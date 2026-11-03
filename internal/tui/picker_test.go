package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

func TestTypePathQIsText(t *testing.T) { // kept: q is text while typing a path
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

func TestPickerScrollsOnTinyHeight(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = update(m, tea.WindowSizeMsg{Width: 30, Height: 5})
	m = selectRow(m, "Type a path…")
	if !strings.Contains(view(m), "Type a path") {
		t.Fatal(view(m))
	}
}

// With no recent folders the picker list is: Shortcuts, ~/Movies, ~/Downloads, divider,
// Browse…, Type a path…, drawn from screen row 2.
func TestPickerClickSelects(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = click(m, 5, 4)
	if !strings.Contains(m.selectedLabel(), "Downloads") {
		t.Fatal(m.selectedLabel())
	}
}

func TestPickerDoubleClickOpens(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = click(m, 5, 6)
	m = click(m, 5, 6) // Browse…
	if m.picker.mode != pickBrowse {
		t.Fatal("double-click on Browse… did not open it")
	}
	m = selectRow(m, "Movies")
	y := 2 + m.picker.bcursor // no scrolling at this size
	m = advance(m, time.Second)
	m = click(m, 5, y)
	m = click(m, 5, y)
	if !strings.HasSuffix(m.picker.cwd, "Movies") {
		t.Fatal(m.picker.cwd)
	}
}

func TestPickerDoubleClickHeading(t *testing.T) {
	m, _ := pickerModel(t, nil)
	started := false
	m = withStart(m, func(d string) (Share, error) { started = true; return &fakeShare{root: d}, nil })
	before := m.selectedLabel()
	m = click(m, 5, 2)
	m = click(m, 5, 2) // "Shortcuts" heading
	m = click(m, 5, 5)
	m = click(m, 5, 5) // divider
	if started || m.picker.mode != pickList || m.selectedLabel() != before {
		t.Fatal("clicking a heading or divider did something")
	}
}

func TestPickerScrolledDoubleClickOpensClickedRow(t *testing.T) {
	m, home := pickerModel(t, nil)
	for i := 0; i < 32; i++ {
		os.Mkdir(filepath.Join(home, "Movies", fmt.Sprintf("d%02d", i)), 0o755)
	}
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 15})
	m = selectRow(m, "Browse…")
	m = press(m, "enter")
	m = selectRow(m, "Movies")
	m = press(m, "enter")
	for i := 0; i < 20; i++ {
		m = press(m, "down")
	}
	row := func(y int) string { return strings.TrimSpace(ansi.Strip(strings.Split(view(m), "\n")[y])) }
	target := strings.TrimPrefix(row(2), "> ")
	m = advance(m, time.Second)
	m = click(m, 5, 2)
	if got := strings.TrimPrefix(row(2), "> "); got != target {
		t.Fatalf("list jumped on click: row 2 was %q, now %q", target, got)
	}
	m = click(m, 5, 2)
	if !strings.HasSuffix(m.picker.cwd, target) {
		t.Fatalf("double-click on %q opened %q", target, m.picker.cwd)
	}
}
