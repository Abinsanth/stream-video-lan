package tui

import (
	tea "charm.land/bubbletea/v2"
)

// copiedMsg reports a finished copy. err is from the system clipboard tool; OSC 52 can't report.
type copiedMsg struct {
	text string
	err  error
}

// copyCmd copies text two ways at once: OSC 52 (works in most terminals, even over SSH) and the
// system clipboard tool (pbcopy, wl-copy, xclip), because macOS's Terminal.app ignores OSC 52.
func copyCmd(text string, copyText func(string) error) tea.Cmd {
	return tea.Batch(tea.SetClipboard(text), func() tea.Msg {
		var err error
		if copyText != nil {
			err = copyText(text)
		}
		return copiedMsg{text: text, err: err}
	})
}
