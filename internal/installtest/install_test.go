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
	env := map[string]string{
		"HOME":                    home,
		"PATH":                    "/usr/bin:/bin",
		"SHELL":                   "/bin/zsh",
		"LANSTREAM_DOWNLOAD_BASE": ts.URL,
		"LANSTREAM_INSTALL_DIR":   dir,
	}
	for _, kv := range extraEnv { // later values win; "{home}" and "{dir}" are filled in
		k, v, _ := strings.Cut(kv, "=")
		env[k] = strings.NewReplacer("{home}", home, "{dir}", dir).Replace(v)
	}
	cmd := exec.Command(shell, "../../install.sh")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
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
		for _, want := range []string{"Installed lanstream 9.9.9 to " + dir + "/lanstream", "Added " + dir + " to your PATH in ", "lanstream"} {
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

const marker = "# Added by lanstream installer"

// rcAfterInstall runs install.sh with SHELL=shell and returns the home dir and the output.
func rcAfterInstall(t *testing.T, shell string, extraEnv ...string) (string, string) {
	t.Helper()
	dir, out, err := runInstall(t, "sh", goodChecksums, append([]string{"SHELL=" + shell}, extraEnv...)...)
	if err != nil {
		t.Fatalf("SHELL=%q: %v\n%s", shell, err, out)
	}
	return filepath.Dir(dir), out
}

func TestInstallAddsPathForTheUsersShell(t *testing.T) {
	bashRC := ".bashrc"
	if runtime.GOOS == "darwin" {
		bashRC = ".bash_profile"
	}
	cases := map[string]struct{ file, line string }{
		"/bin/zsh":      {".zshrc", `export PATH="{dir}:$PATH"`},
		"/usr/bin/zsh":  {".zshrc", `export PATH="{dir}:$PATH"`},
		"/bin/bash":     {bashRC, `export PATH="{dir}:$PATH"`},
		"/usr/bin/fish": {".config/fish/conf.d/lanstream.fish", "fish_add_path {dir}"},
		"/bin/sh":       {".profile", `export PATH="{dir}:$PATH"`},
		"":              {".profile", `export PATH="{dir}:$PATH"`},
	}
	for sh, c := range cases {
		home, out := rcAfterInstall(t, sh)
		rc := filepath.Join(home, c.file)
		b, err := os.ReadFile(rc)
		line := strings.ReplaceAll(c.line, "{dir}", filepath.Join(home, "bin"))
		if err != nil || !strings.Contains(string(b), marker+"\n"+line+"\n") {
			t.Errorf("SHELL=%q: %s = %q (%v)", sh, rc, b, err)
		}
		if !strings.Contains(out, "to your PATH in "+rc) || !strings.Contains(out, "Open a new terminal") {
			t.Errorf("SHELL=%q: output doesn't say what changed:\n%s", sh, out)
		}
	}
}

func TestInstallDefaultDirUsesHOMEInRC(t *testing.T) {
	home, _ := rcAfterInstall(t, "/bin/zsh", "LANSTREAM_INSTALL_DIR={home}/.local/bin")
	b, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if !strings.Contains(string(b), `export PATH="$HOME/.local/bin:$PATH"`) {
		t.Fatalf("%q", b)
	}
}

func TestInstallKeepsExistingRCAndAddsOnce(t *testing.T) {
	home := t.TempDir()
	rc := filepath.Join(home, ".zshrc")
	os.WriteFile(rc, []byte("alias ll='ls -l'"), 0o644) // no trailing newline
	for i := 0; i < 2; i++ {
		_, out, err := runInstall(t, "sh", goodChecksums, "HOME="+home, "LANSTREAM_INSTALL_DIR="+filepath.Join(home, "bin"))
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	b, _ := os.ReadFile(rc)
	if !strings.HasPrefix(string(b), "alias ll='ls -l'\n") || strings.Count(string(b), marker) != 1 {
		t.Fatalf("%q", b)
	}
}

func TestInstallOptOutLeavesRCAlone(t *testing.T) {
	home, out := rcAfterInstall(t, "/bin/zsh", "LANSTREAM_NO_MODIFY_PATH=1")
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal(".zshrc written despite LANSTREAM_NO_MODIFY_PATH")
	}
	if !strings.Contains(out, "Add it to your PATH") {
		t.Fatal(out)
	}
}

func TestInstallLeavesRCAloneWhenAlreadyOnPath(t *testing.T) {
	home, out := rcAfterInstall(t, "/bin/zsh", "PATH={dir}:/usr/bin:/bin")
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal(".zshrc written though the dir is already on PATH")
	}
	if strings.Contains(out, "PATH") {
		t.Fatalf("PATH advice printed though not needed:\n%s", out)
	}
}

func TestInstallInterruptedExitsCleanly(t *testing.T) {
	fake := t.TempDir() // a curl that interrupts the installer, as Ctrl-C would
	os.WriteFile(filepath.Join(fake, "curl"), []byte("#!/bin/sh\nkill -INT $PPID\nsleep 1\n"), 0o755)
	dir, out, err := runInstall(t, "sh", goodChecksums, "PATH="+fake+":/usr/bin:/bin")
	if err == nil || strings.Contains(out, "Checksum mismatch") || strings.Contains(out, "Download failed") {
		t.Fatalf("err=%v, misleading output:\n%s", err, out)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("install dir created by an interrupted run")
	}
}
