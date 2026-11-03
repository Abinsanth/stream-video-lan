package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Abinsanth/stream-video-lan/internal/library"
)

type testServer struct {
	*httptest.Server
	root string
}

// newTestServer writes files (relPath → contents) under a fresh shared folder and serves it.
// The folder sits inside its own temp dir, so tests can put "outside" files next to it.
func newTestServer(t *testing.T, files map[string]string) *testServer {
	t.Helper()
	root := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := library.New(root)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(lib).Handler())
	t.Cleanup(ts.Close)
	return &testServer{ts, root}
}

func rootOf(ts *testServer) string { return ts.root }

type result struct {
	code   int
	header http.Header
	body   string
}

// do performs a request; header is optional key/value pairs. Safe to call from goroutines:
// failures are reported with t.Errorf and a zero result.
func do(t *testing.T, method, url string, header ...string) result {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Errorf("%s %s: %v", method, url, err)
		return result{}
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Errorf("%s %s: %v", method, url, err)
		return result{}
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return result{res.StatusCode, res.Header, string(b)}
}

func get(t *testing.T, url string, header ...string) result { return do(t, "GET", url, header...) }

func playlistURLs(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

func TestRangeRequests(t *testing.T) {
	ts := newTestServer(t, map[string]string{"m.mp4": "0123456789"})
	cases := []struct {
		rng, body, cr string
		code          int
	}{
		{"bytes=0-3", "0123", "bytes 0-3/10", 206},
		{"bytes=7-", "789", "bytes 7-9/10", 206},
		{"bytes=-2", "89", "bytes 8-9/10", 206},
		{"bytes=20-30", "", "bytes */10", 416},
	}
	for _, c := range cases {
		res := get(t, ts.URL+"/v/m.mp4", "Range", c.rng)
		if res.code != c.code || res.header.Get("Content-Range") != c.cr || (c.code == 206 && res.body != c.body) {
			t.Errorf("%s: got %d %q %q", c.rng, res.code, res.header.Get("Content-Range"), res.body)
		}
	}
	full := get(t, ts.URL+"/v/m.mp4")
	if full.code != 200 || full.header.Get("Accept-Ranges") != "bytes" || full.header.Get("Content-Type") != "video/mp4" || full.body != "0123456789" {
		t.Fatal(full.code, full.header, full.body)
	}
}

func TestHeadAndMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t, map[string]string{"m.mkv": "abc"})
	if r := do(t, "HEAD", ts.URL+"/v/m.mkv"); r.code != 200 || r.header.Get("Content-Length") != "3" || r.body != "" || r.header.Get("Content-Type") != "video/x-matroska" {
		t.Fatal(r)
	}
	if r := do(t, "POST", ts.URL+"/v/m.mkv"); r.code != 405 {
		t.Fatal(r.code)
	}
}

func TestTraversalNeverLeaks(t *testing.T) {
	ts := newTestServer(t, map[string]string{"a.mp4": "ok"})
	secret := filepath.Join(filepath.Dir(rootOf(ts)), "secret.mp4")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/v/../secret.mp4", "/v/%2e%2e/secret.mp4", "/v/%252e%252e/secret.mp4",
		"/v/..%2fsecret.mp4", "/v//etc/passwd", "/v/", "/v/%2Fetc%2Fpasswd"} {
		r := get(t, ts.URL+p) // the client follows redirects
		if r.code == 200 || strings.Contains(r.body, "SECRET") {
			t.Errorf("%s: %d %q", p, r.code, r.body)
		}
	}
}

func TestPlaylistURLsRoundTrip(t *testing.T) {
	names := []string{"ep 1.mkv", "50% off.mp4", "a#b?.mp4", "മലയാളം.mp4", "Show/S01E2.mkv", "Show/S01E10.mkv"}
	files := map[string]string{}
	for _, n := range names {
		files[n] = n
	}
	ts := newTestServer(t, files)
	pl := get(t, ts.URL+"/playlist.m3u8")
	if pl.code != 200 || pl.header.Get("Content-Type") != "audio/x-mpegurl; charset=utf-8" || !strings.HasPrefix(pl.body, "#EXTM3U\n") {
		t.Fatal(pl.code, pl.header, pl.body)
	}
	urls := playlistURLs(pl.body)
	if len(urls) != len(names) {
		t.Fatal(urls)
	}
	for _, u := range urls {
		if !strings.HasPrefix(u, ts.URL+"/v/") {
			t.Errorf("URL %q does not use the request's host", u)
		}
		r := get(t, u)
		if r.code != 200 || files[r.body] == "" {
			t.Errorf("%s → %d %q", u, r.code, r.body)
		}
	}
	if !strings.Contains(pl.body, "#EXTINF:-1,Show/S01E2\n") {
		t.Error("title missing ext-stripped path")
	}
	if strings.Index(pl.body, "S01E2.mkv") > strings.Index(pl.body, "S01E10.mkv") {
		t.Error("not natural order")
	}
}

