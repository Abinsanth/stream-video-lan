// Package installtest checks the repo's shell scripts (install.sh against a fake release server,
// and the release tagging script against a fake gh). It has no other code.
package installtest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeBinary = "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'lanstream 9.9.9'; fi\n"

type sumsMode int

const (
	goodChecksums sumsMode = iota
	badChecksums
)

func availableShells(t *testing.T) []string {
	shells := []string{"sh"}
	if _, err := exec.LookPath("dash"); err == nil {
		shells = append(shells, "dash")
	}
	return shells
}

// runInstall serves a fake release and runs install.sh with shell. It returns the install dir,
// the combined output, and the run error.
func runInstall(t *testing.T, shell string, mode sumsMode, extraEnv ...string) (string, string, error) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "lanstream", Mode: 0o755, Size: int64(len(fakeBinary)), Typeflag: tar.TypeReg})
	tw.Write([]byte(fakeBinary))
	tw.Close()
	gz.Close()
	arc := buf.Bytes()
	asset := "lanstream_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(arc), asset)
	if mode == badChecksums {
		sums = strings.Repeat("0", 64) + "  " + asset + "\n"
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + asset:
			w.Write(arc)
		case "/checksums.txt":
			w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	home := t.TempDir()
	dir := filepath.Join(home, "bin")
	cmd := exec.Command(shell, "../../install.sh")
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"LANSTREAM_DOWNLOAD_BASE=" + ts.URL,
		"LANSTREAM_INSTALL_DIR=" + dir,
	}
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return dir, string(out), err
}

func TestInstallScriptPOSIX(t *testing.T) {
	for _, shell := range availableShells(t) {
		dir, out, err := runInstall(t, shell, goodChecksums)
		if err != nil {
			t.Fatalf("%s: %v\n%s", shell, err, out)
		}
		b, err := exec.Command(filepath.Join(dir, "lanstream"), "--version").Output()
		if err != nil || !strings.Contains(string(b), "9.9.9") {
			t.Fatalf("%s: installed binary: %q %v", shell, b, err)
		}
		for _, want := range []string{"Installed lanstream 9.9.9 to " + dir + "/lanstream", "Add it to your PATH", "Run: lanstream"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: output missing %q:\n%s", shell, want, out)
			}
		}
		if ents, _ := os.ReadDir(dir); len(ents) != 1 {
			t.Errorf("%s: leftovers in install dir: %v", shell, ents)
		}
	}
}

func TestInstallScriptRejectsBadChecksum(t *testing.T) {
	dir, out, err := runInstall(t, "sh", badChecksums)
	if err == nil || !strings.Contains(out, "Checksum mismatch: nothing was installed.") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "lanstream")); !os.IsNotExist(statErr) {
		t.Fatal("installed despite mismatch")
	}
}

func TestInstallScriptNoSudo(t *testing.T) {
	b, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sudo") {
		t.Fatal("install.sh mentions sudo")
	}
}

func TestInstallScriptPathHintFollowsShell(t *testing.T) {
	bashRC := ".bashrc"
	if runtime.GOOS == "darwin" {
		bashRC = ".bash_profile"
	}
	cases := map[string]string{
		"/bin/zsh":      "/.zshrc",
		"/usr/bin/zsh":  "/.zshrc",
		"/bin/bash":     "/" + bashRC,
		"/usr/bin/fish": "fish_add_path",
		"/bin/sh":       "/.profile",
		"":              "/.profile",
	}
	for sh, want := range cases {
		_, out, err := runInstall(t, "sh", goodChecksums, "SHELL="+sh)
		if err != nil || !strings.Contains(out, want) {
			t.Errorf("SHELL=%q: want %q in\n%s", sh, want, out)
		}
	}
}
