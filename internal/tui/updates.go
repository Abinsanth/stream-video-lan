package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	upd "github.com/Abinsanth/stream-video-lan/internal/update"
)

const (
	startupCheckTimeout = 2 * time.Second
	modalCheckTimeout   = 5 * time.Second
	maxNotesLines       = 10
)

type (
	// updateMsg reports, once at startup, that a newer release exists.
	updateMsg struct{ latest string }
	// updateInfoMsg answers /update.
	updateInfoMsg struct {
		latest, notes string
		err           error
	}
)

// updateInfo is what the /update box shows.
type updateInfo struct {
	loading       bool
	latest, notes string
	err           error
}

// startupCheckCmd asks GitHub once, quietly, whether a newer version exists. It is nil for
// development builds or when there is no checker, and its message is nil on any failure, so
// being offline never shows an error or slows anything down.
func startupCheckCmd(d Deps) tea.Cmd {
	if d.Version == "dev" || d.CheckUpdate == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), startupCheckTimeout)
		defer cancel()
		latest, err := d.CheckUpdate(ctx)
		if err != nil || !upd.Newer(latest, d.Version) {
			return nil
		}
		return updateMsg{latest: latest}
	}
}

func noticeFor(latest string) string { return "v" + latest + " available — run lanstream --update" }

// openUpdate shows the /update box and, unless this is a development build, fetches the
// latest version and its notes in the background.
func (m Model) openUpdate() (tea.Model, tea.Cmd) {
	m.modal = modalUpdate
	if m.deps.Version == "dev" {
		m.updateInfo = updateInfo{}
		return m, nil
	}
	m.updateInfo = updateInfo{loading: true}
	d := m.deps
	return m, func() tea.Msg {
		if d.CheckUpdate == nil {
			return updateInfoMsg{err: errors.New("update checks aren't available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), modalCheckTimeout)
		defer cancel()
		latest, err := d.CheckUpdate(ctx)
		if err != nil {
			return updateInfoMsg{err: err}
		}
		notes := ""
		if upd.Newer(latest, d.Version) && d.UpdateNotes != nil {
			notes, _ = d.UpdateNotes(ctx, latest)
		}
		return updateInfoMsg{latest: latest, notes: notes}
	}
}

func (m Model) updateView() string {
	lines := []string{"", " " + titleStyle.Render("Updates"), ""}
	info := m.updateInfo
	switch {
	case m.deps.Version == "dev":
		lines = append(lines, " This is a development build. Install a release with:")
		for _, l := range strings.Split(ansi.Hardwrap(upd.InstallLine, max(m.width-4, 10), true), "\n") {
			lines = append(lines, "   "+l)
		}
	case info.loading:
		lines = append(lines, " You have v"+m.deps.Version+".", " Checking…")
	case info.err != nil:
		lines = append(lines, " You have v"+m.deps.Version+".", " Couldn't check for updates: "+info.err.Error())
	case upd.Newer(info.latest, m.deps.Version):
		lines = append(lines, " You have v"+m.deps.Version+".", " v"+info.latest+" is available.")
		if info.notes != "" {
			notes := strings.Split(info.notes, "\n")
			if len(notes) > maxNotesLines {
				notes = append(notes[:maxNotesLines], "…")
			}
			lines = append(lines, "")
			for _, n := range notes {
				lines = append(lines, "   "+displayName(strings.TrimRight(n, "\r")))
			}
		}
		lines = append(lines, "", " To install it, quit and run:  lanstream --update")
	default:
		lines = append(lines, " You have v"+m.deps.Version+".", " This is the latest version.")
	}
	return strings.Join(append(lines, "", " "+dimStyle.Render("esc to close")), "\n")
}
