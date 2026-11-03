package library

import (
	"slices"
	"testing"
)

func TestIsVideo(t *testing.T) {
	for _, n := range []string{"a.mp4", "B.MKV", "c.m2ts", "d.3gp", "e.OgV", "f.webm"} {
		if !IsVideo(n) {
			t.Errorf("IsVideo(%q) = false", n)
		}
	}
	for _, n := range []string{"a.srt", "b.txt", "mkv", "c.mp4.part", "noext"} {
		if IsVideo(n) {
			t.Errorf("IsVideo(%q) = true", n)
		}
	}
}

func TestContentType(t *testing.T) {
	want := map[string]string{
		"x.mp4": "video/mp4", "x.m4v": "video/x-m4v", "x.mkv": "video/x-matroska",
		"x.webm": "video/webm", "x.mov": "video/quicktime", "x.avi": "video/x-msvideo",
		"x.wmv": "video/x-ms-wmv", "x.flv": "video/x-flv", "x.ts": "video/mp2t",
		"x.m2ts": "video/mp2t", "x.mts": "video/mp2t", "x.mpg": "video/mpeg",
		"x.mpeg": "video/mpeg", "x.3gp": "video/3gpp", "x.ogv": "video/ogg",
		"X.MKV": "video/x-matroska", "x.srt": "",
	}
	for n, ct := range want {
		if got := ContentType(n); got != ct {
			t.Errorf("ContentType(%q) = %q, want %q", n, got, ct)
		}
	}
}

func TestSkipName(t *testing.T) {
	for _, n := range []string{".hidden", "._clip.mp4", ".DS_Store", "$RECYCLE.BIN", "System Volume Information"} {
		if !skipName(n) {
			t.Errorf("skipName(%q) = false", n)
		}
	}
	if skipName("Movies") {
		t.Error("skipName(Movies) = true")
	}
}

func TestNaturalLess(t *testing.T) {
	in := []string{"S01E10.mkv", "s01e2.mkv", "S01E1.mkv", "b/x.mp4", "A/y.mp4", "S01E02.mkv"}
	slices.SortStableFunc(in, func(a, b string) int {
		if NaturalLess(a, b) {
			return -1
		}
		if NaturalLess(b, a) {
			return 1
		}
		return 0
	})
	want := []string{"A/y.mp4", "b/x.mp4", "S01E1.mkv", "s01e2.mkv", "S01E02.mkv", "S01E10.mkv"}
	if !slices.Equal(in, want) {
		t.Fatalf("got %v, want %v", in, want)
	}
}
