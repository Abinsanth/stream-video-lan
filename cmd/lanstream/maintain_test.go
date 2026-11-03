package main

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
	"runtime"
	"strings"
	"testing"

	"github.com/Abinsanth/stream-video-lan/internal/update"
	"github.com/Abinsanth/stream-video-lan/internal/version"
)

var ctx = context.Background()

func setVersion(t *testing.T, v string) {
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

// fakeRelease points the updater at a fake GitHub whose latest release is ver, carrying bin for
// this machine's OS and architecture.
func fakeRelease(t *testing.T, ver string, bin []byte) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "lanstream", Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
	tw.Write(bin)
	tw.Close()
	gz.Close()
	arc := buf.Bytes()
	asset := "lanstream_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	dl := "/" + update.Repo + "/releases/download/v" + ver + "/"
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + update.Repo + "/releases/latest":
			http.Redirect(w, r, ts.URL+"/"+update.Repo+"/releases/tag/v"+ver, http.StatusFound)
		case dl + "checksums.txt":
			fmt.Fprintf(w, "%x  %s\n", sha256.Sum256(arc), asset)
		case dl + asset:
			w.Write(arc)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	old := updater
	updater = update.Client{Web: ts.URL, API: ts.URL, HTTP: ts.Client()}
	t.Cleanup(func() { updater = old })
}

// fakeExe makes "the running lanstream" a temp file containing OLD and returns its path.
func fakeExe(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lanstream")
	if err := os.WriteFile(p, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	useExecutable(t, p)
	return p
}

func useExecutable(t *testing.T, p string) {
	old := osExecutable
	osExecutable = func() (string, error) { return p, nil }
	t.Cleanup(func() { osExecutable = old })
}

// fakeConfig makes a temp config folder with a file in it and returns its path.
func fakeConfig(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "lanstream")
	os.MkdirAll(d, 0o700)
	os.WriteFile(filepath.Join(d, "config.json"), []byte("{}"), 0o600)
	old := configDir
	configDir = func() (string, error) { return d, nil }
	t.Cleanup(func() { configDir = old })
	return d
}

func TestUpdateDevBuild(t *testing.T) {
	setVersion(t, "dev")
	var out bytes.Buffer
	if code := run(ctx, []string{"--update"}, strings.NewReader("y\n"), &out, &out); code != 1 || !strings.Contains(out.String(), update.InstallLine) {
		t.Fatal(code, out.String())
	}
}

func TestUpdateUpToDate(t *testing.T) {
	setVersion(t, "0.2.0")
	fakeRelease(t, "0.2.0", []byte("NEW"))
	exe := fakeExe(t)
	var out bytes.Buffer
	if code := run(ctx, []string{"--update"}, strings.NewReader(""), &out, &out); code != 0 || !strings.Contains(out.String(), "lanstream v0.2.0 is up to date.") {
		t.Fatal(code, out.String())
	}
	if b, _ := os.ReadFile(exe); string(b) != "OLD" {
		t.Fatal("changed")
	}
}

func TestUpdateDeclined(t *testing.T) {
	setVersion(t, "0.1.0")
	fakeRelease(t, "0.2.0", []byte("NEW"))
	exe := fakeExe(t)
	var out bytes.Buffer
	run(ctx, []string{"--update"}, strings.NewReader("n\n"), &out, &out)
	if !strings.Contains(out.String(), "Update v0.1.0 → v0.2.0? (y/n)") || !strings.Contains(out.String(), "No changes made.") {
		t.Fatal(out.String())
	}
	if b, _ := os.ReadFile(exe); string(b) != "OLD" {
		t.Fatal("changed after n")
	}
}

func TestUpdateAccepted(t *testing.T) {
	setVersion(t, "0.1.0")
	fakeRelease(t, "0.2.0", []byte("NEW"))
	exe := fakeExe(t)
	var out bytes.Buffer
	if code := run(ctx, []string{"--update"}, strings.NewReader("YES\n"), &out, &out); code != 0 {
		t.Fatal(out.String())
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" || !strings.Contains(out.String(), "Updated to v0.2.0. Running lanstream windows keep the old version until restarted.") {
		t.Fatal(string(b), out.String())
	}
}

