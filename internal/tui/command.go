package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// cmdState is the command bar at the bottom of the serving screen.
type cmdState struct {
	input   textinput.Model
	focused bool
}

func newCmdBar() cmdState {
	in := textinput.New()
	in.Prompt = ""
	return cmdState{input: in}
}

// focusBar starts typing a command; the "/" that triggered it is kept.
func (m Model) focusBar() (tea.Model, tea.Cmd) {
	m.bar.focused = true
	m.bar.input.SetValue("/")
	m.bar.input.CursorEnd()
	return m, m.bar.input.Focus()
}

func (m *Model) blurBar() {
	m.bar.focused = false
	m.bar.input.SetValue("")
	m.bar.input.Blur()
}

func (m Model) barKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.blurBar()
		return m, nil
	case "enter":
		line := strings.TrimSpace(m.bar.input.Value())
		m.blurBar()
		return m.runCommand(line)
	}
	return m.forwardToInput(k)
}

func (m Model) runCommand(line string) (tea.Model, tea.Cmd) {
	name, _, _ := strings.Cut(line, " ")
	switch strings.ToLower(name) {
	case "", "/":
		return m, nil
	case "/h", "/help":
		m.modal = modalHelp
		return m, nil
	case "/f", "/folder":
		return m.confirmOr(switchFolder)
	case "/q", "/quit":
		return m.confirmOr(m.quitAction())
	}
	m.setFlash("Unknown command "+name+". /h for help.", copyFlashFor)
	return m, nil
}

// switchFolder stops sharing and goes back to the picker (with an up-to-date recent list).
func switchFolder(m Model) (Model, tea.Cmd) {
	if m.share != nil {
		m.share.Stop()
		m.share = nil
	}
	m.screen = screenPicker
	m.modal = modalNone
	m.picker = newPicker(m.deps)
	return m, m.picker.countCmds()
}

func (m Model) modalKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := strings.ToLower(k.String())
	if m.modal == modalConfirm {
		switch key {
		case "y":
			a := m.confirmAction
			m.modal, m.confirmAction = modalNone, nil
			return a(m)
		case "n", "esc":
			m.modal, m.confirmAction = modalNone, nil
		}
		return m, nil
	}
	switch key { // help or link box
	case "esc", "q", "enter":
		m.modal, m.linkText = modalNone, ""
	}
	return m, nil
}

func (m Model) modalView() string {
	switch m.modal {
	case modalConfirm:
		return "\n " + titleStyle.Render(m.confirmText)
	case modalLink:
		out := []string{"", " " + titleStyle.Render("Copy this link") + " (the clipboard isn't available here):", ""}
		for _, l := range strings.Split(ansi.Hardwrap(m.linkText, max(m.width-4, 10), true), "\n") {
			out = append(out, "  "+l)
		}
		return strings.Join(append(out, "", " "+dimStyle.Render("esc to close")), "\n")
	}
	lines := []string{
		" " + titleStyle.Render("lanstream help"),
		"",
		" Commands  /h help · /f share another folder · /q quit (Ctrl-C too)",
		" Keys      tab switch tabs · ↑ ↓ pgup pgdown home end move",
		"           enter or double-click a video: copy its link",
		"           double-click the top line: copy the playlist link",
		"",
		" Playlist links",
	}
	if m.share != nil {
		for _, u := range m.share.PlaylistURLs() {
			lines = append(lines, "   "+u)
		}
	}
	lines = append(lines, "", " Tips",
		"   Phone and computer must be on the same Wi-Fi or hotspot.",
		"   In VLC: Open Network Stream, paste the playlist link.")
	if m.deps.GOOS == "darwin" {
		lines = append(lines,
			"   If your Mac asks to allow incoming connections, click Allow.",
			"   Closing the lid on battery still puts the Mac to sleep.")
	} else {
		lines = append(lines, "   If a firewall is on, allow incoming connections for lanstream.")
	}
	lines = append(lines, "", " "+dimStyle.Render("lanstream "+m.deps.Version+" · esc to close"))
	return strings.Join(lines, "\n")
}
