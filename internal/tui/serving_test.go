package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Abinsanth/stream-video-lan/internal/config"
	"github.com/Abinsanth/stream-video-lan/internal/library"
)

func TestServingHeaderAndRows(t *testing.T) {
	m, _ := servingModel(t, vid("Attack on Titan/S01E01.mkv", 1_200_000_000), vid("Your Name.mp4", 3_400_000_000))
	v := view(m)
	for _, s := range []string{"/videos", testBase + "/playlist.m3u8", "[ Videos ]", "Viewers (0)", "Attack on Titan/S01E01.mkv", "1.2 GB", "3.4 GB", "/h help"} {
		if !strings.Contains(v, s) {
			t.Errorf("missing %q in\n%s", s, v)
		}
	}
	if n := len(strings.Split(v, "\n")); n != 20 {
		t.Errorf("view has %d lines, want 20", n)
	}
}

func TestHeaderKeepsURLWhenRootIsLong(t *testing.T) {
	m, _ := servingModel(t)
	m.share.(*fakeShare).root = "/" + strings.Repeat("very-long-folder/", 10)
	if !strings.Contains(view(m), testBase+"/playlist.m3u8") {
		t.Fatal(view(m))
	}
}

func TestEnterCopiesVideoURL(t *testing.T) {
	m, copied := servingModel(t, vid("a.mp4", 1), vid("b.mp4", 1))
	m = press(m, "down", "enter")
	m = runCmds(m)
	if *copied != testBase+"/v/b.mp4" || !strings.Contains(view(m), "Copied") {
		t.Fatal(*copied, view(m))
	}
}

func TestDoubleClickCopiesSingleClickSelects(t *testing.T) {
	m, copied := servingModel(t, vid("a.mp4", 1), vid("b.mp4", 1))
	m = click(m, 5, 4) // row 4 = second video
	m = runCmds(m)
	if *copied != "" || m.serving.sel != 1 {
		t.Fatal("single click copied or did not select", *copied, m.serving.sel)
	}
	m = advance(m, 300*time.Millisecond)
	m = click(m, 5, 4)
	m = runCmds(m)
	if *copied != testBase+"/v/b.mp4" {
		t.Fatal(*copied)
	}
}

func TestSlowSecondClickIsNotDoubleClick(t *testing.T) {
	m, copied := servingModel(t, vid("a.mp4", 1))
	m = click(m, 5, 3)
	m = advance(m, 600*time.Millisecond)
	m = click(m, 5, 3)
	m = runCmds(m)
	if *copied != "" {
		t.Fatal("copied on slow clicks")
	}
}

func TestDoubleClickHeaderCopiesPlaylist(t *testing.T) {
	m, copied := servingModel(t, vid("a.mp4", 1))
	m = click(m, 40, 0)
	m = click(m, 40, 0)
	m = runCmds(m)
	if *copied != testBase+"/playlist.m3u8" {
		t.Fatal(*copied)
	}
}

func TestClickBelowListDoesNothing(t *testing.T) {
	m, copied := servingModel(t, vid("a.mp4", 1))
	m = click(m, 5, 10)
	m = click(m, 5, 10)
	m = runCmds(m)
	if *copied != "" || m.serving.sel != 0 {
		t.Fatal(*copied, m.serving.sel)
	}
}

func TestTabSwitchByKeyAndClick(t *testing.T) {
	m, _ := servingModel(t)
	m = press(m, "tab")
	if !strings.Contains(view(m), "[ Viewers (0) ]") {
		t.Fatal(view(m))
	}
	m = click(m, 2, 1)
	if !strings.Contains(view(m), "[ Videos ]") {
		t.Fatal(view(m))
	}
	m = click(m, 15, 1)
	if !strings.Contains(view(m), "[ Viewers (0) ]") {
		t.Fatal(view(m))
	}
}

func TestViewersTabShowsWatchersAndAwake(t *testing.T) {
	m, _ := servingModel(t, vid("Your Name.mp4", 1000))
	m.deps.Tracker.Begin("192.168.1.12", "Your Name.mp4", 1000, 450)
	m = update(m, tickMsg{})
	m = press(m, "tab")
	v := view(m)
	for _, s := range []string{"Viewers (1)", "192.168.1.12", "Your Name.mp4", "~45%", "Keeping this computer awake"} {
		if !strings.Contains(v, s) {
			t.Errorf("missing %q in\n%s", s, v)
		}
	}
}

func TestViewersTabEmpty(t *testing.T) {
	m, _ := servingModel(t)
	m = press(m, "tab")
	if !strings.Contains(view(m), "Nobody is watching right now.") {
		t.Fatal(view(m))
	}
}

func TestSelectionSurvivesRefreshByName(t *testing.T) {
	m, _ := servingModel(t, vid("b.mp4", 1), vid("c.mp4", 1))
	m = press(m, "down") // c.mp4
	m.share.(*fakeShare).videos = []library.Video{vid("a.mp4", 1), vid("b.mp4", 1), vid("c.mp4", 1)}
	m = update(m, tickMsg{})
	if m.serving.videos[m.serving.sel].RelPath != "c.mp4" {
		t.Fatal(m.serving.sel)
	}
}

func TestSelectionClampedWhenListShrinks(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 1), vid("b.mp4", 1), vid("c.mp4", 1))
	m = press(m, "end")
	m.share.(*fakeShare).videos = []library.Video{vid("a.mp4", 1)}
	m = update(m, tickMsg{})
	if m.serving.sel != 0 {
		t.Fatal(m.serving.sel)
	}
	_ = view(m)
	m.share.(*fakeShare).videos = nil
	m = update(m, tickMsg{})
	m = press(m, "enter", "down", "end")
	_ = view(m)
}

