//go:build darwin

package awake

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func caffeinateRunning() bool {
	return exec.Command("pgrep", "-f", fmt.Sprintf("caffeinate -i -w %d", os.Getpid())).Run() == nil
}

func TestCaffeinateAcquireRelease(t *testing.T) {
	inh := System()
	if err := inh.Acquire(); err != nil {
		t.Fatal(err)
	}
	if !caffeinateRunning() {
		t.Fatal("caffeinate not running after Acquire")
	}
	if err := inh.Release(); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(2 * time.Second); caffeinateRunning(); {
		if time.Now().After(deadline) {
			t.Fatal("caffeinate still running after Release")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
