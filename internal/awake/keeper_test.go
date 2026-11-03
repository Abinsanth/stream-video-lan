package awake

import (
	"errors"
	"testing"
	"time"
)

type fakeInh struct {
	held, acquires, releases int
	fail                     bool
}

func (f *fakeInh) Acquire() error {
	if f.fail {
		return errors.New("no")
	}
	f.held = 1
	f.acquires++
	return nil
}

func (f *fakeInh) Release() error {
	f.held = 0
	f.releases++
	return nil
}

func TestKeeperHoldsWhileActiveAndReleasesAfterTwoMinutes(t *testing.T) {
	now := time.Unix(0, 0)
	f := &fakeInh{}
	k := NewKeeper(f, func() time.Time { return now })
	k.Update(true)
	k.Update(true)
	if !k.Held() || f.acquires != 1 {
		t.Fatal(f)
	}
	now = now.Add(119 * time.Second)
	k.Update(false)
	if !k.Held() {
		t.Fatal("released too early")
	}
	now = now.Add(2 * time.Second)
	k.Update(false)
	if k.Held() || f.releases != 1 {
		t.Fatal(f)
	}
}

func TestKeeperRetriesFailedAcquireAndCloseReleases(t *testing.T) {
	f := &fakeInh{fail: true}
	k := NewKeeper(f, nil)
	k.Update(true)
	if k.Held() {
		t.Fatal("held after failed acquire")
	}
	f.fail = false
	k.Update(true)
	if !k.Held() {
		t.Fatal("no retry")
	}
	k.Close()
	k.Close()
	if k.Held() || f.releases != 1 {
		t.Fatal(f)
	}
}

func TestNilKeeperIsSafe(t *testing.T) {
	var k *Keeper
	k.Update(true)
	k.Close()
	if k.Held() {
		t.Fatal("nil keeper held")
	}
}
