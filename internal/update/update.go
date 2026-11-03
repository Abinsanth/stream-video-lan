// Package update finds, downloads, verifies and installs new lanstream releases from GitHub.
// It is the only code that talks to the internet.
package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Repo        = "Abinsanth/stream-video-lan"
	InstallLine = "curl -fsSL https://raw.githubusercontent.com/Abinsanth/stream-video-lan/release/install.sh | sh"

	maxArchive = 200 << 20 // a lanstream archive is ~5 MB; anything near this is not one
)

// Client talks to GitHub. The URLs are fields so tests can point it at a fake server.
type Client struct {
	Web  string // https://github.com
	API  string // https://api.github.com
	HTTP *http.Client
}

// Default is the real GitHub with a two-minute overall timeout per request.
func Default() Client {
	return Client{Web: "https://github.com", API: "https://api.github.com", HTTP: &http.Client{Timeout: 2 * time.Minute}}
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Latest returns the newest released version without the "v" (e.g. "0.2.0"). It reads where
// github.com/<repo>/releases/latest redirects to, which needs no API call and so no rate limit.
func (c Client) Latest(ctx context.Context) (string, error) {
	hc := *c.httpClient()
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Web+"/"+Repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	res, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	res.Body.Close()
	_, tag, ok := strings.Cut(res.Header.Get("Location"), "/releases/tag/v")
	if res.StatusCode < 300 || res.StatusCode >= 400 || !ok || tag == "" {
		return "", errors.New("no release found")
	}
	return tag, nil
}

// Notes returns the release notes for v<version> ("" when there are none).
func (c Client) Notes(ctx context.Context, version string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.API+"/repos/"+Repo+"/releases/tags/v"+version, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release notes: HTTP %d", res.StatusCode)
	}
	var rel struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&rel); err != nil {
		return "", err
	}
	return strings.TrimSpace(rel.Body), nil
}

// Newer reports whether latest is a higher x.y.z version than current. A "dev" build, or any
// version that isn't x.y.z, is never considered older.
func Newer(latest, current string) bool {
	l, okL := parse(latest)
	c, okC := parse(current)
	if !okL || !okC {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Download fetches the archive for goos/goarch of v<version>, checks its SHA-256 against
// checksums.txt, and returns the lanstream binary inside it.
func (c Client) Download(ctx context.Context, version, goos, goarch string) ([]byte, error) {
	asset := "lanstream_" + goos + "_" + goarch + ".tar.gz"
	base := c.Web + "/" + Repo + "/releases/download/v" + version + "/"
	sums, err := c.get(ctx, base+"checksums.txt", "checksums.txt")
	if err != nil {
		return nil, err
	}
	want, ok := checksumFor(sums, asset)
	if !ok {
		return nil, fmt.Errorf("%s not listed in checksums.txt", asset)
	}
	arc, err := c.get(ctx, base+asset, asset)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(arc)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s", asset)
	}
	return extract(arc, asset)
}

func (c Client) get(ctx context.Context, url, name string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", name, res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxArchive+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	if len(b) > maxArchive {
		return nil, fmt.Errorf("download %s: too large", name)
	}
	return b, nil
}

// checksumFor finds "<hex>  <asset>" in a checksums.txt.
func checksumFor(sums []byte, asset string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return strings.ToLower(f[0]), true
		}
	}
	return "", false
}

func extract(arc []byte, asset string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(arc))
	if err != nil {
		return nil, fmt.Errorf("%s is not a valid archive: %w", asset, err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("no lanstream binary in %s", asset)
		}
		if err != nil {
			return nil, fmt.Errorf("%s is not a valid archive: %w", asset, err)
		}
		if path.Base(h.Name) == "lanstream" && h.Typeflag == tar.TypeReg {
			b, err := io.ReadAll(io.LimitReader(tr, maxArchive))
			if err != nil {
				return nil, fmt.Errorf("%s is not a valid archive: %w", asset, err)
			}
			return b, nil
		}
	}
}

// Replace swaps the file at p (following symlinks to the real file) for bin. It writes a temp
// file next to it and renames it into place, so lanstream processes already running keep their
// old copy, and macOS never sees a signed binary modified in place.
func Replace(p string, bin []byte) error {
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(real), ".lanstream-new-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	if _, err := f.Write(bin); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o755); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, real)
}
