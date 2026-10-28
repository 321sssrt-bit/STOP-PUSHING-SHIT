package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitFixture(t *testing.T) (Event, func(...string) string, string) {
	t.Helper()
	e := setup(t)
	root := t.TempDir()
	work, remote := filepath.Join(root, "work"), filepath.Join(root, "remote.git")
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "--bare", remote)
	run("init", "-b", "main", work)
	git := func(args ...string) string { return run(append([]string{"-C", work}, args...)...) }
	git("config", "user.name", "Guard test")
	git("config", "user.email", "guard@example.invalid")
	git("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(work, "example.txt"), []byte("review me\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "example.txt")
	git("commit", "-m", "First reviewed change")
	git("remote", "add", "origin", remote)
	e.Cwd, e.Tool, e.Input = work, "exec_command", map[string]any{"cmd": "git push -u origin main"}
	return e, git, remote
}

func TestPushSingleUseRealGit(t *testing.T) {
	e, git, remote := gitFixture(t)
	blocked(t, call(t, e))
	s := state(t, e)
	if s.Target.Push == nil || s.Target.Push.Commit != git("rev-parse", "HEAD") || s.Target.Push.URL != remote || !strings.Contains(s.Target.Push.Changes, "example.txt") {
		t.Fatal("missing concrete push preview", s.Target)
	}
	e.Turn = "turn-2"
	prompt(t, e, "授权提交")
	if len(call(t, e)) != 0 {
		t.Fatal("reviewed push blocked")
	}
	if state(t, e) != (State{}) {
		t.Fatal("must consume before Git runs")
	}
	git("push", "-u", "origin", "main")
	blocked(t, call(t, e))
}

func TestPushChangesRevoke(t *testing.T) {
	for _, change := range []string{"commit", "remote", "command", "config", "mapping"} {
		t.Run(change, func(t *testing.T) {
			e, git, _ := gitFixture(t)
			e = grant(t, e)
			switch change {
			case "commit":
				git("commit", "--allow-empty", "-m", "Unreviewed change")
			case "remote":
				git("remote", "set-url", "origin", filepath.Join(t.TempDir(), "other.git"))
			case "command":
				e.Input.(map[string]any)["cmd"] = "git push --force origin main"
			case "config":
				git("config", "push.followTags", "true")
			case "mapping":
				git("config", "remote.origin.push", "main:refs/heads/other")
			}
			blocked(t, call(t, e))
		})
	}
}

