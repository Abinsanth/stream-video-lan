package server

import (
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/Abinsanth/stream-video-lan/internal/library"
)

// PlaylistURL is the address VLC opens to see every video.
func PlaylistURL(base string) string { return base + "/playlist.m3u8" }

// VideoURL builds the URL of one video, escaping each path segment so names with spaces,
// '#', '?', '%' or non-ASCII characters survive the trip through the player.
func VideoURL(base, relPath string) string {
	segs := strings.Split(relPath, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return base + "/v/" + strings.Join(segs, "/")
}

// titleCleaner keeps a file name from breaking the one-entry-per-line M3U format.
var titleCleaner = strings.NewReplacer("\r", " ", "\n", " ")

// writePlaylist writes an extended M3U playlist (UTF-8) listing vids in order.
func writePlaylist(w io.Writer, base string, vids []library.Video) error {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, v := range vids {
		title := strings.TrimSuffix(v.RelPath, path.Ext(v.RelPath))
		b.WriteString("#EXTINF:-1," + titleCleaner.Replace(title) + "\n")
		b.WriteString(VideoURL(base, v.RelPath) + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
