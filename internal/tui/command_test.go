package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHelpModalShowsLinksAndTips(t *testing.T) {
	m, _ := servingModel(t)
	m.share.(*fakeShare).urls = []string{testBase + "/playlist.m3u8", "http://10.0.0.2:8080/playlist.m3u8"}
	m = typeText(m, "/h")
	m = press(m, "enter")
	v := view(m)
	for _, s := range []string{"http://10.0.0.2:8080/playlist.m3u8", "Open Network Stream", "click Allow", "same Wi-Fi", "lanstream dev", "/stop", "/q"} {
		if !strings.Contains(v, s) {
			t.Errorf("help missing %q in\n%s", s, v)
		}
	}
	m = press(m, "esc")
	if strings.Contains(view(m), "Open Network Stream") {
		t.Fatal("help did not close")
	}
}

func TestCommandBarShowsTypingAndEscClears(t *testing.T) {
	m, _ := servingModel(t)
	m = typeText(m, "/he")
	if !strings.Contains(view(m), "/he") {
		t.Fatal(view(m))
	}
	m = press(m, "esc")
	if m.bar.focused || strings.Contains(view(m), "/he") {
		t.Fatal("esc did not clear the command bar")
	}
}

func TestKeysGoToBarWhileTyping(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 1))
	m = typeText(m, "/x")
	m = press(m, "tab") // would switch tabs if not typing
	if m.serving.tab != tabVideos {
		t.Fatal("tab switched while typing a command")
	}
}

func TestUnknownCommand(t *testing.T) {
	m, _ := servingModel(t)
	m = typeText(m, "/zzz")
	m = press(m, "enter")
	if !strings.Contains(view(m), "Unknown command /zzz") || m.bar.focused {
		t.Fatal(view(m))
	}
}

func TestQuitWithoutViewersQuitsAndStops(t *testing.T) {
	m, _ := servingModel(t)
	fs := m.share.(*fakeShare)
	m = typeText(m, "/q")
	m, cmd := updateCmd(m, "enter")
	if !quits(m, cmd) || fs.stops != 1 {
		t.Fatal("did not quit cleanly", fs.stops)
	}
}

func TestQuitWithViewersConfirms(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 10))
	fs := m.share.(*fakeShare)
	m.deps.Tracker.Begin("ip", "a.mp4", 10, 0)
	m = press(m, "ctrl+c")
	if !strings.Contains(view(m), "1 person is watching. Stop anyway? (y/n)") {
		t.Fatal(view(m))
	}
	m, cmd := updateCmd(m, "n")
	if quits(m, cmd) || fs.stops != 0 || m.modal != modalNone {
		t.Fatal("quit after n")
	}
	m = press(m, "ctrl+c")
	m, cmd = updateCmd(m, "y")
	if !quits(m, cmd) || fs.stops != 1 {
		t.Fatal("no quit after y")
	}
}

func TestConfirmCountsPeople(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 10))
	m.deps.Tracker.Begin("10.0.0.1", "a.mp4", 10, 0)
	m.deps.Tracker.Begin("10.0.0.2", "a.mp4", 10, 0)
	m = typeText(m, "/q")
	m = press(m, "enter")
	if !strings.Contains(view(m), "2 people are watching") {
		t.Fatal(view(m))
	}
}

func TestSwitchFolderStopsShareAndShowsPicker(t *testing.T) {
	m, _ := servingModel(t)
	fs := m.share.(*fakeShare)
	m = typeText(m, "/f")
	m, cmd := updateCmd(m, "enter")
	m = update(m, cmd())
	if m.screen != screenPicker || m.share != nil || fs.stops != 1 {
		t.Fatal(m.screen, fs.stops)
	}
	if !strings.Contains(view(m), "Pick a folder to share") {
		t.Fatal(view(m))
	}
}

func TestCtrlCOnPickerQuits(t *testing.T) {
	m, _ := pickerModel(t, nil)
	if m, cmd := updateCmd(m, "ctrl+c"); !quits(m, cmd) {
		t.Fatal("ctrl+c did not quit picker")
	}
}

