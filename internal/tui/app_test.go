package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/Abinsanth/stream-video-lan/internal/awake"
	"github.com/Abinsanth/stream-video-lan/internal/viewers"
)

func TestCopiedFlash(t *testing.T) {
	m, _ := servingModel(t)
	m = update(m, copiedMsg{text: "http://x/playlist.m3u8"})
	if !strings.Contains(view(m), "Copied") {
		t.Fatal(view(m))
	}
	m = update(m, copiedMsg{text: "http://x/p", err: errors.New("no clipboard")})
	if !strings.Contains(view(m), "http://x/p") || !strings.Contains(view(m), "Copy this link") {
		t.Fatal(view(m))
	}
}

func TestFlashExpires(t *testing.T) {
	m, _ := servingModel(t)
	m = update(m, copiedMsg{text: "x"})
	m = advance(m, 4e9)
	m = update(m, tickMsg{})
	if strings.Contains(view(m), "Copied") {
		t.Fatal("flash did not expire")
	}
}

func TestTickDrivesKeeper(t *testing.T) {
	inh := &fakeInh{}
	tr := viewers.New(nil)
	m := newTestModel(t, func(d *Deps) { d.Tracker = tr; d.Keeper = awake.NewKeeper(inh, nil) })
	tr.Begin("ip", "a.mp4", 10, 0)
	m = update(m, tickMsg{})
	if inh.acquires != 1 {
		t.Fatal("keeper not updated on tick")
	}
}
