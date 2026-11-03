package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Abinsanth/stream-video-lan/internal/library"
	"github.com/Abinsanth/stream-video-lan/internal/viewers"
)

// Fixed screen rows on the serving screen (mouse clicks are mapped through these).
const (
	rowHeader    = 0
	rowTabs      = 1
	rowListStart = 3
	tabSplitX    = 10 // clicks left of this on the tab row pick Videos
)

const (
	tabVideos = iota
	tabViewers
)

// servingState is the serving screen: the video list, who's watching, and click tracking.
type servingState struct {
	tab      int
	videos   []library.Video
	watchers []viewers.Viewer
	sel, off int

	lastClickY  int
	lastClickAt time.Time
}

// refreshServing reloads the lists, keeping the selected video selected by name when it still
// exists, and otherwise clamping the selection into range.
func (m *Model) refreshServing() {
	s := &m.serving
	prev := ""
	if s.sel < len(s.videos) {
		prev = s.videos[s.sel].RelPath
	}
	s.videos = m.share.Videos()
	s.sel = min(s.sel, max(len(s.videos)-1, 0))
	for i, v := range s.videos {
		if v.RelPath == prev {
			s.sel = i
			break
		}
	}
	if m.deps.Tracker != nil {
		s.watchers = m.deps.Tracker.Snapshot()
	}
	m.fixScroll()
}

func (m Model) listRows() int { return max(m.height-5, 0) }

// fixScroll keeps the selected row inside the visible part of the list.
func (m *Model) fixScroll() {
	s, n := &m.serving, m.listRows()
	if n == 0 {
		s.off = 0
		return
	}
	if s.sel < s.off {
		s.off = s.sel
	}
	if s.sel >= s.off+n {
		s.off = s.sel - n + 1
	}
	s.off = max(min(s.off, max(len(s.videos)-n, 0)), 0)
}

func (m Model) servingKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.bar.focused {
		return m.barKey(k)
	}
	if k.Text == "/" {
		return m.focusBar()
	}
	s := &m.serving
	switch k.String() {
	case "tab", "shift+tab", "left", "right":
		s.tab = 1 - s.tab
		return m, nil
	}
	if s.tab != tabVideos || len(s.videos) == 0 {
		return m, nil
	}
	page := max(m.listRows()-1, 1)
	switch k.String() {
	case "up":
		s.sel--
	case "down":
		s.sel++
	case "pgup":
		s.sel -= page
	case "pgdown":
		s.sel += page
	case "home":
		s.sel = 0
	case "end":
		s.sel = len(s.videos) - 1
	case "enter":
		return m, m.copyVideo(s.sel)
	}
	s.sel = max(min(s.sel, len(s.videos)-1), 0)
	m.fixScroll()
	return m, nil
}

func (m Model) copyVideo(i int) tea.Cmd {
	if i < 0 || i >= len(m.serving.videos) {
		return nil
	}
	return copyCmd(m.share.VideoURL(m.serving.videos[i].RelPath), m.deps.CopyText)
}

// servingClick handles a left click: tab row switches tabs, a list row selects, and a second
// click on the same row within half a second copies (the header copies the playlist link).
func (m Model) servingClick(c tea.Mouse) (tea.Model, tea.Cmd) {
	if c.Button != tea.MouseLeft {
		return m, nil
	}
	s := &m.serving
	now := m.deps.Now()
	double := c.Y == s.lastClickY && !s.lastClickAt.IsZero() && now.Sub(s.lastClickAt) <= doubleClick
	if double {
		s.lastClickAt = time.Time{} // a third click starts over
	} else {
		s.lastClickY, s.lastClickAt = c.Y, now
	}
	switch {
	case c.Y == rowHeader:
		if double {
			return m, copyCmd(m.share.PlaylistURL(), m.deps.CopyText)
		}
	case c.Y == rowTabs:
		s.tab = tabVideos
		if c.X >= tabSplitX {
			s.tab = tabViewers
		}
	case c.Y >= rowListStart && c.Y < rowListStart+m.listRows() && s.tab == tabVideos:
		i := s.off + c.Y - rowListStart
		if i >= len(s.videos) {
			return m, nil
		}
		s.sel = i
		if double {
			return m, m.copyVideo(i)
		}
	}
	return m, nil
}

func (m Model) servingView() string {
	w, n := m.width, m.listRows()
	sep := dimStyle.Render(strings.Repeat("─", max(w, 0)))
	lines := []string{m.headerLine(), m.tabsLine(), sep}
	var list []string
	if m.serving.tab == tabVideos {
		list = m.videoRows(n)
	} else {
		list = m.viewerRows(n)
	}
	for len(list) < n {
		list = append(list, "")
	}
	lines = append(lines, list[:n]...)
	lines = append(lines, sep, m.barLine())
	return strings.Join(lines, "\n")
}

// headerLine shows the folder and the playlist link; when space is short the folder gives way,
// because the link is what people need.
func (m Model) headerLine() string {
	url := m.share.PlaylistURL()
	root := tildePath(m.share.Root(), m.deps.Home)
	avail := m.width - 1 - lipgloss.Width(url) - 3
	if avail < 4 {
		return " " + titleStyle.Render(url)
	}
	return " " + fit(root, avail) + " · " + titleStyle.Render(url)
}

func (m Model) tabsLine() string {
	videos, watchers := "  Videos  ", "  Viewers ("+strconv.Itoa(len(m.serving.watchers))+")  "
	if m.serving.tab == tabVideos {
		videos = titleStyle.Render("[ Videos ]")
	} else {
		watchers = titleStyle.Render("[ Viewers (" + strconv.Itoa(len(m.serving.watchers)) + ") ]")
	}
	return videos + "  " + watchers
}

func (m Model) videoRows(n int) []string {
	s := m.serving
	if err := m.share.ScanErr(); err != nil {
		return []string{"  " + errStyle.Render("Folder can't be read: "+err.Error())}
	}
	if len(s.videos) == 0 {
		return []string{"  " + dimStyle.Render("No videos here yet. New files show up automatically.")}
	}
	nameW := max(m.width-14, 0)
	var rows []string
	for i := s.off; i < len(s.videos) && i < s.off+n; i++ {
		v := s.videos[i]
		text := pad(v.RelPath, nameW) + " " + fmt.Sprintf("%10s", humanSize(v.Size))
		if i == s.sel {
			rows = append(rows, "> "+selStyle.Render(text))
		} else {
			rows = append(rows, "  "+text)
		}
	}
	return rows
}

func (m Model) viewerRows(n int) []string {
	var rows []string
	if len(m.serving.watchers) == 0 {
		rows = append(rows, "  "+dimStyle.Render("Nobody is watching right now."))
	}
	nameW := max(m.width-26, 0)
	for _, v := range m.serving.watchers {
		rows = append(rows, "  "+pad(v.IP, 15)+" "+pad(v.RelPath, nameW)+" ~"+strconv.Itoa(v.Percent)+"%")
	}
	if m.deps.Keeper.Held() && n > 0 {
		awake := "  " + dimStyle.Render("Keeping this computer awake while someone watches")
		if len(rows) >= n {
			rows = rows[:n-1]
		}
		for len(rows) < n-1 {
			rows = append(rows, "")
		}
		rows = append(rows, awake)
	}
	return rows
}

// barLine is the bottom row: the command being typed, or the latest message and a help hint.
func (m Model) barLine() string {
	if m.bar.focused {
		return " " + m.bar.input.View()
	}
	hint := "/h help"
	return " " + pad(m.flash, max(m.width-2-len(hint), 0)) + dimStyle.Render(hint)
}
