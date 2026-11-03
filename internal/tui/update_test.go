package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	upd "github.com/Abinsanth/stream-video-lan/internal/update"
)

func TestStartupCheckShowsNotice(t *testing.T) {
	m, _ := servingModel(t, vid("a.mp4", 1)) // no "no videos yet" flash covering the notice
	m.deps.Version = "0.1.0"
	m.deps.CheckUpdate = func(context.Context) (string, error) { return "0.2.0", nil }
	m = update(m, startupCheckCmd(m.deps)())
	if !strings.Contains(view(m), "v0.2.0 available — run lanstream --update") {
		t.Fatal(view(m))
	}
	m = press(m, "esc")
	if strings.Contains(view(m), "available") {
		t.Fatal("esc did not dismiss")
	}
}

func TestStartupNoticeOnPicker(t *testing.T) {
	m, _ := pickerModel(t, nil)
	m = update(m, updateMsg{latest: "0.2.0"})
	if !strings.Contains(view(m), "v0.2.0 available — run lanstream --update") {
		t.Fatal(view(m))
	}
}

func TestStartupCheckSilentOnFailure(t *testing.T) {
	d := Deps{Version: "0.1.0", CheckUpdate: func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	start := time.Now()
	if msg := startupCheckCmd(d)(); msg != nil {
		t.Fatal(msg)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("check not bounded by its 2 s timeout")
	}
	d.CheckUpdate = func(context.Context) (string, error) { return "", errors.New("offline") }
	if msg := startupCheckCmd(d)(); msg != nil {
		t.Fatal("notice on error")
	}
	d.CheckUpdate = func(context.Context) (string, error) { return "0.1.0", nil }
	if msg := startupCheckCmd(d)(); msg != nil {
		t.Fatal("notice for same version")
	}
	d.Version = "dev"
	if startupCheckCmd(d) != nil {
		t.Fatal("dev build checks for updates")
	}
	d.Version, d.CheckUpdate = "0.1.0", nil
	if startupCheckCmd(d) != nil {
		t.Fatal("check without a checker")
	}
}

func TestUpdateModal(t *testing.T) {
	m, _ := servingModel(t)
	m.deps.Version = "0.1.0"
	m.deps.CheckUpdate = func(context.Context) (string, error) { return "0.2.0", nil }
	m.deps.UpdateNotes = func(context.Context, string) (string, error) { return "Faster seeking", nil }
	m = typeText(m, "/update")
	m = press(m, "enter")
	if !strings.Contains(view(m), "Checking") {
		t.Fatal(view(m))
	}
	m = runCmds(m)
	for _, s := range []string{"You have v0.1.0.", "v0.2.0 is available.", "Faster seeking", "lanstream --update"} {
		if !strings.Contains(view(m), s) {
			t.Errorf("missing %q in\n%s", s, view(m))
		}
	}
	m = press(m, "esc")
	if strings.Contains(view(m), "You have") {
		t.Fatal("esc did not close")
	}
}

func TestUpdateModalUpToDateErrorAndDev(t *testing.T) {
	m, _ := servingModel(t)
	m.deps.Version = "0.2.0"
	m.deps.CheckUpdate = func(context.Context) (string, error) { return "0.2.0", nil }
	m = typeText(m, "/update")
	m = press(m, "enter")
	m = runCmds(m)
	if !strings.Contains(view(m), "This is the latest version.") {
		t.Fatal(view(m))
	}
	m = press(m, "esc")
	m.deps.CheckUpdate = func(context.Context) (string, error) { return "", errors.New("offline") }
	m = typeText(m, "/update")
	m = press(m, "enter")
	m = runCmds(m)
	if !strings.Contains(view(m), "Couldn't check for updates: offline") {
		t.Fatal(view(m))
	}
	m = press(m, "esc")
	m.deps.Version = "dev"
	m = typeText(m, "/update")
	m, cmd := updateCmd(m, "enter")
	if cmd != nil {
		t.Fatal("dev build went to the network")
	}
	if !strings.Contains(view(m), "development build") || !strings.Contains(view(m), "install.sh") {
		t.Fatal(view(m))
	}
}

func TestHelpListsUpdate(t *testing.T) {
	m, _ := servingModel(t)
	m = typeText(m, "/h")
	m = press(m, "enter")
	if !strings.Contains(view(m), "/update") {
		t.Fatal(view(m))
	}
	_ = upd.InstallLine
}
