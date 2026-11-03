// Package tui is lanstream's full-screen terminal interface: a folder picker, then a serving
// screen with Videos and Viewers tabs and a command bar.
package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Abinsanth/stream-video-lan/internal/awake"
	"github.com/Abinsanth/stream-video-lan/internal/config"
	"github.com/Abinsanth/stream-video-lan/internal/library"
	"github.com/Abinsanth/stream-video-lan/internal/viewers"
)

const (
	tickEvery    = 500 * time.Millisecond
	copyFlashFor = 3 * time.Second
	tipFlashFor  = 8 * time.Second
	doubleClick  = 500 * time.Millisecond
)

// Share is what the TUI needs from a running share; *share.Share satisfies it.
type Share interface {
	Root() string
	Videos() []library.Video
	ScanErr() error
	PlaylistURL() string
	PlaylistURLs() []string
	VideoURL(rel string) string
	Stop()
}

// Deps is everything the TUI uses from the outside world, so tests can replace any of it.
type Deps struct {
	Start         func(root string) (Share, error)
	Config        config.Config
	SaveConfig    func(config.Config) error
	CopyText      func(string) error // system clipboard tool; OSC 52 is sent as well
	Tracker       *viewers.Tracker
	Keeper        *awake.Keeper
	Now           func() time.Time
	Home          string
	GOOS          string
	InitialFolder string // non-empty: share it straight away, skipping the picker
	Version       string

	// CheckUpdate returns the newest released version; UpdateNotes its release notes.
	// Either may be nil (no update checks).
	CheckUpdate func(ctx context.Context) (latest string, err error)
	UpdateNotes func(ctx context.Context, version string) (string, error)
}

type screen int

const (
	screenPicker screen = iota
	screenServing
)

type modalKind int

const (
	modalNone modalKind = iota
	modalHelp
	modalConfirm
	modalLink   // copying failed: show the whole link until dismissed
	modalUpdate // /update
	modalQR     // /qr
)

// stopNext says what to do once a share has finished stopping.
type stopNext int

const (
	afterQuit stopNext = iota
	afterPicker
)

type (
	tickMsg    struct{}
	stoppedMsg struct{ next stopNext }
	startReq   struct{ dir string }
	countMsg   struct {
		dir string
		n   int // -1 when the folder can't be scanned
	}
)

// action is something to run, possibly after the user confirms it.
type action func(Model) (Model, tea.Cmd)

// Model is the whole UI state. It is a value; Update returns the next one.
type Model struct {
	deps   Deps
	screen screen
	width  int
	height int
	share  Share

	flash      string
	flashUntil time.Time
	notice     string // "vX available — run lanstream --update", until dismissed

	updateInfo updateInfo

	stopping bool // a share is shutting down in the background
	quitting bool // tea.Quit has been sent; Bubble Tea still draws one last frame

	modal         modalKind
	linkText      string
	confirmText   string
	confirmAction action

	picker  pickerState
	serving servingState
	bar     cmdState
}

// NewModel builds the initial model (the picker, or the serving screen once Init's start runs).
func NewModel(d Deps) Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	m := Model{deps: d, width: 80, height: 24}
	m.picker = newPicker(m.deps)
	m.bar = newCmdBar()
	return m
}

// Init starts the refresh tick, the recent-folder counts, and the initial share if one was given.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), m.picker.countCmds(), startupCheckCmd(m.deps)}
	if m.deps.InitialFolder != "" {
		dir := m.deps.InitialFolder
		cmds = append(cmds, func() tea.Msg { return startReq{dir: dir} })
	}
	return tea.Batch(cmds...)
}

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.fixScroll()
		m.syncPickerScroll()
		return m, nil
	case stoppedMsg:
		return m.afterStop(msg.next)
	case updateMsg:
		m.notice = noticeFor(msg.latest)
		return m, nil
	case updateInfoMsg:
		m.updateInfo = updateInfo{latest: msg.latest, notes: msg.notes, err: msg.err}
		return m, nil
	case tickMsg:
		m.onTick()
		return m, tick()
	case copiedMsg:
		if msg.err == nil {
			m.setFlash("Copied", copyFlashFor)
		} else {
			// No clipboard tool (and the terminal may ignore OSC 52): show the whole link,
			// wrapped, and keep it up until dismissed so it can be copied by hand.
			m.modal, m.linkText = modalLink, msg.text
		}
		return m, nil
	case startReq:
		return m.startShare(msg.dir)
	case countMsg:
		m.picker.counts[msg.dir] = msg.n
		return m, nil
	case tea.KeyPressMsg:
		return m.onKey(msg)
	case tea.MouseClickMsg:
		// Not while stopping: the share has already been handed off to shut down.
		if m.screen == screenServing && m.modal == modalNone && !m.stopping && m.share != nil {
			return m.servingClick(msg.Mouse())
		}
		if m.screen == screenPicker && m.modal == modalNone && !m.bar.focused {
			return m.pickerClick(msg.Mouse())
		}
		return m, nil
	}
	// Anything else (paste, cursor blink) goes to whichever text input is active.
	return m.forwardToInput(msg)
}

func (m Model) onKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.stopping {
		return m, nil // already on the way out
	}
	if k.String() == "ctrl+c" {
		if m.modal == modalConfirm { // a second Ctrl-C means "yes, really"
			return m.quit()
		}
		return m.confirmOr(m.quitAction())
	}
	if m.modal != modalNone {
		return m.modalKey(k)
	}
	// One command bar on every screen. While typing a path, "/" is part of the path.
	if m.bar.focused {
		return m.barKey(k)
	}
	if k.Text == "/" && !(m.screen == screenPicker && m.picker.mode == pickType) {
		return m.focusBar()
	}
	if m.screen == screenPicker {
		nm, cmd := m.pickerKey(k)
		if pm, ok := nm.(Model); ok {
			pm.syncPickerScroll()
			return pm, cmd
		}
		return nm, cmd
	}
	return m.servingKey(k)
}

