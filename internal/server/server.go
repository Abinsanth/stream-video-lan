// Package server serves a library's videos over HTTP with range support, plus an M3U playlist.
package server

import (
	"io"
	"net/http"

	"github.com/Abinsanth/stream-video-lan/internal/library"
)

// Server serves one Library. It is read-only: there are no routes that change files.
type Server struct {
	lib *library.Library
}

// New returns a Server for lib.
func New(lib *library.Library) *Server { return &Server{lib: lib} }

// Handler returns the HTTP routes. GET routes also answer HEAD; other methods get 405.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /playlist.m3u8", s.playlist)
	mux.HandleFunc("GET /v/{path...}", s.video)
	return mux
}

// baseURL is the address the client used to reach us, so a device on the hotspot gets
// hotspot links and one on Ethernet gets Ethernet links.
func baseURL(r *http.Request) string { return "http://" + r.Host }

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
	http.ServeContent(w, r, "", fi.ModTime(), f)
}
