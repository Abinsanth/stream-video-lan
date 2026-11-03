package netinfo

import (
	"net"
	"testing"
)

func busyPort(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func TestListenFallsBack(t *testing.T) {
	_, p := busyPort(t)
	ln, got, err := Listen(p, false)
	if err != nil || got == p {
		t.Fatalf("got %d err %v", got, err)
	}
	defer ln.Close()
	if ln.Addr().(*net.TCPAddr).Port != got {
		t.Fatal("returned port differs from bound port")
	}
}

func TestListenExplicitBusyFails(t *testing.T) {
	_, p := busyPort(t)
	if _, _, err := Listen(p, true); err == nil {
		t.Fatal("want error")
	}
}

func TestListenPortZeroReportsBoundPort(t *testing.T) {
	ln, got, err := Listen(0, true)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if got == 0 || got != ln.Addr().(*net.TCPAddr).Port {
		t.Fatalf("got %d", got)
	}
}

func TestFallbackRange(t *testing.T) {
	cases := []struct{ port, first, last int }{
		{8080, 8080, 8099},
		{8095, 8095, 8099},
		{9000, 9000, 9019},
	}
	for _, c := range cases {
		if f, l := fallbackRange(c.port); f != c.first || l != c.last {
			t.Errorf("fallbackRange(%d) = %d..%d, want %d..%d", c.port, f, l, c.first, c.last)
		}
	}
}

func TestIsPrivate(t *testing.T) {
	for _, s := range []string{"10.0.0.1", "172.16.5.4", "172.31.255.1", "192.168.1.5"} {
		if !isPrivate(net.ParseIP(s)) {
			t.Error(s)
		}
	}
	for _, s := range []string{"8.8.8.8", "172.32.0.1", "127.0.0.1", "169.254.1.1", "::1", "fd00::1"} {
		if isPrivate(net.ParseIP(s)) {
			t.Error(s)
		}
	}
}

func TestPrivateIPv4sAreAllPrivate(t *testing.T) {
	for _, ip := range PrivateIPv4s() {
		if !isPrivate(ip) {
			t.Errorf("%s is not private", ip)
		}
	}
}

func TestPrimaryFirst(t *testing.T) {
	a, b, c := net.ParseIP("10.0.0.2"), net.ParseIP("192.168.1.5"), net.ParseIP("172.16.0.9")
	got := primaryFirst([]net.IP{a, b, c}, b)
	if !got[0].Equal(b) || len(got) != 3 {
		t.Fatal(got)
	}
	if got := primaryFirst([]net.IP{a, c}, b); !got[0].Equal(a) {
		t.Fatal("primary not in list should keep order", got)
	}
}
