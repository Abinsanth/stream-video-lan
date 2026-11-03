package viewers

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestActiveWhileTransferringAndForIdleWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	tr := New(func() time.Time { return now })
	x := tr.Begin("192.168.1.12", "Your Name.mp4", 1000, 400)
	x.Wrote(50)
	if got := tr.Snapshot(); len(got) != 1 || got[0] != (Viewer{"192.168.1.12", "Your Name.mp4", 45}) {
		t.Fatal(got)
	}
	x.End()
	now = now.Add(29 * time.Second)
	if !tr.Active() {
		t.Fatal("should still be active inside 30 s")
	}
	now = now.Add(2 * time.Second)
	if tr.Active() || len(tr.Snapshot()) != 0 {
		t.Fatal("should be idle after 30 s")
	}
}

func TestInFlightTransferStaysActiveBeyondWindow(t *testing.T) {
	now := time.Unix(0, 0)
	tr := New(func() time.Time { return now })
	tr.Begin("ip", "long.mkv", 100, 0) // never ends: a long, slow stream
	now = now.Add(10 * time.Minute)
	if !tr.Active() {
		t.Fatal("in-flight transfer went idle")
	}
}

func TestOlderOverlappingTransferDoesNotMovePosition(t *testing.T) {
	tr := New(nil)
	old := tr.Begin("ip", "f.mkv", 100, 0)
	seek := tr.Begin("ip", "f.mkv", 100, 80)
	old.Wrote(10)
	seek.Wrote(5)
	if p := tr.Snapshot()[0].Percent; p != 85 {
		t.Fatal(p)
	}
}

func TestSnapshotSortedAndClamped(t *testing.T) {
	tr := New(nil)
	tr.Begin("192.168.1.20", "b.mp4", 10, 0).Wrote(50) // over-report clamps to 100
	tr.Begin("192.168.1.12", "z.mp4", 0, 0)            // zero size → 0
	got := tr.Snapshot()
	want := []Viewer{{"192.168.1.12", "z.mp4", 0}, {"192.168.1.20", "b.mp4", 100}}
	if !slices.Equal(got, want) {
		t.Fatal(got)
	}
}

func TestConcurrentUse(t *testing.T) {
	tr := New(nil)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			x := tr.Begin(fmt.Sprintf("10.0.0.%d", i%5), "f.mp4", 1000, int64(i))
			x.Wrote(10)
			_ = tr.Snapshot()
			x.End()
		}(i)
	}
	wg.Wait()
	if len(tr.Snapshot()) != 5 {
		t.Fatal(tr.Snapshot())
	}
}

func TestReset(t *testing.T) {
	tr := New(nil)
	tr.Begin("ip", "a.mp4", 10, 0)
	tr.Reset()
	if tr.Active() {
		t.Fatal("Reset kept viewers")
	}
}
