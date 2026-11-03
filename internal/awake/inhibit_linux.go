//go:build linux

package awake

import (
	"context"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// System returns Linux's inhibitor: a systemd-logind "idle:sleep" block lock over D-Bus.
func System() Inhibitor { return &logind{fd: -1} }

type logind struct{ fd int }

// Acquire asks logind for an inhibitor lock. logind keeps it while we hold the returned file
// descriptor; the kernel closes it if lanstream dies, which releases the lock.
func (l *logind) Acquire() error {
	if l.fd >= 0 {
		return nil
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	// Bounded, so a hung logind can't freeze the UI for D-Bus's 25 s default.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var fd dbus.UnixFD
	err = conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		CallWithContext(ctx, "org.freedesktop.login1.Manager.Inhibit", 0,
			"idle:sleep", "lanstream", "Someone is watching a video", "block").
		Store(&fd)
	if err != nil {
		return err
	}
	l.fd = int(fd)
	return nil
}

func (l *logind) Release() error {
	if l.fd < 0 {
		return nil
	}
	err := syscall.Close(l.fd)
	l.fd = -1
	return err
}
