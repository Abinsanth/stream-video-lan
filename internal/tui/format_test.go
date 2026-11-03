package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{0: "0 B", 999: "999 B", 1000: "1.0 KB", 1_200_000_000: "1.2 GB", 3_400_000_000_000: "3.4 TB"}
	for n, want := range cases {
		if got := humanSize(n); got != want {
			t.Errorf("%d: %q", n, got)
		}
	}
}

func TestTildePath(t *testing.T) {
	if tildePath("/Users/a/Movies", "/Users/a") != "~/Movies" || tildePath("/Users/a", "/Users/a") != "~" || tildePath("/Users/ab", "/Users/a") != "/Users/ab" {
		t.Fatal()
	}
}

func TestCleanPath(t *testing.T) {
	cases := map[string]string{
		`/Users/a/My\ Movies `: "/Users/a/My Movies", `'/Users/a/My Movies'`: "/Users/a/My Movies",
		`"/x/y"`: "/x/y", `~/Movies`: "/Users/a/Movies", `/x/\(1\)`: "/x/(1)", `  /plain  `: "/plain", `~`: "/Users/a",
	}
	for in, want := range cases {
		if got := cleanPath(in, "/Users/a"); got != want {
			t.Errorf("%q → %q", in, got)
		}
	}
}

func TestFitByDisplayWidth(t *testing.T) {
	if got := fit("മലയാളം-movie.mkv", 8); lipgloss.Width(got) > 8 || !utf8.ValidString(got) || !strings.HasSuffix(got, "…") {
		t.Fatal(got)
	}
	if fit("abc", 0) != "" || fit("abc", 10) != "abc" {
		t.Fatal()
	}
}
