package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Abinsanth/stream-video-lan/internal/library"
)

type pickMode int

const (
	pickList pickMode = iota
	pickBrowse
	pickType
)

type rowKind int

const (
	rowHeading rowKind = iota
	rowRecent
	rowShortcut
	rowDivider
	rowBrowse
	rowType
)

type pickRow struct {
	kind  rowKind
	label string
	dir   string
}

func (r pickRow) selectable() bool { return r.kind != rowHeading && r.kind != rowDivider }

// pickerState is the folder picker: a list of recent folders and shortcuts, a folder browser,
// and a type-a-path box.
type pickerState struct {
	mode   pickMode
	rows   []pickRow
	cursor int

	cwd     string   // browse: current folder
	entries []string // browse: ".." then subfolder names
	bcursor int

	input  textinput.Model
	counts map[string]int // folder → video count (-1: can't scan); absent while counting
	err    string
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Faint(true)
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	selStyle   = lipgloss.NewStyle().Reverse(true)
)

func newPicker(d Deps) pickerState {
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "/path/to/folder"
	in.ShowSuggestions = true
	p := pickerState{input: in, counts: map[string]int{}}
	p.rows = pickerRows(d)
	p.cursor = nextSelectable(p.rows, -1, 1)
	return p
}

// pickerRows builds the list: recent folders that still exist, the platform's shortcut folders
// that exist, then Browse and Type. Empty sections are left out.
func pickerRows(d Deps) []pickRow {
	var rows []pickRow
	if recent := d.Config.ExistingRecent(); len(recent) > 0 {
		rows = append(rows, pickRow{kind: rowHeading, label: "Recent"})
		for _, dir := range recent {
			rows = append(rows, pickRow{kind: rowRecent, label: tildePath(dir, d.Home), dir: dir})
		}
	}
	names := []string{"Movies", "Downloads", "Desktop"}
	if d.GOOS == "linux" {
		names = []string{"Videos", "Downloads", "Desktop"}
	}
	var shortcuts []pickRow
	for _, n := range names {
		dir := filepath.Join(d.Home, n)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			shortcuts = append(shortcuts, pickRow{kind: rowShortcut, label: tildePath(dir, d.Home), dir: dir})
		}
	}
	if len(shortcuts) > 0 {
		rows = append(rows, pickRow{kind: rowHeading, label: "Shortcuts"})
		rows = append(rows, shortcuts...)
	}
	return append(rows,
		pickRow{kind: rowDivider},
		pickRow{kind: rowBrowse, label: "Browse…"},
		pickRow{kind: rowType, label: "Type a path…"},
	)
}

func nextSelectable(rows []pickRow, from, step int) int {
	for i := from + step; i >= 0 && i < len(rows); i += step {
		if rows[i].selectable() {
			return i
		}
	}
	return max(from, 0)
}

// countCmds counts the videos in each recent folder in the background. Only recent folders are
// counted up front: scanning Desktop or Downloads unasked would trigger macOS privacy prompts.
func (p pickerState) countCmds() tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range p.rows {
		if r.kind == rowRecent {
			cmds = append(cmds, countCmd(r.dir))
		}
	}
	return tea.Batch(cmds...)
}

func countCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		vs, err := library.Scan(dir)
		if err != nil {
			return countMsg{dir: dir, n: -1}
		}
		return countMsg{dir: dir, n: len(vs)}
	}
}

func (p pickerState) countText(dir string) string {
	n, ok := p.counts[dir]
	switch {
	case !ok:
		return "…"
	case n < 0:
		return ""
	case n == 1:
		return "1 video"
	}
	return strconv.Itoa(n) + " videos"
}