func TestUpdateThroughSymlinkReplacesTarget(t *testing.T) {
	setVersion(t, "0.1.0")
	fakeRelease(t, "0.2.0", []byte("NEW"))
	dir := t.TempDir()
	target, link := filepath.Join(dir, "real-lanstream"), filepath.Join(dir, "lanstream")
	os.WriteFile(target, []byte("OLD"), 0o755)
	os.Symlink(target, link)
	useExecutable(t, link)
	var out bytes.Buffer
	if code := run(ctx, []string{"--update"}, strings.NewReader("y\n"), &out, &out); code != 0 {
		t.Fatal(out.String())
	}
	if b, _ := os.ReadFile(target); string(b) != "NEW" {
		t.Fatal("target not updated")
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced by a file")
	}
}

func TestUpdateUnwritableSaysReinstall(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	setVersion(t, "0.1.0")
	fakeRelease(t, "0.2.0", []byte("NEW"))
	exe := fakeExe(t)
	os.Chmod(filepath.Dir(exe), 0o555)
	t.Cleanup(func() { os.Chmod(filepath.Dir(exe), 0o755) })
	var out bytes.Buffer
	if code := run(ctx, []string{"--update"}, strings.NewReader("y\n"), &out, &out); code != 1 || !strings.Contains(out.String(), "can't write") || !strings.Contains(out.String(), update.InstallLine) {
		t.Fatal(code, out.String())
	}
}

func TestPromptEOFMeansNo(t *testing.T) {
	setVersion(t, "0.1.0")
	fakeRelease(t, "0.2.0", []byte("NEW"))
	exe := fakeExe(t)
	cfg := fakeConfig(t)
	var out bytes.Buffer
	run(ctx, []string{"--update"}, strings.NewReader(""), &out, &out)
	run(ctx, []string{"--uninstall"}, strings.NewReader(""), &out, &out)
	if b, _ := os.ReadFile(exe); string(b) != "OLD" {
		t.Fatal("binary changed on EOF")
	}
	if _, err := os.Stat(cfg); err != nil {
		t.Fatal("config removed on EOF")
	}
}

func TestUninstallRemovesBinaryAndConfig(t *testing.T) {
	setVersion(t, "0.1.0")
	exe := fakeExe(t)
	cfg := fakeConfig(t)
	var out bytes.Buffer
	if code := run(ctx, []string{"--uninstall"}, strings.NewReader("y\n"), &out, &out); code != 0 {
		t.Fatal(out.String())
	}
	if !strings.Contains(out.String(), "Remove lanstream and its settings? (y/n)") || !strings.Contains(out.String(), "lanstream removed.") {
		t.Fatal(out.String())
	}
	for _, p := range []string{exe, cfg} {
		if !strings.Contains(out.String(), p) {
			t.Errorf("didn't name %s", p)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still there", p)
		}
	}
}

func TestUninstallThroughSymlink(t *testing.T) {
	setVersion(t, "0.1.0")
	fakeConfig(t)
	dir := t.TempDir()
	target, link := filepath.Join(dir, "real-lanstream"), filepath.Join(dir, "lanstream")
	os.WriteFile(target, []byte("OLD"), 0o755)
	os.Symlink(target, link)
	useExecutable(t, link)
	var out bytes.Buffer
	run(ctx, []string{"--uninstall"}, strings.NewReader("y\n"), &out, &out)
	if !strings.Contains(out.String(), target) {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("target still there")
	}
}

func TestUninstallDevBuildKeepsBinary(t *testing.T) {
	setVersion(t, "dev")
	exe := fakeExe(t)
	cfg := fakeConfig(t)
	var out bytes.Buffer
	run(ctx, []string{"--uninstall"}, strings.NewReader("y\n"), &out, &out)
	if _, err := os.Stat(exe); err != nil {
		t.Fatal("dev uninstall removed the go build output")
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Fatal("config kept")
	}
}

func TestUsageListsMaintenanceFlags(t *testing.T) {
	var out bytes.Buffer
	run(ctx, []string{"-h"}, strings.NewReader(""), &out, io.Discard)
	for _, s := range []string{"lanstream --update          update to the latest version", "lanstream --uninstall       remove lanstream and its settings"} {
		if !strings.Contains(out.String(), s) {
			t.Fatal(out.String())
		}
	}
}