func TestPlaylistTitleNewlineCannotBreakFormat(t *testing.T) {
	ts := newTestServer(t, map[string]string{"bad\nname.mp4": "x"})
	pl := get(t, ts.URL+"/playlist.m3u8")
	urls := playlistURLs(pl.body)
	if len(urls) != 1 || !strings.HasPrefix(urls[0], ts.URL+"/v/") {
		t.Fatalf("playlist broken by newline in name: %q", pl.body)
	}
	if r := get(t, urls[0]); r.code != 200 {
		t.Fatal(r.code)
	}
}

func TestPlaylistRescansNewFiles(t *testing.T) {
	ts := newTestServer(t, map[string]string{})
	if err := os.WriteFile(filepath.Join(rootOf(ts), "late.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, ts.URL+"/playlist.m3u8").body, "late.mp4") {
		t.Fatal("new file not in playlist")
	}
}

func TestVideoDeletedAfterScan(t *testing.T) {
	ts := newTestServer(t, map[string]string{"gone.mp4": "x"})
	if err := os.Remove(filepath.Join(rootOf(ts), "gone.mp4")); err != nil {
		t.Fatal(err)
	}
	if r := get(t, ts.URL+"/v/gone.mp4"); r.code != 404 {
		t.Fatal(r.code)
	}
}

func TestConcurrentViewers(t *testing.T) {
	data := strings.Repeat("abcdefghij", 100_000) // 1 MB
	ts := newTestServer(t, map[string]string{"big.mp4": data})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := i * 50_000
			r := get(t, ts.URL+"/v/big.mp4", "Range", fmt.Sprintf("bytes=%d-%d", start, start+9_999))
			if r.code != 206 || r.body != data[start:start+10_000] {
				t.Errorf("viewer %d got wrong bytes (code %d)", i, r.code)
			}
		}(i)
	}
	wg.Wait()
}

func TestIndex(t *testing.T) {
	ts := newTestServer(t, nil)
	r := get(t, ts.URL+"/")
	if r.code != 200 || r.body != "lanstream is running. Open "+ts.URL+"/playlist.m3u8 in VLC.\n" {
		t.Fatal(r)
	}
	if get(t, ts.URL+"/nope").code != 404 {
		t.Fatal("unknown path not 404")
	}
}

func TestVideoURLEscapesEachSegment(t *testing.T) {
	got := VideoURL("http://h:1", "My Show/ep #1?.mkv")
	if got != "http://h:1/v/My%20Show/ep%20%231%3F.mkv" {
		t.Fatal(got)
	}
	if PlaylistURL("http://h:1") != "http://h:1/playlist.m3u8" {
		t.Fatal(PlaylistURL("http://h:1"))
	}
}

// A process that can write into the shared folder flips a scanned file between a regular file
// and a symlink to an outside secret while requests run. No response may ever carry the secret.
func TestSymlinkSwapRaceNeverLeaks(t *testing.T) {
	ts := newTestServer(t, map[string]string{"a.mp4": "public"})
	secret := filepath.Join(filepath.Dir(rootOf(ts)), "secret.mp4")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(rootOf(ts), "a.mp4")
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tmpLink, tmpFile := target+".l", target+".f"
		for {
			select {
			case <-stop:
				return
			default:
			}
			os.Symlink(secret, tmpLink)
			os.Rename(tmpLink, target)
			os.WriteFile(tmpFile, []byte("public"), 0o644)
			os.Rename(tmpFile, target)
		}
	}()
	leaks := 0
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if strings.Contains(get(t, ts.URL+"/v/a.mp4").body, "SECRET") {
			leaks++
		}
	}
	close(stop)
	wg.Wait()
	if leaks > 0 {
		t.Fatalf("served the outside file %d times", leaks)
	}
}

// A file swapped for a FIFO after the scan must not hang the request.
func TestFIFOSwappedInDoesNotHang(t *testing.T) {
	ts := newTestServer(t, map[string]string{"a.mp4": "x"})
	p := filepath.Join(rootOf(ts), "a.mp4")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Get(ts.URL + "/v/a.mp4")
	if err != nil {
		t.Fatalf("request hung or failed: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatal(res.StatusCode)
	}
}