func TestSecondCtrlCInConfirmQuits(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 10))
	m.deps.Tracker.Begin("ip", "a.mp4", 10, 0)
	m = press(m, "ctrl+c")
	if m, cmd := updateCmd(m, "ctrl+c"); !quits(m, cmd) {
		t.Fatal("second ctrl+c did not quit")
	}
}

func TestTinyTerminalsNeverPanic(t *testing.T) {
	for _, sz := range [][2]int{{1, 1}, {20, 5}, {40, 10}} {
		for _, build := range []func() Model{
			func() Model { m, _ := pickerModel(t, []string{"Movies/Anime"}); return m },
			func() Model {
				m, _ := servingModel(t, vid(strings.Repeat("x", 300)+".mkv", 1), vid("b.mp4", 1))
				return m
			},
		} {
			m := update(build(), tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			m = press(m, "end", "down")
			_ = view(m)
			m = click(m, 0, 3)
			m = press(m, "tab")
			_ = view(m)
			m = typeText(m, "/h")
			m = press(m, "enter")
			for _, line := range strings.Split(view(m), "\n") {
				if w := len([]rune(line)); w > 0 && sz[0] < 3 && w > 400 {
					t.Fatal("unbounded line")
				}
			}
		}
	}
}

func TestQuitStopsInBackgroundThenQuits(t *testing.T) {
	m, _ := servingModel(t)
	fs := m.share.(*fakeShare)
	m = typeText(m, "/q")
	m, cmd := updateCmd(m, "enter")
	if !strings.Contains(view(m), "Stopping…") {
		t.Fatal(view(m))
	}
	if fs.stops != 0 {
		t.Fatal("stopped inside Update instead of in the background")
	}
	if !quits(m, cmd) || fs.stops != 1 {
		t.Fatal("did not stop then quit")
	}
}

func TestSwitchFolderClearsViewers(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 10))
	m.deps.Tracker.Begin("ip", "a.mp4", 10, 0)
	m = typeText(m, "/f")
	m = press(m, "enter") // confirm appears
	m, cmd := updateCmd(m, "y")
	m = update(m, cmd()) // stoppedMsg → picker
	if m.screen != screenPicker || m.deps.Tracker.Active() {
		t.Fatal("viewers survived the switch")
	}
}

func TestClicksWhileStoppingDoNotCrash(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 1))
	m = typeText(m, "/q")
	m = press(m, "enter") // now "Stopping…", share already handed off
	for _, y := range []int{rowHeader, rowTabs, rowListStart} {
		m = click(m, 5, y)
		m = click(m, 5, y)
	}
	m = runCmds(m)
	_ = view(m)
}

// Bubble Tea draws one last frame after the model asks to quit; every way out must survive it.
func TestLastFrameAfterQuittingDoesNotCrash(t *testing.T) {
	drawAfter := func(name string, m Model, cmd tea.Cmd) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s: drawing the last frame panicked: %v", name, r)
			}
		}()
		for i := 0; i < 5 && cmd != nil; i++ { // follow stop → stoppedMsg → quit
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				break
			}
			var nm tea.Model
			nm, cmd = m.Update(msg)
			m = nm.(Model)
			_ = view(m)
		}
		_ = view(m)
	}

	m, _ := servingModel(t, vid("a.mp4", 1))
	m = typeText(m, "/q")
	m, cmd := updateCmd(m, "enter")
	drawAfter("/q", m, cmd)

	m, _ = servingModel(t, vid("a.mp4", 1))
	m, cmd = updateCmd(m, "ctrl+c")
	drawAfter("ctrl+c", m, cmd)

	m, _ = servingModel(t, vid("a.mp4", 1))
	m.deps.Tracker.Begin("ip", "a.mp4", 1, 0)
	m = press(m, "ctrl+c")
	m, cmd = updateCmd(m, "y")
	drawAfter("ctrl+c then y", m, cmd)

	m, _ = servingModel(t, vid("a.mp4", 1))
	m = typeText(m, "/f")
	m, cmd = updateCmd(m, "enter")
	drawAfter("/f", m, cmd)

	p, _ := pickerModel(t, nil)
	p, cmd = updateCmd(p, "q")
	drawAfter("picker q", p, cmd)
}