func (m Model) pickerKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.picker.mode {
	case pickBrowse:
		return m.browseKey(k)
	case pickType:
		return m.typeKey(k)
	}
	p := &m.picker
	switch k.String() {
	case "up":
		p.cursor = nextSelectable(p.rows, p.cursor, -1)
	case "down":
		p.cursor = nextSelectable(p.rows, p.cursor, 1)
	case "q":
		return m.quitNow()
	case "enter":
		switch r := p.rows[p.cursor]; r.kind {
		case rowRecent, rowShortcut:
			return m.startShare(r.dir)
		case rowBrowse:
			p.err = ""
			p.mode = pickBrowse
			nm, cmd := m.enterDir(m.deps.Home)
			if nm.picker.cwd == "" { // home unreadable: stay on the list
				nm.picker.mode = pickList
			}
			return nm, cmd
		case rowType:
			p.err = ""
			p.mode = pickType
			p.input.SetValue("")
			return m, p.input.Focus()
		}
	}
	return m, nil
}

func (m Model) browseKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := &m.picker
	switch k.String() {
	case "up":
		p.bcursor = max(p.bcursor-1, 0)
	case "down":
		p.bcursor = min(p.bcursor+1, max(len(p.entries)-1, 0))
	case "esc":
		p.mode, p.err = pickList, ""
	case "backspace", "left":
		return m.enterDir(filepath.Dir(p.cwd))
	case "s":
		return m.startShare(p.cwd)
	case "enter", "right":
		if len(p.entries) == 0 {
			return m, nil
		}
		if name := p.entries[p.bcursor]; name == ".." {
			return m.enterDir(filepath.Dir(p.cwd))
		} else {
			return m.enterDir(filepath.Join(p.cwd, name))
		}
	}
	return m, nil
}

// enterDir moves the browser into dir, or explains why it can't and stays put.
func (m Model) enterDir(dir string) (Model, tea.Cmd) {
	p := &m.picker
	ents, err := os.ReadDir(dir)
	if err != nil {
		p.err = "Can't open " + tildePath(dir, m.deps.Home) + ": " + rootCause(err)
		return m, nil
	}
	var names []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if fi, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && fi.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		switch {
		case library.NaturalLess(a, b):
			return -1
		case library.NaturalLess(b, a):
			return 1
		}
		return 0
	})
	if dir != string(filepath.Separator) {
		names = append([]string{".."}, names...)
	}
	p.cwd, p.entries, p.bcursor, p.err = dir, names, 0, ""
	if len(names) > 1 && names[0] == ".." {
		p.bcursor = 1 // start on the first real folder
	}
	if _, known := p.counts[dir]; known {
		return m, nil
	}
	if !worthCounting(dir, m.deps.Home) {
		p.counts[dir] = -1 // show no count
		return m, nil
	}
	return m, countCmd(dir)
}

// worthCounting says whether to count videos under dir while browsing. Home, the folders above
// it, and ~/Library hold the whole disk's worth of files: scanning them is slow and, on macOS,
// sets off privacy prompts for Desktop, Documents and other apps' data.
func worthCounting(dir, home string) bool {
	sep := string(filepath.Separator)
	switch {
	case dir == sep, home == "":
		return dir != sep
	case dir == home, strings.HasPrefix(home, dir+sep):
		return false
	case dir == filepath.Join(home, "Library"), strings.HasPrefix(dir, filepath.Join(home, "Library")+sep):
		return false
	}
	return true
}

// rootCause trims Go's "open /path: " prefix, since the path is already in the message.
func rootCause(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err.Error()
	}
	return err.Error()
}

func (m Model) typeKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := &m.picker
	switch k.String() {
	case "esc":
		p.mode, p.err = pickList, ""
		p.input.Blur()
		return m, nil
	case "enter":
		dir := cleanPath(p.input.Value(), m.deps.Home)
		if dir == "" {
			return m, nil
		}
		return m.startShare(dir)
	}
	return m.forwardToInput(k)
}

// forwardToInput sends msg (a key, a paste, a cursor blink) to whichever text box is active.
func (m Model) forwardToInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case m.screen == screenPicker && m.picker.mode == pickType:
		m.picker.input, cmd = m.picker.input.Update(msg)
		m.picker.input.SetSuggestions(m.pathSuggestions(m.picker.input.Value()))
	case m.screen == screenServing && m.bar.focused:
		m.bar.input, cmd = m.bar.input.Update(msg)
	}
	return m, cmd
}

