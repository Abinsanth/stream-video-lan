// Package viewers turns HTTP transfers into "who is watching what, and roughly where".
//
// The server only sees byte ranges, not playback, so a viewer is a (client IP, file) pair that is
// transferring now or did within the last 30 seconds, and its position is the byte offset reached
// by its most recent request as a share of the file size.
package viewers

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

const idleWindow = 30 * time.Second

// Viewer is one device watching one file.
type Viewer struct {
	IP, RelPath string
	Percent     int // 0..100, approximate playback position
}

type key struct{ ip, rel string }

type entry struct {
	size     int64
	pos      int64 // byte offset reached by the latest transfer
	inflight int
	last     time.Time // when the last transfer ended (or began, while none has ended)
	seq      uint64    // id of the latest transfer; only it may move pos
}

// Tracker records transfers. It is safe for concurrent use.
type Tracker struct {
	now func() time.Time

	mu      sync.Mutex
	entries map[key]*entry
	seq     uint64
}

// New returns a Tracker using now as its clock (nil means time.Now).
func New(now func() time.Time) *Tracker {
	if now == nil {
		now = time.Now
	}
	return &Tracker{now: now, entries: map[key]*entry{}}
}

// Transfer is one HTTP response body being sent.
type Transfer struct {
	t   *Tracker
	k   key
	seq uint64
	off int64
}

// Begin records the start of a transfer of rel (size bytes) to ip, starting at byte start.
func (t *Tracker) Begin(ip, rel string, size, start int64) *Transfer {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := key{ip, rel}
	e := t.entries[k]
	if e == nil {
		e = &entry{}
		t.entries[k] = e
	}
	t.seq++
	e.size, e.pos, e.seq, e.last = size, start, t.seq, t.now()
	e.inflight++
	return &Transfer{t: t, k: k, seq: t.seq, off: start}
}

// Wrote records n more bytes sent.
func (x *Transfer) Wrote(n int64) {
	x.t.mu.Lock()
	defer x.t.mu.Unlock()
	x.off += n
	if e := x.t.entries[x.k]; e != nil && e.seq == x.seq {
		e.pos = x.off
	}
}

// End records that the transfer finished (completed or the client went away).
func (x *Transfer) End() {
	x.t.mu.Lock()
	defer x.t.mu.Unlock()
	if e := x.t.entries[x.k]; e != nil {
		e.inflight--
		e.last = x.t.now()
	}
}

// Snapshot returns the active viewers sorted by IP then file, and forgets idle ones.
func (t *Tracker) Snapshot() []Viewer {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	var out []Viewer
	for k, e := range t.entries {
		if e.inflight <= 0 && now.Sub(e.last) >= idleWindow {
			delete(t.entries, k)
			continue
		}
		out = append(out, Viewer{IP: k.ip, RelPath: k.rel, Percent: percent(e.pos, e.size)})
	}
	slices.SortFunc(out, func(a, b Viewer) int {
		return cmp.Or(cmp.Compare(a.IP, b.IP), cmp.Compare(a.RelPath, b.RelPath))
	})
	return out
}

// Active reports whether anyone is watching.
func (t *Tracker) Active() bool { return len(t.Snapshot()) > 0 }

func percent(pos, size int64) int {
	if size <= 0 {
		return 0
	}
	return int(min(max(pos*100/size, 0), 100))
}

// Reset forgets every viewer (used when switching to a different folder).
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	clear(t.entries)
}
