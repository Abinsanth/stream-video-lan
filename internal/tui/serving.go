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
	s := &m.serving
	switch k.String() {
	case "esc":
		m.notice = ""
		return m, nil
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
	root := displayName(tildePath(m.share.Root(), m.deps.Home))
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
	tabs := videos + "  " + watchers
	// The sharing status sits at the right end; it is the first thing to go when space is short.
	room := m.width - lipgloss.Width(tabs) - 2
	if room < 10 {
		return tabs
	}
	status := fit(m.statusText(), room)
	return tabs + strings.Repeat(" ", room-lipgloss.Width(status)+1) + statusStyle.Render(status)
}

// statusText is the one place that says the share is live, who is on it, and whether the
// computer is being kept awake.
func (m Model) statusText() string {
	s := "● sharing · "
	switch n := len(m.serving.watchers); n {
	case 0:
		s += "nobody watching"
	default:
		s += strconv.Itoa(n) + " watching"
	}
	if m.deps.Keeper.Held() {
		s += " · keeping awake"
	}
	return s
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
		text := pad(displayName(v.RelPath), nameW) + " " + fmt.Sprintf("%10s", humanSize(v.Size))
		if i == s.sel {
			rows = append(rows, "> "+selStyle.Render(text))
		} else {
			rows = append(rows, "  "+text)
		}
	}
	return rows
}

func (m Model) viewerRows(n int) []string {
	watchers := m.serving.watchers
	var rows []string
	if len(watchers) == 0 {
		rows = append(rows, "  "+dimStyle.Render("Nobody is watching right now."))
	}
	shown := watchers
	if len(watchers) > n {
		shown = watchers[:max(n-1, 0)]
	}
	nameW := max(m.width-26, 0)
	for _, v := range shown {
		rows = append(rows, "  "+pad(v.IP, 15)+" "+pad(displayName(v.RelPath), nameW)+" ~"+strconv.Itoa(v.Percent)+"%")
	}
	if more := len(watchers) - len(shown); more > 0 && n > 0 {
		rows = append(rows, "  "+dimStyle.Render("+"+strconv.Itoa(more)+" more"))
	}
	return rows
}

// barLine is the bottom row of every screen: the command being typed; otherwise the latest
// message (flash, then update notice) with a help hint; otherwise what the keys do here.
func (m Model) barLine() string {
	if m.bar.focused {
		return " " + m.bar.input.View()
	}
	msg := m.flash
	if msg == "" && m.notice != "" {
		msg = noticeStyle.Render(m.notice)
	}
	if msg == "" {
		return " " + dimStyle.Render(fit(m.hints(), max(m.width-1, 0)))
	}
	hint := "/h help"
	return " " + pad(msg, max(m.width-2-len(hint), 0)) + dimStyle.Render(hint)
}

// hints says what the keys do on the current screen.
func (m Model) hints() string {
	switch {
	case m.screen == screenServing && m.serving.tab == tabViewers:
		return "tab videos · /stop · /qr · /h help"
	case m.screen == screenServing:
		return "↑↓ select · enter copy link · tab viewers · /stop · /qr · /h help"
	case m.picker.mode == pickBrowse:
		return "enter open · ⌫ up · s share this folder · esc back · /h help"
	case m.picker.mode == pickType:
		return "tab completes · enter shares · esc back (then /h help)"
	}
	return "↑↓ move · enter select · /h help · /q quit"
}
