package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rsc.io/qr"
)

func TestQRLinesSize(t *testing.T) {
	url := testBase + "/playlist.m3u8"
	code, err := qr.Encode(url, qr.L)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := qrLines(url)
	if err != nil {
		t.Fatal(err)
	}
	side := code.Size + 4 // 2-module quiet zone each side
	if len(lines) != (side+1)/2 {
		t.Fatalf("%d lines, want %d", len(lines), (side+1)/2)
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != side {
			t.Fatalf("line %d is %d wide, want %d", i, w, side)
		}
	}
}

func TestQRUsesExplicitColours(t *testing.T) {
	lines, _ := qrLines(testBase + "/playlist.m3u8")
	sawBlack := false
	for i, l := range lines {
		cells := strings.Count(l, "▀")
		// Every cell sets both colours itself, so the terminal's own colours never show through.
		if strings.Count(l, "38;2;") != cells || strings.Count(l, "48;2;") != cells {
			t.Fatalf("line %d has cells without explicit colours: %q", i, l)
		}
		sawBlack = sawBlack || strings.Contains(l, "0;0;0m") || strings.Contains(l, "38;2;0;0;0")
	}
	if !sawBlack {
		t.Fatal("no black modules at all")
	}
}

func TestQRModalPointsToLauncherPage(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 1))
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = typeText(m, "/qr")
	m = press(m, "enter")
	v := view(m)
	if !strings.Contains(v, "Link: "+testBase+"/\n") || !strings.Contains(v, "Open in VLC") || !strings.Contains(v, "▀") {
		t.Fatal(v)
	}
	if strings.Contains(v, "vlc://") || strings.Contains(v, "v: ") {
		t.Fatal("the old vlc:// toggle is still there")
	}
	m = press(m, "esc")
	if strings.Contains(view(m), "Link:") {
		t.Fatal("esc did not close")
	}
}

func TestQRTooSmall(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 1))
	m = update(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	m = typeText(m, "/qr")
	m = press(m, "enter")
	if v := view(m); !strings.Contains(v, "Make the window bigger") || strings.Contains(v, "▀") {
		t.Fatal(v)
	}
}
