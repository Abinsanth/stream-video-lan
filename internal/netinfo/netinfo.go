// Package netinfo finds this machine's LAN addresses and opens the listening socket.
package netinfo

import (
	"fmt"
	"net"
	"strconv"
)

const (
	DefaultPort     = 8080
	MaxFallbackPort = 8099
	fallbackTries   = 20
)

// Listen binds 0.0.0.0:port (IPv4). If explicit is false and the port is busy, it tries the
// following ports (up to 8099 when starting at or below it, otherwise 20 ports in all).
// It returns the listener and the port actually bound.
func Listen(port int, explicit bool) (net.Listener, int, error) {
	first, last := port, port
	if !explicit {
		first, last = fallbackRange(port)
	}
	var err error
	for p := first; p <= last; p++ {
		var ln net.Listener
		if ln, err = net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(p))); err == nil {
			return ln, ln.Addr().(*net.TCPAddr).Port, nil
		}
	}
	if first == last {
		return nil, 0, fmt.Errorf("port %d is busy: %w", first, err)
	}
	return nil, 0, fmt.Errorf("ports %d-%d are all busy: %w", first, last, err)
}

func fallbackRange(port int) (first, last int) {
	if port <= MaxFallbackPort {
		return port, MaxFallbackPort
	}
	return port, port + fallbackTries - 1
}

// isPrivate reports whether ip is an IPv4 address in 10/8, 172.16/12 or 192.168/16.
func isPrivate(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4.IsPrivate()
}

// PrivateIPv4s returns the private IPv4 addresses of interfaces that are up, primary first.
// The primary is the one the default route uses; it returns nil when there are none.
func PrivateIPv4s() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []net.IP
	seen := map[string]bool{}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || !isPrivate(ipn.IP) || seen[ipn.IP.String()] {
				continue
			}
			seen[ipn.IP.String()] = true
			ips = append(ips, ipn.IP.To4())
		}
	}
	return primaryFirst(ips, defaultRouteIP())
}

// defaultRouteIP asks the OS which local address it would use to reach a public address.
// A UDP "dial" only picks a route; nothing is sent. 192.0.2.1 is a documentation address.
func defaultRouteIP() net.IP {
	c, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP
}

// primaryFirst moves primary to the front of ips if it is present, keeping the rest in order.
func primaryFirst(ips []net.IP, primary net.IP) []net.IP {
	if primary == nil {
		return ips
	}
	for i, ip := range ips {
		if ip.Equal(primary) {
			out := append([]net.IP{ip}, ips[:i]...)
			return append(out, ips[i+1:]...)
		}
	}
	return ips
}
