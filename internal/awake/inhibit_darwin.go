//go:build darwin

package awake

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

// System returns macOS's inhibitor: the built-in caffeinate tool.
func System() Inhibitor { return &caffeinate{} }

type caffeinate struct{ cmd *exec.Cmd }

// Acquire runs "caffeinate -i -w <our pid>": it blocks idle sleep, and because of -w it also
// exits by itself if lanstream dies, so a crash can't leave the Mac awake forever.
func (c *caffeinate) Acquire() error {
	if c.cmd != nil {
		return nil
	}
	cmd := exec.Command("caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	return nil
}

func (c *caffeinate) Release() error {
	if c.cmd == nil {
		return nil
	}
	err := c.cmd.Process.Kill()
	c.cmd.Wait()
	c.cmd = nil
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
