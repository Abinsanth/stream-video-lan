package installtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tagRepo makes a git repo with VERSION 0.1.0 and a bare "origin", plus a fake gh on PATH whose
// "release view" reports state: "none" (no release), "true" (draft) or "false" (published).
func tagRepo(t *testing.T, state string) (repo string, run func() (string, error)) {
	t.Helper()
	root := t.TempDir()
	repo, origin, bin := filepath.Join(root, "repo"), filepath.Join(root, "origin.git"), filepath.Join(root, "bin")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	os.MkdirAll(repo, 0o755)
	os.MkdirAll(bin, 0o755)
	git(root, "init", "-q", "--bare", origin)
	git(repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "VERSION"), []byte("0.1.0\n"), 0o644)
	git(repo, "add", ".")
	git(repo, "commit", "-q", "-m", "one")
	git(repo, "remote", "add", "origin", origin)
	git(repo, "push", "-q", "origin", "main")
	gh := "#!/bin/sh\ncase \"$1 $2\" in\n\"release view\") [ \"" + state + "\" = none ] && exit 1; echo " + state + " ;;\nesac\n"
	os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755)
	script, _ := filepath.Abs("../../.github/tag-release.sh")
	return repo, func() (string, error) {
		cmd := exec.Command("sh", script)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "GITHUB_REPOSITORY=o/r",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
}

func originHasTag(t *testing.T, repo, tag string) bool {
	out, _ := exec.Command("git", "-C", repo, "ls-remote", "--tags", "origin", tag).Output()
	return strings.Contains(string(out), tag)
}

func TestTagReleaseFirstRun(t *testing.T) {
	repo, run := tagRepo(t, "none")
	if out, err := run(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !originHasTag(t, repo, "v0.1.0") {
		t.Fatal("tag not pushed")
	}
}

func TestTagReleaseRerunAfterPartialFailure(t *testing.T) {
	repo, run := tagRepo(t, "none")
	if out, err := run(); err != nil { // first attempt tagged and pushed, then (say) goreleaser failed
		t.Fatalf("%v\n%s", err, out)
	}
	_, rerun := tagRepoAgain(t, repo, "true") // a draft release was left behind
	if out, err := rerun(); err != nil {
		t.Fatalf("re-run failed: %v\n%s", err, out)
	}
}

func TestTagReleaseRefusesPublishedVersion(t *testing.T) {
	_, run := tagRepo(t, "false")
	out, err := run()
	if err == nil || !strings.Contains(out, "already released") {
		t.Fatalf("err=%v\n%s", err, out)
	}
}

func TestTagReleaseRefusesTagOnOtherCommit(t *testing.T) {
	repo, run := tagRepo(t, "none")
	exec.Command("git", "-C", repo, "tag", "v0.1.0").Run()
	os.WriteFile(filepath.Join(repo, "x"), []byte("x"), 0o644)
	c := exec.Command("git", "-C", repo, "add", "x")
	c.Run()
	c = exec.Command("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "two")
	c.Run()
	out, err := run()
	if err == nil || !strings.Contains(out, "points at another commit") {
		t.Fatalf("err=%v\n%s", err, out)
	}
}

// tagRepoAgain reuses repo but swaps the fake gh's reported release state.
func tagRepoAgain(t *testing.T, repo, state string) (string, func() (string, error)) {
	t.Helper()
	bin := filepath.Join(filepath.Dir(repo), "bin")
	gh := "#!/bin/sh\ncase \"$1 $2\" in\n\"release view\") [ \"" + state + "\" = none ] && exit 1; echo " + state + " ;;\nesac\n"
	os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755)
	script, _ := filepath.Abs("../../.github/tag-release.sh")
	return repo, func() (string, error) {
		cmd := exec.Command("sh", script)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "GITHUB_REPOSITORY=o/r")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
}
