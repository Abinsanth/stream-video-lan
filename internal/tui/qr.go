package tui

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rsc.io/qr"
)

const qrQuiet = 2 // modules of white border scanners need around the code

// Each character cell shows two modules stacked: "▀" in the top module's colour on the bottom
// module's colour. Colours are explicit black and white, so the code scans the right way round
// on dark and light terminals alike.
var qrCells = func() [2][2]string {
	shade := [2]color.Color{lipgloss.Color("#FFFFFF"), lipgloss.Color("#000000")} // 0 white, 1 black
	var cells [2][2]string
	for top := 0; top < 2; top++ {
		for bottom := 0; bottom < 2; bottom++ {
			cells[top][bottom] = lipgloss.NewStyle().Foreground(shade[top]).Background(shade[bottom]).Render("▀")
		}
	}
	return cells
}()

// qrLines renders text as a QR code: (size+4) columns wide, half as many lines tall.
func qrLines(text string) ([]string, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return nil, err
	}
	side := code.Size + 2*qrQuiet
	dark := func(x, y int) int {
		if code.Black(x-qrQuiet, y-qrQuiet) { // out-of-range modules (the border) are white
			return 1
		}
		return 0
	}
	var lines []string
	for y := 0; y < side; y += 2 {
		var b strings.Builder
		for x := 0; x < side; x++ {
			b.WriteString(qrCells[dark(x, y)][dark(x, y+1)])
		}
		lines = append(lines, b.String())
	}
	return lines, nil
}

// openQR shows a QR code for phones to scan.
func (m Model) openQR() (tea.Model, tea.Cmd) {
	m.modal = modalQR
	return m, nil
}

// qrLink is what the code holds: lanstream's own page, where the phone taps "Open in VLC". A
// camera only hands links to the browser, so pointing straight at the playlist can't reach VLC.
func (m Model) qrLink() string {
	return strings.TrimSuffix(m.share.PlaylistURL(), "playlist.m3u8")
}

func (m Model) qrView() string {
	if m.share == nil {
		return ""
	}
	link := m.qrLink()
	out := []string{" " + titleStyle.Render("Scan with your phone, then tap Open in VLC"), ""}
	code, err := qrLines(link)
	if err != nil || len(code) == 0 || m.width < lipgloss.Width(code[0])+2 || m.height < len(code)+6 {
		out = append(out, " Make the window bigger to show the QR code.")
	} else {
		for _, l := range code {
			out = append(out, "  "+l)
		}
	}
	return strings.Join(append(out, "", " Link: "+link, " "+dimStyle.Render("Same Wi-Fi or hotspot as this computer · esc to close")), "\n")
}
