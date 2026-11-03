// Package share runs one shared folder: the library with its rescan loop, the listener, and the
// HTTP server. Both the TUI and the plain-text mode use it.
package share

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Abinsanth/stream-video-lan/internal/library"
	"github.com/Abinsanth/stream-video-lan/internal/netinfo"
	"github.com/Abinsanth/stream-video-lan/internal/server"
	"github.com/Abinsanth/stream-video-lan/internal/viewers"
)

const (
	rescanEvery  = 5 * time.Second
	shutdownWait = 3 * time.Second
)

// Options configure Start.
type Options struct {
	Port     int  // 0 lets the OS choose (used by tests)
	Explicit bool // the user chose Port: fail if busy instead of trying the next ones
	Tracker  *viewers.Tracker
}

// Share is one folder being served.
type Share struct {
	lib    *library.Library
	port   int
	bases  []string
	onLAN  bool
	srv    *http.Server
	cancel context.CancelFunc
	failed chan error
	once   sync.Once
}

// Start opens root, binds the port, and begins serving.
func Start(root string, opt Options) (*Share, error) {
	lib, err := library.New(root)
	if err != nil {
		return nil, err
	}
	ln, port, err := netinfo.Listen(opt.Port, opt.Explicit)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	ips := netinfo.PrivateIPv4s()
	s := &Share{
		lib:    lib,
		port:   port,
		bases:  bases(ips, port),
		onLAN:  len(ips) > 0,
		srv:    newHTTPServer(server.New(lib, opt.Tracker).Handler()),
		cancel: cancel,
		failed: make(chan error, 1),
	}
	go lib.Run(ctx, rescanEvery)
	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.failed <- err
		}
	}()
	return s, nil
}

// newHTTPServer sets timeouts that suit streaming: idle keep-alive connections left behind by
// VLC's seeking are closed after a minute, while there is deliberately no read/write timeout,
// which would cut off long films.
func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
}

// bases are the http://ip:port prefixes to advertise, primary first; loopback if not on a LAN.
func bases(ips []net.IP, port int) []string {
	if len(ips) == 0 {
		ips = []net.IP{net.IPv4(127, 0, 0, 1)}
	}
	out := make([]string, len(ips))
	for i, ip := range ips {
		out[i] = "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port))
	}
	return out
}

func (s *Share) Root() string               { return s.lib.Root() }
func (s *Share) Videos() []library.Video    { return s.lib.Videos() }
func (s *Share) ScanErr() error             { return s.lib.Err() }
func (s *Share) Port() int                  { return s.port }
func (s *Share) OnLAN() bool                { return s.onLAN }
func (s *Share) PlaylistURL() string        { return server.PlaylistURL(s.bases[0]) }
func (s *Share) VideoURL(rel string) string { return server.VideoURL(s.bases[0], rel) }
func (s *Share) Failed() <-chan error       { return s.failed }

// PlaylistURLs lists the playlist on every address, primary first.
func (s *Share) PlaylistURLs() []string {
	out := make([]string, len(s.bases))
	for i, b := range s.bases {
		out[i] = server.PlaylistURL(b)
	}
	return out
}

// Stop ends the rescan loop and the server, waiting up to 3 s for transfers before cutting
// them off. It is safe to call more than once.
func (s *Share) Stop() {
	s.once.Do(func() {
		s.cancel()
		ctx, cancel := context.WithTimeout(context.Background(), shutdownWait)
		defer cancel()
		if err := s.srv.Shutdown(ctx); err != nil {
			s.srv.Close()
		}
	})
}