// pathSuggestions lists the subfolders of the folder typed so far whose names start with the
// last, partly typed part, spelled the way the user typed the start (so "~/Mov" → "~/Movies").
func (m Model) pathSuggestions(typed string) []string {
	i := strings.LastIndex(typed, "/")
	if i < 0 {
		return nil
	}
	typedDir, prefix := typed[:i+1], strings.ToLower(typed[i+1:])
	ents, err := os.ReadDir(cleanPath(typedDir, m.deps.Home))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), prefix) {
			continue
		}
		if fi, err := os.Stat(filepath.Join(cleanPath(typedDir, m.deps.Home), name)); err == nil && fi.IsDir() {
			out = append(out, typedDir+name)
		}
	}
	return out
}

// selectedLabel is the text of the highlighted row (used by tests to navigate).
func (m Model) selectedLabel() string {
	p := m.picker
	switch {
	case m.screen != screenPicker:
		return ""
	case p.mode == pickBrowse && len(p.entries) > 0:
		return p.entries[p.bcursor]
	case p.mode == pickType:
		return p.input.Value()
	case p.cursor < len(p.rows):
		return p.rows[p.cursor].label
	}
	return ""
}

// listWindow returns the first row to show so that sel stays visible in a view of n rows.
func listWindow(total, sel, n int) int {
	if n <= 0 || total <= n {
		return 0
	}
	return min(max(sel-n+1, 0), total-n)
}

func (m Model) pickerView() string {
	p := m.picker
	var title, footer string
	var lines []string
	sel := 0
	switch p.mode {
	case pickBrowse:
		title = "Browse: " + tildePath(p.cwd, m.deps.Home)
		if c := p.countText(p.cwd); c != "" {
			title = pad(title, m.width-14) + " " + c
		}
		for i, name := range p.entries {
			lines = append(lines, m.pickLine(name, "", i == p.bcursor))
		}
		sel = p.bcursor
		footer = "enter open · backspace up · s share this folder · esc back"
	case pickType:
		title = "Type a path"
		lines = []string{" " + p.input.View()}
		footer = "tab completes · enter shares · esc back"
	default:
		title = "Pick a folder to share"
		for i, r := range p.rows {
			switch r.kind {
			case rowHeading:
				lines = append(lines, "  "+dimStyle.Render(r.label))
			case rowDivider:
				lines = append(lines, "  "+dimStyle.Render(strings.Repeat("─", max(min(m.width-4, 40), 0))))
			case rowRecent:
				lines = append(lines, m.pickLine(r.label, p.countText(r.dir), i == p.cursor))
			case rowBrowse:
				lines = append(lines, m.pickLine(pad(r.label, 15)+dimStyle.Render("move through folders with arrows"), "", i == p.cursor))
			case rowType:
				lines = append(lines, m.pickLine(pad(r.label, 15)+dimStyle.Render("tab completes, paste works"), "", i == p.cursor))
			default:
				lines = append(lines, m.pickLine(r.label, "", i == p.cursor))
			}
		}
		sel = p.cursor
		footer = "↑↓ move · enter select · q quit"
	}
	avail := m.height - 5
	start := listWindow(len(lines), sel, avail)
	if avail > 0 && len(lines) > avail {
		lines = lines[start : start+avail]
	}
	out := []string{" " + titleStyle.Render(title), ""}
	out = append(out, lines...)
	out = append(out, "")
	if p.err != "" {
		out = append(out, " "+errStyle.Render(p.err))
	}
	out = append(out, " "+dimStyle.Render(footer))
	return strings.Join(out, "\n")
}

// pickLine renders one selectable row: a marker, the label, and an optional right-hand note.
func (m Model) pickLine(label, note string, selected bool) string {
	marker := "  "
	if selected {
		marker = "> "
	}
	w := m.width - 2
	text := label
	if note != "" {
		text = pad(label, max(w-13, 0)) + " " + fit(note, 12)
	}
	if selected {
		return " " + marker + selStyle.Render(fit(text, w-1))
	}
	return " " + marker + fit(text, w-1)
}