func TestPushFailedExecutionConsumes(t *testing.T) {
	e, git, _ := gitFixture(t)
	git("remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	e = grant(t, e)
	if len(call(t, e)) != 0 {
		t.Fatal("grant not accepted")
	}
	cmd := exec.Command("git", "-C", e.Cwd, "push", "origin", "main")
	if err := cmd.Run(); err == nil {
		t.Fatal("fixture must fail")
	}
	blocked(t, call(t, e))
}

func TestPushRecognizedForms(t *testing.T) {
	for _, command := range []string{
		"git push", "git -C /tmp push origin main", "git -c push.followTags=true push origin main", "git --git-dir=/tmp/r push",
		"git push --help --force origin main", "env X=1 git push", "command git push", "sh -c 'git push origin main'", "echo x | bash -c 'git push'",
		"python3 -c 'import subprocess; subprocess.run([\"git\",\"push\",\"origin\",\"main\"])'", "ssh host git push origin main", "pwsh -Command 'git push origin main'", "git.exe push origin main", "git $ACTION",
	} {
		t.Run(command, func(t *testing.T) {
			if !shellCreation(command, 0, "") {
				t.Fatal("missed push")
			}
		})
	}
	for _, command := range []string{"git init", "git clone https://example.com/a.git", "git diff", "git status", "git push --help", "git -C /tmp log", "printf '%s' 'git push origin main'"} {
		if shellCreation(command, 0, "") {
			t.Fatal("blocked read/local work", command)
		}
	}
}

func TestUnboundPushNeverAuthorizable(t *testing.T) {
	e, _, _ := gitFixture(t)
	for _, command := range []string{"git push", "git push --all origin", "git push --tags origin", "git push --delete origin main", "git push origin main other", "git push origin main; git push origin main", "X=1 git push origin main", "git push origin $BRANCH", "git -c core.sshCommand=evil push origin main", "/tmp/git push origin main"} {
		t.Run(command, func(t *testing.T) {
			e.Input = map[string]any{"cmd": command}
			blocked(t, call(t, e))
			e.Turn = "later"
			prompt(t, e, "授权提交")
			blocked(t, call(t, e))
			if state(t, e).Target != nil {
				t.Fatal("ambiguous push became pending")
			}
		})
	}
}

func TestPlainAuthorizationWithoutPending(t *testing.T) {
	e := setup(t)
	e.Turn = "turn-2"
	prompt(t, e, "授权提交")
	blocked(t, call(t, e))
}

func directoryLink(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			t.Fatalf("create junction: %v %s", err, out)
		}
	} else if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestPushDirectoryBinding(t *testing.T) {
	a, _, _ := gitFixture(t)
	b, _, _ := gitFixture(t)
	sub := filepath.Join(b.Cwd, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(a.Cwd, "link")
	directoryLink(t, sub, link)
	// Keep ".." intact so the actual Git process, not a path helper, is the oracle.
	relative := "link" + string(os.PathSeparator) + ".."
	absolute := a.Cwd + string(os.PathSeparator) + relative
	for _, tc := range []struct {
		name    string
		workdir string
		dirs    []string
	}{
		{"plain", "", nil},
		{"relative-workdir", relative, nil},
		{"absolute-workdir", absolute, nil},
		{"relative-C", "", []string{relative}},
		{"absolute-C", "", []string{absolute}},
		{"successive-C", "", []string{"link", ".."}},
		{"empty-C", "", []string{"link", "", ".."}},
		{"workdir-and-C", "link", []string{".."}},
		{"missing-before-absolute-C", "", []string{"missing", b.Cwd}},
		{"missing-workdir-before-absolute-C", "missing", []string{b.Cwd}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := a
			input := map[string]any{}
			cwd := a.Cwd
			if tc.workdir != "" {
				input["workdir"] = tc.workdir
				cwd = tc.workdir
				if !filepath.IsAbs(cwd) {
					cwd = a.Cwd + string(os.PathSeparator) + cwd
				}
			}
			var prefix []string
			command := "git"
			for _, dir := range tc.dirs {
				prefix = append(prefix, "-C", dir)
				command += " -C '" + filepath.ToSlash(dir) + "'"
			}
			command += " push origin main"
			input["cmd"], e.Input = command, input
			oracle := exec.Command("git", append(prefix, "rev-parse", "--show-toplevel")...)
			oracle.Dir = cwd
			expected, oracleErr := oracle.Output()
			target, err := inspectPush(e, command)
			if oracleErr != nil {
				if err == nil || target != nil {
					t.Fatalf("invalid execution directory became authorizable: %+v, %v", target, err)
				}
				return
			}
			if err != nil || target == nil {
				t.Fatalf("inspect: %v", err)
			}
			if target.Repository != strings.TrimSpace(string(expected)) {
				t.Fatalf("reviewed %s; actual Git uses %s", target.Repository, expected)
			}
		})
	}
}

func TestPushDirectoryRetargetRevokes(t *testing.T) {
	e, _, _ := gitFixture(t)
	other, _, _ := gitFixture(t)
	link := filepath.Join(t.TempDir(), "repository")
	directoryLink(t, e.Cwd, link)
	e.Input = map[string]any{"cmd": "git -C '" + filepath.ToSlash(link) + "' push origin main"}
	e = grant(t, e)
	previous := state(t, e).Target.Push.Repository
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	directoryLink(t, other.Cwd, link)
	blocked(t, call(t, e))
	if next := state(t, e).Target.Push.Repository; next == previous {
		t.Fatal("retargeted directory kept the previous authorization target")
	}
}

func TestPushCodexBashRequiresAbsoluteDirectory(t *testing.T) {
	e, _, _ := gitFixture(t)
	e.Tool = "Bash"
	for _, command := range []string{"git push origin main", "git -C . push origin main", "git -C '' -C '" + filepath.ToSlash(e.Cwd) + "' push origin main"} {
		e.Input = map[string]any{"command": command}
		blocked(t, call(t, e))
		e.Turn = "later"
		prompt(t, e, "授权提交")
		blocked(t, call(t, e))
		if state(t, e).Target != nil {
			t.Fatal("missing execution directory became authorizable")
		}
	}
	e.Input = map[string]any{"command": "git -C '" + filepath.ToSlash(e.Cwd) + "' push origin main"}
	e = grant(t, e)
	if len(call(t, e)) != 0 {
		t.Fatal("explicit absolute repository was blocked")
	}
	blocked(t, call(t, e))
}
