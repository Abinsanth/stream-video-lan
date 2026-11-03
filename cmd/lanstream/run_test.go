package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// waitForLine waits up to 5 s for a line starting with prefix and returns the first field after it.
func waitForLine(t *testing.T, buf *syncBuffer, prefix string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range strings.Split(buf.String(), "\n") {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				return strings.Fields(rest)[0]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %q line in output:\n%s", prefix, buf.String())
	return ""
}

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

func TestRunVersionAndHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, &out, &errb); code != 0 || out.String() != "lanstream dev\n" {
		t.Fatal(code, out.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"-h"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "lanstream <folder>") {
		t.Fatal(code, out.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"--help"}, &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "lanstream: stream videos") {
		t.Fatal(code, out.String())
	}
}

func TestRunNeedsFolder(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, &out, &errb); code != 2 || !strings.Contains(errb.String(), "Usage:") {
		t.Fatal(code, errb.String())
	}
	errb.Reset()
	if code := run(context.Background(), []string{"a", "b"}, &out, &errb); code != 2 {
		t.Fatal(code)
	}
	errb.Reset()
	if code := run(context.Background(), []string{"/definitely/not/here"}, &out, &errb); code != 1 || !strings.HasPrefix(errb.String(), "lanstream: ") {
		t.Fatal(code, errb.String())
	}
	errb.Reset()
	if code := run(context.Background(), []string{"--nope"}, &out, &errb); code != 2 {
		t.Fatal(code)
	}
}

func TestRunServesAndStops(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var out syncBuffer
	done := make(chan int)
	// Folder before the flag: flags after the folder must still be honoured.
	go func() { done <- run(ctx, []string{root, "--port", "0"}, &out, io.Discard) }()
	url := waitForLine(t, &out, "Playlist: ")
	if b := httpGet(t, url); !strings.Contains(b, "a.mp4") {
		t.Fatal(b)
	}
	if !strings.Contains(out.String(), "(1 videos)") {
		t.Fatalf("startup line missing count:\n%s", out.String())
	}
	cancel()
	select {
	case c := <-done:
		if c != 0 {
			t.Fatal(c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cases := map[string]string{"~": home, "~/Movies": filepath.Join(home, "Movies"), "/x/~y": "/x/~y", "rel": "rel"}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
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
