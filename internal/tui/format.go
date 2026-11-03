package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// humanSize formats bytes with decimal units, as Finder does: "999 B", "1.0 KB", "1.2 GB".
func humanSize(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, unit := range []string{"KB", "MB", "GB", "TB"} {
		v /= 1000
		if v < 999.95 || unit == "TB" {
			return fmt.Sprintf("%.1f %s", v, unit)
		}
	}
	return "" // unreachable
}

// tildePath shows paths under home as "~/…".
func tildePath(p, home string) string {
	if home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return p
}

// cleanPath turns what people type or drag into a terminal into a plain path: it trims space,
// removes one pair of surrounding quotes, undoes shell backslash escapes ("My\ Movies"), and
// expands a leading "~".
func cleanPath(s, home string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	} else {
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				i++
			}
			b.WriteByte(s[i])
		}
		s = b.String()
	}
	if s == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(s, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return s
}

// fit truncates s to w display columns (ANSI- and wide-character-aware), ending in "…".
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// pad fits s into exactly w display columns.
func pad(s string, w int) string {
	s = fit(s, w)
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// displayName makes a file or folder name safe to draw: control characters (newline, tab,
// escape, DEL…) become "?", so a strange name can't add screen lines, shift columns or inject
// terminal escape sequences.
func displayName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}
