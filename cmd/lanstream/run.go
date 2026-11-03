package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
	"golang.org/x/term"

	"github.com/Abinsanth/stream-video-lan/internal/awake"
	"github.com/Abinsanth/stream-video-lan/internal/config"
	"github.com/Abinsanth/stream-video-lan/internal/netinfo"
	"github.com/Abinsanth/stream-video-lan/internal/share"
	"github.com/Abinsanth/stream-video-lan/internal/tui"
	"github.com/Abinsanth/stream-video-lan/internal/version"
	"github.com/Abinsanth/stream-video-lan/internal/viewers"
)

const usage = `lanstream: stream videos from a folder to VLC on your network

Usage:
  lanstream                   pick a folder to share
  lanstream <folder>          share this folder
  lanstream --port <n>        use port n (default 8080)
  lanstream --version         print the version
  lanstream --help, -h        show this help
`

// run is the whole program; main only wires it to the OS. It returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lanstream", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	port := fs.Int("port", netinfo.DefaultPort, "")
	showVersion := fs.Bool("version", false, "")

	folders, err := parseInterleaved(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		io.WriteString(stdout, usage)
		return 0
	}
	if err != nil {
		io.WriteString(stderr, usage)
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "lanstream %s\n", version.Version)
		return 0
	}
	if *port < 0 || *port > 65535 {
		fmt.Fprintf(stderr, "lanstream: invalid port %d (use 0-65535)\n", *port)
		return 2
	}
	if len(folders) > 1 {
		io.WriteString(stderr, usage)
		return 2
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "port" })

	if isTerminal(stdout) {
		folder := ""
		if len(folders) == 1 {
			folder = expandHome(folders[0])
		}
		return runTUI(ctx, folder, share.Options{Port: *port, Explicit: explicit}, stderr)
	}
	if len(folders) == 0 { // the picker needs a terminal
		io.WriteString(stderr, usage)
		return 2
	}

	s, err := share.Start(expandHome(folders[0]), share.Options{Port: *port, Explicit: explicit})
	if err != nil {
		fmt.Fprintf(stderr, "lanstream: %v\n", err)
		return 1
	}
	defer s.Stop()
	printStartup(stdout, s)

	select {
	case err := <-s.Failed():
		fmt.Fprintf(stderr, "lanstream: %v\n", err)
		return 1
	case <-ctx.Done():
		return 0
	}
}

// isTerminal reports whether w is an interactive terminal (so the full-screen UI can run).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// runTUI runs the full-screen interface, starting on folder if one was given.
func runTUI(ctx context.Context, folder string, opt share.Options, stderr io.Writer) int {
	tracker := viewers.New(nil)
	keeper := awake.NewKeeper(awake.System(), nil)
	defer keeper.Close()
	opt.Tracker = tracker
	cfgPath, cfgErr := config.Path()
	home, _ := os.UserHomeDir()
	deps := tui.Deps{
		Start: func(root string) (tui.Share, error) {
			s, err := share.Start(root, opt)
			if err != nil {
				return nil, err // a bare nil, not a nil *share.Share inside the interface
			}
			return s, nil
		},
		Config: config.Load(cfgPath),
		SaveConfig: func(c config.Config) error {
			if cfgErr != nil {
				return cfgErr
			}
			return config.Save(cfgPath, c)
		},
		CopyText:      clipboard.WriteAll,
		Tracker:       tracker,
		Keeper:        keeper,
		Home:          home,
		GOOS:          runtime.GOOS,
		InitialFolder: folder,
		Version:       version.Version,
	}
	if err := tui.Run(ctx, deps); err != nil {
		fmt.Fprintf(stderr, "lanstream: %v\n", err)
		return 1
	}
	return 0
}

// parseInterleaved parses flags anywhere in args (Go's flag package stops at the first
// positional), so "lanstream ~/Movies --port 9000" works. It returns the positionals.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func printStartup(w io.Writer, s *share.Share) {
	n := len(s.Videos())
	noun := "videos"
	if n == 1 {
		noun = "video"
	}
	fmt.Fprintf(w, "lanstream %s: sharing %s (%d %s)\n", version.Version, s.Root(), n, noun)
	urls := s.PlaylistURLs()
	if !s.OnLAN() {
		fmt.Fprintf(w, "Playlist: %s  (no network found: connect to Wi-Fi or a hotspot)\n", urls[0])
	} else {
		fmt.Fprintf(w, "Playlist: %s\n", urls[0])
		for _, u := range urls[1:] {
			fmt.Fprintf(w, "  also:   %s\n", u)
		}
	}
	fmt.Fprintln(w, "Open the playlist in VLC (Open Network Stream). Press Ctrl-C to stop.")
}

// expandHome turns a leading "~" or "~/" into the user's home folder.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