func TestPickerSlashHOpensHelp(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = selectRow(m, "Browse…")
	m = press(m, "enter", "esc") // the owner's path: browse, then back
	m = typeText(m, "/h")
	m = press(m, "enter")
	if !strings.Contains(view(m), "Commands") {
		t.Fatal(view(m))
	}
	if strings.Contains(view(m), "Playlist links") {
		t.Fatal("playlist links shown while nothing is shared")
	}
}

func TestPickerSlashQQuits(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = typeText(m, "/q")
	if m, cmd := updateCmd(m, "enter"); !quits(m, cmd) {
		t.Fatal("/q did not quit the picker")
	}
}

func TestPickerPlainQDoesNotQuit(t *testing.T) {
	m, _ := pickerModel(t, nil)
	if m, cmd := updateCmd(m, "q"); quits(m, cmd) {
		t.Fatal("a bare q quit the picker")
	}
}

func TestBrowseSlashOpensBar(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = selectRow(m, "Browse…")
	m = press(m, "enter")
	m = typeText(m, "/h")
	m = press(m, "enter")
	if !strings.Contains(view(m), "Commands") {
		t.Fatal(view(m))
	}
}

func TestSlashInTypePathIsText(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = selectRow(m, "Type a path…")
	m = press(m, "enter")
	m = typeText(m, "/tmp/x")
	if m.bar.focused || m.picker.input.Value() != "/tmp/x" {
		t.Fatalf("bar=%v value=%q", m.bar.focused, m.picker.input.Value())
	}
}

func TestStopOnPickerSaysNothingShared(t *testing.T) {
	m, _ := pickerModel(t, nil)
	for _, c := range []string{"/stop", "/qr"} {
		m = typeText(m, c)
		m = press(m, "enter")
		if !strings.Contains(view(m), "Nothing is being shared yet.") {
			t.Fatalf("%s: %s", c, view(m))
		}
	}
}

func TestStopReturnsToPicker(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 1))
	fs := m.share.(*fakeShare)
	m = typeText(m, "/stop")
	m, cmd := updateCmd(m, "enter")
	m = update(m, cmd())
	if m.screen != screenPicker || m.share != nil || fs.stops != 1 {
		t.Fatal(m.screen, fs.stops)
	}
}

func TestStopConfirms(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 10))
	fs := m.share.(*fakeShare)
	m.deps.Tracker.Begin("ip", "a.mp4", 10, 0)
	m = typeText(m, "/stop")
	m = press(m, "enter")
	if !strings.Contains(view(m), "1 person is watching. Stop anyway? (y/n)") {
		t.Fatal(view(m))
	}
	m = press(m, "n")
	if m.screen != screenServing || m.share == nil || fs.stops != 0 {
		t.Fatal("n did not keep sharing")
	}
}

func TestHelpListsStopAndFitsWidth(t *testing.T) {
	m, _ := servingModel(t)
	m = typeText(m, "/h")
	m = press(m, "enter")
	v := view(m)
	if !strings.Contains(v, "/stop stop sharing") || !strings.Contains(v, "/qr") {
		t.Fatal(v)
	}
	for _, line := range strings.Split(v, "\n") {
		if strings.HasSuffix(line, "…") {
			t.Fatalf("help line cut off at 80 columns: %q", line)
		}
	}
}

func TestHelpFromPickerShowsPickerKeys(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = typeText(m, "/h")
	m = press(m, "enter")
	v := view(m)
	if !strings.Contains(v, "enter select") || strings.Contains(v, "switch tabs") || strings.Contains(v, "a video") {
		t.Fatal(v)
	}
	if strings.Contains(v, "\n\n\n") {
		t.Fatal("double blank line in help")
	}
}
