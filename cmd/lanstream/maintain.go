package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Abinsanth/stream-video-lan/internal/config"
	"github.com/Abinsanth/stream-video-lan/internal/update"
	"github.com/Abinsanth/stream-video-lan/internal/version"
)

// Swappable in tests.
var (
	osExecutable = os.Executable
	updater      = update.Default()
	configDir    = config.Dir
)

// executablePath is the real file of the running lanstream (symlinks resolved).
func executablePath() (string, error) {
	p, err := osExecutable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// ask prints question and reads a yes/no answer. Only "y" or "yes" count as yes; an empty
// line, anything else, or a closed input means no.
func ask(stdin io.Reader, stdout io.Writer, question string) bool {
	fmt.Fprint(stdout, question+" (y/n) ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(stdout)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func runUpdate(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) int {
	cur := version.Version
	if cur == "dev" {
		fmt.Fprintf(stdout, "This is a development build. Install a release with:\n  %s\n", update.InstallLine)
		return 1
	}
	latest, err := updater.Latest(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "lanstream: can't check for updates: %v\n", err)
		return 1
	}
	if !update.Newer(latest, cur) {
		fmt.Fprintf(stdout, "lanstream v%s is up to date.\n", cur)
		return 0
	}
	if !ask(stdin, stdout, fmt.Sprintf("Update v%s → v%s?", cur, latest)) {
		fmt.Fprintln(stdout, "No changes made.")
		return 0
	}
	exe, err := executablePath()
	if err != nil {
		fmt.Fprintf(stderr, "lanstream: can't find the installed program: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Downloading v%s…\n", latest)
	bin, err := updater.Download(ctx, latest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(stderr, "lanstream: %v\nNo changes made.\n", err)
		return 1
	}
	if err := update.Replace(exe, bin); err != nil {
		fmt.Fprintf(stderr, "lanstream: can't write %s: %v\nReinstall with:\n  %s\n", exe, rootErr(err), update.InstallLine)
		return 1
	}
	fmt.Fprintf(stdout, "Updated to v%s. Running lanstream windows keep the old version until restarted.\n", latest)
	return 0
}

func runUninstall(stdin io.Reader, stdout, stderr io.Writer) int {
	var paths []string
	binary := ""
	if version.Version != "dev" { // a dev build is a `go build` output: leave it alone
		if p, err := executablePath(); err == nil {
			binary = p
			paths = append(paths, p)
		}
	}
	cfg, cfgErr := configDir()
	if cfgErr == nil {
		paths = append(paths, cfg)
	}
	fmt.Fprintln(stdout, "This removes:")
	for _, p := range paths {
		fmt.Fprintln(stdout, "  "+p)
	}
	if !ask(stdin, stdout, "Remove lanstream and its settings?") {
		fmt.Fprintln(stdout, "No changes made.")
		return 0
	}
	var failed []string
	if cfgErr == nil {
		if err := os.RemoveAll(cfg); err != nil {
			fmt.Fprintf(stderr, "lanstream: can't remove %s: %v\n", cfg, rootErr(err))
			failed = append(failed, "rm -rf "+cfg)
		}
	}
	if binary != "" {
		if err := os.Remove(binary); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "lanstream: can't remove %s: %v\n", binary, rootErr(err))
			failed = append([]string{"rm " + binary}, failed...)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(stderr, "Remove it by hand:\n  %s\n", strings.Join(failed, "\n  "))
		return 1
	}
	fmt.Fprintln(stdout, "lanstream removed.")
	return 0
}

// rootErr drops Go's "op path:" prefix when the path is already in the message.
func rootErr(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err
	}
	return err
}