func TestScrollKeepsSelectionVisible(t *testing.T) {
	var vids []library.Video
	for i := 0; i < 40; i++ {
		vids = append(vids, vid("v"+strings.Repeat("0", 2-len(itoa2(i)))+itoa2(i)+".mp4", 1))
	}
	m, _ := servingModel(t, vids...)
	m = press(m, "end")
	if !strings.Contains(view(m), "v39.mp4") {
		t.Fatal(view(m))
	}
	m = press(m, "home")
	if !strings.Contains(view(m), "v00.mp4") {
		t.Fatal(view(m))
	}
}

func itoa2(i int) string { return string(rune('0'+i/10)) + string(rune('0'+i%10)) }

func TestCopyFailureShowsWholeURLUntilDismissed(t *testing.T) {
	long := "Some Show Name/Season 01/Some Show Name - S01E01 - A Rather Long Episode Title (1080p).mkv"
	m, _ := servingModel(t, vid(long, 1))
	m.deps.CopyText = func(string) error { return errors.New("no clipboard utility") }
	m = press(m, "enter")
	m = runCmds(m)
	url := testBase + "/v/" + long
	flat := strings.ReplaceAll(strings.ReplaceAll(view(m), "\n", ""), " ", "")
	if !strings.Contains(flat, strings.ReplaceAll(url, " ", "")) {
		t.Fatalf("full link not shown:\n%s", view(m))
	}
	m = advance(m, time.Minute)
	m = update(m, tickMsg{})
	if !strings.Contains(view(m), "Copy this link") {
		t.Fatal("link box disappeared on its own")
	}
	m = press(m, "esc")
	if strings.Contains(view(m), "Copy this link") {
		t.Fatal("esc did not close the link box")
	}
}

func TestLongUnicodeNamesTruncate(t *testing.T) {
	long := strings.Repeat("മലയാളം 🎬 ", 30) + ".mkv"
	m, _ := servingModel(t, vid(long, 5_000_000_000))
	for _, line := range strings.Split(view(m), "\n") {
		if lipgloss.Width(line) > 80 || !utf8.ValidString(line) {
			t.Fatalf("bad line %q", line)
		}
	}
	if !strings.Contains(view(m), "5.0 GB") {
		t.Fatal("size pushed off screen")
	}
}

func TestEmptyAndErrorStates(t *testing.T) {
	m, _ := servingModel(t)
	if !strings.Contains(view(m), "No videos here yet") {
		t.Fatal(view(m))
	}
	m.share.(*fakeShare).scanErr = errors.New("no such file or directory")
	m = update(m, tickMsg{})
	if !strings.Contains(view(m), "Folder can't be read: no such file or directory") {
		t.Fatal(view(m))
	}
}

func TestFirstRunTipShownOnce(t *testing.T) {
	m, _ := servingModelWithConfig(t, config.Config{})
	if !strings.Contains(view(m), "click Allow") || !m.deps.Config.TipShown {
		t.Fatal("tip not shown or not recorded", view(m))
	}
	m2, _ := servingModelWithConfig(t, config.Config{TipShown: true})
	if strings.Contains(view(m2), "click Allow") {
		t.Fatal("tip shown again")
	}
}

func TestStartShareStopsPreviousShare(t *testing.T) {
	m, _ := servingModel(t)
	old := m.share.(*fakeShare)
	m = update(m, startReq{dir: "/other"})
	if old.stops != 1 {
		t.Fatal("previous share not stopped")
	}
}

func TestResizeKeepsSelectionVisible(t *testing.T) {
	var vids []library.Video
	for i := 0; i < 50; i++ {
		vids = append(vids, vid(fmt.Sprintf("ep%02d.mkv", i), 1))
	}
	m, _ := servingModel(t, vids...)
	m = press(m, "end")
	m = update(m, tea.WindowSizeMsg{Width: 40, Height: 7})
	if !strings.Contains(view(m), "ep49.mkv") {
		t.Fatal(view(m))
	}
}

func TestViewersOverflowShowsMore(t *testing.T) {
	m, _ := servingModel(t)
	for i := 0; i < 10; i++ {
		m.deps.Tracker.Begin(fmt.Sprintf("10.0.0.%d", i), "a.mp4", 10, 0)
	}
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 8})
	m = update(m, tickMsg{})
	m = press(m, "tab")
	v := view(m)
	if !strings.Contains(v, "+") || !strings.Contains(v, "more") {
		t.Fatal(v)
	}
}

func TestControlCharsInNamesDontBreakLayout(t *testing.T) {
	m, copied := servingModel(t, vid("bad\nname\t.mkv", 1), vid("b.mp4", 1))
	v := view(m)
	if n := len(strings.Split(v, "\n")); n != 20 || !strings.Contains(v, "bad?name?.mkv") || !strings.Contains(v, "/h help") {
		t.Fatalf("%d lines:\n%s", n, v)
	}
	m = click(m, 5, 4)
	m = click(m, 5, 4)
	m = runCmds(m)
	if *copied != testBase+"/v/b.mp4" {
		t.Fatal(*copied)
	}
}

func TestDisplayName(t *testing.T) {
	if got := displayName("a\nb\tc\x1b[31m\x7fd"); got != "a?b?c?[31m?d" {
		t.Fatalf("%q", got)
	}
}
