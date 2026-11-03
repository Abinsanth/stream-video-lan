//go:build linux

package awake

import "testing"

func TestLogindAcquireRelease(t *testing.T) {
	inh := System()
	if err := inh.Acquire(); err != nil {
		t.Skip("no logind inhibitor here:", err)
	}
	if err := inh.Release(); err != nil {
		t.Fatal(err)
	}
}