// onTick refreshes what the screen shows from the share and tracker, drives the stay-awake
// keeper, and expires the flash message.
func (m *Model) onTick() {
	if m.deps.Tracker != nil {
		m.deps.Keeper.Update(m.deps.Tracker.Active())
	}
	if m.flash != "" && !m.deps.Now().Before(m.flashUntil) {
		m.flash = ""
	}
	if m.screen == screenServing && m.share != nil {
		m.refreshServing()
	}
}

func (m *Model) setFlash(text string, d time.Duration) {
	m.flash, m.flashUntil = text, m.deps.Now().Add(d)
}

// startShare begins sharing dir and switches to the serving screen, or reports why it can't.
func (m Model) startShare(dir string) (tea.Model, tea.Cmd) {
	if m.share != nil { // e.g. Enter typed ahead before the initial folder started
		m.share.Stop()
		m.share = nil
	}
	s, err := m.deps.Start(dir)
	if err != nil || s == nil {
		if err == nil {
			err = errors.New("could not start")
		}
		m.picker.err = "Can't share " + tildePath(dir, m.deps.Home) + ": " + err.Error()
		m.screen = screenPicker
		return m, nil
	}
	m.share = s
	m.screen = screenServing
	m.picker.err = ""
	m.deps.Config.AddRecent(s.Root())
	m.serving = servingState{}
	m.refreshServing()
	switch {
	case !m.deps.Config.TipShown:
		m.setFlash(firstRunTip(m.deps.GOOS), tipFlashFor)
		m.deps.Config.TipShown = true
	case len(m.serving.videos) == 0 && s.ScanErr() == nil:
		m.setFlash("No videos here yet. New files show up automatically.", tipFlashFor)
	}
	m.saveConfig()
	return m, nil
}

func (m Model) saveConfig() {
	if m.deps.SaveConfig != nil {
		m.deps.SaveConfig(m.deps.Config)
	}
}

func firstRunTip(goos string) string {
	if goos == "darwin" {
		return "Tip: if your Mac asks to allow incoming connections, click Allow. /h for help."
	}
	return "Tip: phone and computer must be on the same Wi-Fi or hotspot. /h for help."
}

// confirmOr runs a, first asking for confirmation if anyone is watching.
func (m Model) confirmOr(a action) (tea.Model, tea.Cmd) {
	n := 0
	if m.screen == screenServing && m.deps.Tracker != nil {
		n = len(m.deps.Tracker.Snapshot())
	}
	if n == 0 {
		return a(m)
	}
	who := "1 person is"
	if n > 1 {
		who = strconv.Itoa(n) + " people are"
	}
	m.modal, m.confirmText, m.confirmAction = modalConfirm, who+" watching. Stop anyway? (y/n)", a
	return m, nil
}

func (m Model) quitAction() action { return func(m Model) (Model, tea.Cmd) { return m.quitNow() } }

func (m Model) quit() (tea.Model, tea.Cmd) { return m.quitNow() }

func (m Model) quitNow() (Model, tea.Cmd) {
	m.modal = modalNone
	if m.share != nil {
		return m.stopShare(afterQuit)
	}
	m.deps.Keeper.Close()
	m.quitting = true
	return m, tea.Quit
}

// stopShare shuts the current share down in the background (it can take up to 3 s while
// someone is mid-stream), showing "Stopping…" until stoppedMsg arrives.
func (m Model) stopShare(next stopNext) (Model, tea.Cmd) {
	s := m.share
	m.share, m.stopping, m.modal = nil, true, modalNone
	return m, func() tea.Msg {
		s.Stop()
		return stoppedMsg{next: next}
	}
}

func (m Model) afterStop(next stopNext) (tea.Model, tea.Cmd) {
	m.stopping = false
	if next == afterQuit {
		m.deps.Keeper.Close()
		m.quitting = true
		return m, tea.Quit
	}
	if m.deps.Tracker != nil {
		m.deps.Tracker.Reset()
	}
	m.screen = screenPicker
	m.picker = newPicker(m.deps)
	return m, m.picker.countCmds()
}

// View renders the current screen, clipped to the terminal so nothing wraps.
func (m Model) View() tea.View {
	var body string
	switch {
	case m.quitting:
		// The last frame: leave the terminal clean.
	case m.screen == screenServing && m.share == nil && !m.stopping:
		// Between shares; nothing to draw.
	case m.stopping:
		body = "\n " + dimStyle.Render("Stopping…")
	case m.modal != modalNone:
		body = m.modalView()
	case m.screen == screenPicker:
		body = m.pickerView()
	default:
		body = m.servingView()
	}
	v := tea.NewView(clip(body, m.width, m.height))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "lanstream"
	return v
}

// clip truncates every line to w columns and keeps at most h lines.
func clip(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if h >= 0 && len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = fit(l, w)
	}
	return strings.Join(lines, "\n")
}

// Run shows the TUI until the user quits or ctx ends, then stops any share and releases the
// stay-awake hold.
func Run(ctx context.Context, d Deps) error {
	final, err := tea.NewProgram(NewModel(d), tea.WithContext(ctx)).Run()
	if fm, ok := final.(Model); ok && fm.share != nil {
		fm.share.Stop()
	}
	d.Keeper.Close()
	if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
