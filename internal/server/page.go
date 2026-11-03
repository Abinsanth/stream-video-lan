package server

import (
	"html/template"
	"io"
	"strings"
)

// The launcher page a phone opens after scanning lanstream's QR code. A camera can only hand a
// link to the browser; these buttons hand the playlist on to VLC. It is a launcher, not a player.
var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>lanstream</title>
<style>
  :root { color-scheme: light dark; --accent: #e8590c; }
  body { font: 17px/1.45 -apple-system, system-ui, sans-serif; margin: 0; padding: 24px 16px; max-width: 30rem; margin-inline: auto; }
  h1 { font-size: 1.4rem; margin: 0 0 .25rem; }
  p { margin: .25rem 0 1.25rem; opacity: .8; }
  a.btn, button { display: block; box-sizing: border-box; width: 100%; padding: 16px; margin: 0 0 12px; border-radius: 12px;
    font: 600 1.05rem/1.2 inherit; text-align: center; text-decoration: none; border: 2px solid var(--accent); cursor: pointer; }
  a.primary { background: var(--accent); color: #fff; }
  a.secondary, button { background: transparent; color: inherit; }
  input { width: 100%; box-sizing: border-box; padding: 12px; font: 15px ui-monospace, monospace; border-radius: 8px;
    border: 1px solid #8884; background: transparent; color: inherit; }
  small { display: block; margin-top: 1.25rem; opacity: .7; }
</style>
</head>
<body>
<h1>lanstream</h1>
<p>Tap to watch in VLC. Your phone must be on the same Wi-Fi or hotspot.</p>
<a class="btn primary" href="{{.Android}}">Open in VLC (Android)</a>
<a class="btn secondary" href="{{.IPhone}}">Open in VLC (iPhone)</a>
<button type="button" onclick="var i=document.getElementById('link');i.select();try{document.execCommand('copy');this.textContent='Copied'}catch(e){}">Copy link</button>
<input id="link" readonly value="{{.Playlist}}" onfocus="this.select()">
<small>No VLC yet? Install "VLC for Android" or "VLC for Mobile" (iPhone) from your app store, then come back and tap again. You can also paste the link into VLC: More → New stream.</small>
</body>
</html>
`))

type pageData struct {
	Android, IPhone template.URL // custom schemes html/template would otherwise blank out
	Playlist        string
}

// writePage renders the launcher for the base URL the phone used (e.g. http://192.168.1.5:8080).
func writePage(w io.Writer, base string) error {
	playlist := PlaylistURL(base)
	hostPath := strings.TrimPrefix(playlist, "http://")
	return pageTmpl.Execute(w, pageData{
		// Android: an intent link names VLC's package, so the browser opens VLC itself (or the
		// Play Store if VLC isn't installed) with the playlist.
		Android: template.URL("intent://" + hostPathSafe(hostPath) + "#Intent;scheme=http;package=org.videolan.vlc;type=audio/x-mpegurl;end"),
		// iPhone: VLC registers vlc:// and opens the link that follows.
		IPhone:   template.URL("vlc://" + hostPathSafe(playlist)),
		Playlist: playlist,
	})
}

// hostPathSafe returns s unchanged when it only holds characters a host:port/path can contain,
// and "" otherwise, so nothing from a forged Host header can escape the href attribute.
func hostPathSafe(s string) string {
	for _, r := range s {
		ok := r == '.' || r == ':' || r == '/' || r == '-' || r == '[' || r == ']' ||
			('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
		if !ok {
			return ""
		}
	}
	return s
}
