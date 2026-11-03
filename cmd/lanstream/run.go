package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Abinsanth/stream-video-lan/internal/library"
	"github.com/Abinsanth/stream-video-lan/internal/netinfo"
	"github.com/Abinsanth/stream-video-lan/internal/server"
	"github.com/Abinsanth/stream-video-lan/internal/version"
)

const usage = `lanstream: stream videos from a folder to VLC on your network

Usage:
  lanstream <folder>          share this folder
  lanstream --port <n>        use port n (default 8080)
  lanstream --version         print the version
  lanstream --help, -h        show this help
`

const rescanEvery = 5 * time.Second

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
	if len(folders) != 1 {
		io.WriteString(stderr, usage)
		return 2
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "port" })

	lib, err := library.New(expandHome(folders[0]))
	if err != nil {
		fmt.Fprintf(stderr, "lanstream: %v\n", err)
		return 1
	}
	ln, bound, err := netinfo.Listen(*port, explicit)
	if err != nil {
		fmt.Fprintf(stderr, "lanstream: %v\n", err)
		return 1
	}

	srv := newHTTPServer(server.New(lib).Handler())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	go lib.Run(ctx, rescanEvery)

	printStartup(stdout, lib, bound)

	select {
	case err := <-serveErr:
		fmt.Fprintf(stderr, "lanstream: %v\n", err)
		return 1
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		srv.Close() // viewers still streaming: cut them off
	}
	return 0
}

// newHTTPServer sets timeouts that suit streaming: idle keep-alive connections left behind by
// VLC's seeking are closed after a minute (macOS allows a process only 256 open files by
// default), while there is deliberately no read/write timeout, which would cut off long films.
func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
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

func printStartup(w io.Writer, lib *library.Library, port int) {
	fmt.Fprintf(w, "lanstream %s: sharing %s (%d videos)\n", version.Version, lib.Root(), len(lib.Videos()))
	base := func(ip net.IP) string { return "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)) }
	ips := netinfo.PrivateIPv4s()
	if len(ips) == 0 {
		fmt.Fprintf(w, "Playlist: %s  (no network found: connect to Wi-Fi or a hotspot)\n",
			server.PlaylistURL(base(net.IPv4(127, 0, 0, 1))))
	} else {
		fmt.Fprintf(w, "Playlist: %s\n", server.PlaylistURL(base(ips[0])))
		for _, ip := range ips[1:] {
			fmt.Fprintf(w, "  also:   %s\n", server.PlaylistURL(base(ip)))
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
