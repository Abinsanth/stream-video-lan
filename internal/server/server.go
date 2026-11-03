// Package server serves a library's videos over HTTP with range support, plus an M3U playlist.
package server

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/Abinsanth/stream-video-lan/internal/library"
	"github.com/Abinsanth/stream-video-lan/internal/viewers"
)

// Server serves one Library. It is read-only: there are no routes that change files.
type Server struct {
	lib *library.Library
	tr  *viewers.Tracker // may be nil
}

// New returns a Server for lib. If tr is non-nil, every video GET is recorded in it.
func New(lib *library.Library, tr *viewers.Tracker) *Server { return &Server{lib: lib, tr: tr} }

// Handler returns the HTTP routes. GET routes also answer HEAD; other methods get 405.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /playlist.m3u8", s.playlist)
	mux.HandleFunc("GET /v/{path...}", s.video)
	return mux
}

// baseURL is the address the client used to reach us, so a device on the hotspot gets
// hotspot links and one on Ethernet gets Ethernet links. Without a Host header (old HTTP/1.0
// clients) it falls back to the local address the connection arrived on.
func baseURL(r *http.Request) string {
	if r.Host != "" {
		return "http://" + r.Host
	}
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		return "http://" + a.String()
	}
	return "http://localhost"
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "lanstream is running. Open "+PlaylistURL(baseURL(r))+" in VLC.\n")
}

func (s *Server) playlist(w http.ResponseWriter, r *http.Request) {
	s.lib.Rescan()
	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	writePlaylist(w, baseURL(r), s.lib.Videos())
}

func (s *Server) video(w http.ResponseWriter, r *http.Request) {
	f, v, err := s.lib.Open(r.PathValue("path"))
	if err != nil {
		http.NotFound(w, r) // unknown, outside the folder, or deleted since the scan
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", library.ContentType(v.RelPath))
	if s.tr != nil && r.Method == http.MethodGet {
		x := s.tr.Begin(clientIP(r), v.RelPath, fi.Size(), rangeStart(r.Header.Get("Range"), fi.Size()))
		defer x.End()
		w = &countingWriter{ResponseWriter: w, x: x}
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rangeStart returns the first byte a Range header asks for: "bytes=N-…" → N, "bytes=-N" (the
// last N bytes) → size-N, and 0 for no header or one it can't read.
func rangeStart(h string, size int64) int64 {
	spec, ok := strings.CutPrefix(h, "bytes=")
	if !ok {
		return 0
	}
	first, _, _ := strings.Cut(spec, ",")
	from, to, ok := strings.Cut(strings.TrimSpace(first), "-")
	if !ok {
		return 0
	}
	if from == "" {
		n, err := strconv.ParseInt(to, 10, 64)
		if err != nil {
			return 0
		}
		return max(size-n, 0)
	}
	n, err := strconv.ParseInt(from, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// countingWriter reports body bytes to the viewer tracker.
type countingWriter struct {
	http.ResponseWriter
	x *viewers.Transfer
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.x.Wrote(int64(n))
	return n, err
}

func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
