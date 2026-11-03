// Package library finds the videos in a shared folder and maps URL paths back to files safely.
package library

import (
	"path/filepath"
	"strings"
)

// contentTypes maps a lowercase extension (with dot) to the Content-Type served for it.
// Every key is a video extension; nothing else is listed or served.
var contentTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".mkv":  "video/x-matroska",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".wmv":  "video/x-ms-wmv",
	".flv":  "video/x-flv",
	".ts":   "video/mp2t",
	".m2ts": "video/mp2t",
	".mts":  "video/mp2t",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
	".3gp":  "video/3gpp",
	".ogv":  "video/ogg",
}

// IsVideo reports whether name has a video extension (case-insensitive).
func IsVideo(name string) bool {
	return ContentType(name) != ""
}

// ContentType returns the Content-Type for a video file name, or "" if it isn't a video.
func ContentType(name string) string {
	return contentTypes[strings.ToLower(filepath.Ext(name))]
}

// skipName reports whether a file or folder should be ignored while scanning:
// hidden entries (including macOS "._" files) and Windows system folders on external drives.
func skipName(name string) bool {
	return strings.HasPrefix(name, ".") || name == "$RECYCLE.BIN" || name == "System Volume Information"
}

// NaturalLess orders names the way people expect: case-insensitive, with digit runs compared
// by value, so "S01E2" sorts before "S01E10". Equal values ("2" vs "02") put the shorter run
// first, and a final byte comparison keeps the order total.
func NaturalLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(la) && j < len(lb) {
		ra, ni := chunk(la, i)
		rb, nj := chunk(lb, j)
		if c := compareChunk(ra, rb); c != 0 {
			return c < 0
		}
		i, j = ni, nj
	}
	if rest := (len(la) - i) - (len(lb) - j); rest != 0 {
		return rest < 0
	}
	return a < b
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// chunk returns the run of digits or non-digits starting at i, and the index after it.
func chunk(s string, i int) (string, int) {
	d := isDigit(s[i])
	j := i + 1
	for j < len(s) && isDigit(s[j]) == d {
		j++
	}
	return s[i:j], j
}

func compareChunk(a, b string) int {
	if isDigit(a[0]) && isDigit(b[0]) {
		ta, tb := strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		if len(ta) != len(tb) {
			return len(ta) - len(tb)
		}
		if c := strings.Compare(ta, tb); c != 0 {
			return c
		}
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}
