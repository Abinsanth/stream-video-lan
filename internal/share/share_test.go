package share

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func httpGet(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func TestStartServesAndStops(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Start(root, Options{Port: 0, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if body := httpGet(t, s.PlaylistURL()); !strings.Contains(body, "a.mp4") {
		t.Fatal(body)
	}
	if !strings.HasSuffix(s.VideoURL("a.mp4"), "/v/a.mp4") || s.Root() != root || len(s.Videos()) != 1 || s.Port() == 0 {
		t.Fatal(s.VideoURL("a.mp4"), s.Root(), s.Videos(), s.Port())
	}
	s.Stop()
	s.Stop()
	if _, err := http.Get(s.PlaylistURL()); err == nil {
		t.Fatal("still serving after Stop")
	}
}

func TestStartBadFolder(t *testing.T) {
	if _, err := Start("/definitely/not/here", Options{Port: 0, Explicit: true}); err == nil {
		t.Fatal("want error")
	}
}

func TestStartBusyExplicitPortFails(t *testing.T) {
	busy, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if _, err := Start(t.TempDir(), Options{Port: busy.Addr().(*net.TCPAddr).Port, Explicit: true}); err == nil {
		t.Fatal("want busy-port error")
	}
}

func TestBases(t *testing.T) {
	if got := bases(nil, 8080); !slices.Equal(got, []string{"http://127.0.0.1:8080"}) {
		t.Fatal(got)
	}
	ips := []net.IP{net.IPv4(192, 168, 1, 5), net.IPv4(10, 0, 0, 2)}
	if got := bases(ips, 9000); !slices.Equal(got, []string{"http://192.168.1.5:9000", "http://10.0.0.2:9000"}) {
		t.Fatal(got)
	}
}

func TestHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer(http.NotFoundHandler())
	if srv.IdleTimeout != 60*time.Second || srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("idle %v header %v", srv.IdleTimeout, srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != 0 || srv.ReadTimeout != 0 {
		t.Fatal("read/write timeouts would cut off long streams")
	}
}
