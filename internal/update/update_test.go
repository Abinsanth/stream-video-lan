package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitHub serves the latest-release redirect, release downloads and the release API.
func fakeGitHub(t *testing.T, version string, archives map[string][]byte, sums string) *httptest.Server {
	t.Helper()
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl := "/" + Repo + "/releases/download/v" + version + "/"
		switch {
		case r.URL.Path == "/"+Repo+"/releases/latest":
			http.Redirect(w, r, ts.URL+"/"+Repo+"/releases/tag/v"+version, http.StatusFound)
		case r.URL.Path == "/repos/"+Repo+"/releases/tags/v"+version:
			w.Write([]byte(`{"tag_name":"v` + version + `","body":"notes"}`))
		case r.URL.Path == dl+"checksums.txt":
			w.Write([]byte(sums))
		case strings.HasPrefix(r.URL.Path, dl):
			b, ok := archives[strings.TrimPrefix(r.URL.Path, dl)]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func client(ts *httptest.Server) Client { return Client{Web: ts.URL, API: ts.URL, HTTP: ts.Client()} }

func tarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tw.Write(data)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func sumLine(b []byte, name string) string { return fmt.Sprintf("%x  %s\n", sha256.Sum256(b), name) }

func TestLatestReadsRedirect(t *testing.T) {
	c := client(fakeGitHub(t, "0.2.0", nil, ""))
	if v, err := c.Latest(context.Background()); err != nil || v != "0.2.0" {
		t.Fatal(v, err)
	}
	if n, err := c.Notes(context.Background(), "0.2.0"); err != nil || n != "notes" {
		t.Fatal(n, err)
	}
}

func TestLatestNoReleaseOrCaptivePortal(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>login</html>"))
	}))
	defer portal.Close()
	if _, err := client(portal).Latest(context.Background()); err == nil {
		t.Fatal("captive portal accepted as a release")
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.2.0", "0.1.0", true}, {"0.10.0", "0.9.0", true}, {"1.0.0", "0.99.99", true},
		{"0.1.0", "0.1.0", false}, {"0.1.0", "0.2.0", false}, {"0.2.0", "dev", false}, {"x", "0.1.0", false},
		{"0.2", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q,%q)=%v", c.latest, c.current, got)
		}
	}
}

func TestDownloadVerifiesAndExtracts(t *testing.T) {
	arc := tarGz(t, "lanstream", []byte("NEWBIN"))
	c := client(fakeGitHub(t, "0.2.0", map[string][]byte{"lanstream_linux_arm64.tar.gz": arc}, sumLine(arc, "lanstream_linux_arm64.tar.gz")))
	bin, err := c.Download(context.Background(), "0.2.0", "linux", "arm64")
	if err != nil || string(bin) != "NEWBIN" {
		t.Fatal(string(bin), err)
	}
}

func TestDownloadRejectsBadArchives(t *testing.T) {
	const name = "lanstream_darwin_arm64.tar.gz"
	good := tarGz(t, "lanstream", []byte("NEWBIN"))
	cases := map[string]struct {
		arc     []byte
		sums    string
		wantErr string
	}{
		"mismatch":  {good, "0000  " + name + "\n", "checksum mismatch"},
		"unlisted":  {good, "", "not listed in checksums.txt"},
		"truncated": {good[:len(good)/2], "auto", ""},
		"no binary": {tarGz(t, "README", []byte("x")), "auto", "no lanstream binary"},
		"html":      {[]byte("<html>oops</html>"), "auto", ""},
	}
	for label, tc := range cases {
		sums := tc.sums
		if sums == "auto" {
			sums = sumLine(tc.arc, name)
		}
		c := client(fakeGitHub(t, "0.2.0", map[string][]byte{name: tc.arc}, sums))
		_, err := c.Download(context.Background(), "0.2.0", "darwin", "arm64")
		if err == nil || (tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: err = %v", label, err)
		}
	}
	c := client(fakeGitHub(t, "0.2.0", nil, sumLine(good, name)))
	if _, err := c.Download(context.Background(), "0.2.0", "darwin", "arm64"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("missing asset: err = %v", err)
	}
}

func TestReplaceSwapsFileAndKeepsOldInodeIntact(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lanstream")
	os.WriteFile(p, []byte("OLD"), 0o755)
	old, err := os.Open(p) // a "running" copy
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err := Replace(p, []byte("NEW")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "NEW" {
		t.Fatal(string(b))
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o755 {
		t.Fatal(fi.Mode())
	}
	if b, _ := io.ReadAll(old); string(b) != "OLD" {
		t.Fatal("old inode was overwritten")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Fatal("temp file left behind")
	}
}

func TestReplaceThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real, link := filepath.Join(dir, "real"), filepath.Join(dir, "link")
	os.WriteFile(real, []byte("OLD"), 0o755)
	os.Symlink(real, link)
	if err := Replace(link, []byte("NEW")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(real); string(b) != "NEW" {
		t.Fatal("target not replaced")
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("link replaced by a file")
	}
}

func TestReplaceUnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "lanstream")
	os.WriteFile(p, []byte("OLD"), 0o755)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if err := Replace(p, []byte("NEW")); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(p); string(b) != "OLD" {
		t.Fatal("file changed despite error")
	}
}
