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
	if code := run(context.Background(), []string{"--version"}, strings.NewReader(""), &out, &errb); code != 0 || out.String() != "lanstream dev\n" {
		t.Fatal(code, out.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"-h"}, strings.NewReader(""), &out, &errb); code != 0 || !strings.Contains(out.String(), "lanstream <folder>") {
		t.Fatal(code, out.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"--help"}, strings.NewReader(""), &out, &errb); code != 0 || !strings.HasPrefix(out.String(), "lanstream: stream videos") {
		t.Fatal(code, out.String())
	}
}

func TestRunNeedsFolder(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, strings.NewReader(""), &out, &errb); code != 2 || !strings.Contains(errb.String(), "Usage:") {
		t.Fatal(code, errb.String())
	}
	errb.Reset()
	if code := run(context.Background(), []string{"a", "b"}, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatal(code)
	}
	errb.Reset()
	if code := run(context.Background(), []string{"/definitely/not/here"}, strings.NewReader(""), &out, &errb); code != 1 || !strings.HasPrefix(errb.String(), "lanstream: ") {
		t.Fatal(code, errb.String())
	}
	errb.Reset()
	if code := run(context.Background(), []string{"--nope"}, strings.NewReader(""), &out, &errb); code != 2 {
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
	go func() { done <- run(ctx, []string{root, "--port", "0"}, strings.NewReader(""), &out, io.Discard) }()
	url := waitForLine(t, &out, "Playlist: ")
	if b := httpGet(t, url); !strings.Contains(b, "a.mp4") {
		t.Fatal(b)
	}
	if !strings.Contains(out.String(), "(1 video)") {
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

func TestRunInvalidPort(t *testing.T) {
	var out, errb bytes.Buffer
	for _, p := range []string{"70000", "-1"} {
		errb.Reset()
		if code := run(context.Background(), []string{"--port", p, t.TempDir()}, strings.NewReader(""), &out, &errb); code != 2 || !strings.Contains(errb.String(), "invalid port") {
			t.Fatal(p, code, errb.String())
		}
	}
}

func TestUsageListsPicker(t *testing.T) {
	var out bytes.Buffer
	run(context.Background(), []string{"-h"}, strings.NewReader(""), &out, io.Discard)
	if !strings.Contains(out.String(), "lanstream                   pick a folder to share") {
		t.Fatal(out.String())
	}
}

func TestIsTerminalFalseForBuffersAndFiles(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(&bytes.Buffer{}) || isTerminal(f) {
		t.Fatal("non-terminal reported as terminal")
	}
}
