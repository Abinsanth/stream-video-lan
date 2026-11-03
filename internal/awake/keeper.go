// Package awake stops the computer from idle-sleeping while someone is watching.
package awake

import (
	"sync"
	"time"
)

// releaseDelay is how long after the last viewer goes idle the machine may sleep again.
const releaseDelay = 2 * time.Minute

// Inhibitor is an OS mechanism that blocks idle sleep while held.
type Inhibitor interface {
	Acquire() error
	Release() error
}

// Keeper decides when to hold the inhibitor. Call Update on every UI tick. A nil *Keeper is a
// valid no-op, so callers needn't check.
type Keeper struct {
	inh Inhibitor
	now func() time.Time

	mu         sync.Mutex
	held       bool
	lastActive time.Time
}

// NewKeeper returns a Keeper for inh, using now as its clock (nil means time.Now).
func NewKeeper(inh Inhibitor, now func() time.Time) *Keeper {
	if now == nil {
		now = time.Now
	}
	return &Keeper{inh: inh, now: now}
}

// Update holds the inhibitor while active is true, and releases it once nobody has been active
// for two minutes. A failed Acquire is retried on the next active Update.
func (k *Keeper) Update(active bool) {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	switch {
	case active:
		k.lastActive = now
		if !k.held && k.inh.Acquire() == nil {
			k.held = true
		}
	case k.held && now.Sub(k.lastActive) >= releaseDelay:
		k.inh.Release()
		k.held = false
	}
}

// Held reports whether sleep is currently being blocked.
func (k *Keeper) Held() bool {
	if k == nil {
		return false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.held
}

// Close releases the inhibitor if held. It is safe to call more than once.
func (k *Keeper) Close() {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.held {
		k.inh.Release()
		k.held = false
	}
}
