package host_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testCodexDirectoryBinding(t *testing.T, workspace, statePath string, model *fixtureModel, resume func(string) string) {
	t.Helper()
	git := func(cwd string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	linkDir := func(target, link string) {
		t.Helper()
		if runtime.GOOS == "windows" {
			out, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
			if err != nil {
				t.Fatalf("junction: %v %s", err, out)
			}
		} else {
			must(os.Symlink(target, link))
		}
	}
	a, b := filepath.Join(workspace, "A"), filepath.Join(workspace, "B")
	// A valid session repository makes a lost workdir dangerous, not merely an error.
	for _, repo := range []string{workspace, a, b} {
		must(os.MkdirAll(repo, 0700))
		git(repo, "init", "-b", "main")
		git(repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
			"-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", filepath.Base(repo))
		git(repo, "init", "--bare", repo+".git")
		git(repo, "remote", "add", "origin", repo+".git")
		git(repo, "config", "core.hooksPath", ".git/hooks")
		write(filepath.Join(repo, ".git", "hooks", "pre-push"),
			[]byte("#!/bin/sh\nprintf 'push\\n' >> '"+filepath.ToSlash(filepath.Join(repo, "push-count"))+"'\n"))
	}
	must(os.Mkdir(filepath.Join(b, "sub"), 0700))
	linkDir(filepath.Join(b, "sub"), filepath.Join(a, "link"))
	for _, tc := range []struct{ name, workdir, prefix string }{
		{"plain", a, "git"},
		{"relative-workdir", "A/link/..", "git"},
		{"absolute-workdir", a + "/link/..", "git"},
		{"relative-C", a, "git -C link/.."},
		{"absolute-C", workspace, "git -C '" + filepath.ToSlash(a) + "/link/..'"},
		{"successive-C", a, "git -C link -C .."},
		{"workdir-and-C", "A/link", "git -C .."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(workspace, tc.name+".txt")
			command := tc.prefix + " rev-parse --show-toplevel > '" + filepath.ToSlash(marker) + "'"
			if runtime.GOOS == "windows" {
				command = tc.prefix + " rev-parse --show-toplevel | Set-Content -Encoding UTF8 -LiteralPath '" + marker + "'"
			}
			call := func(cmd string) action {
				return action{"exec_command", object{"cmd": cmd, "workdir": tc.workdir, "login": false}}
			}
			model.set(call(command))
			resume("Read the actual local repository.")
			if absent(marker) {
				t.Fatal("actual shell did not run", model.feedback())
			}
			actual := strings.TrimSpace(strings.TrimPrefix(string(read(marker)), "\uFEFF"))
			model.set(call(tc.prefix + " push origin main"))
			resume("Review the local push.")
			var s struct {
				Target *struct {
					Push struct {
						Repository string `json:"repository"`
						URL        string `json:"destination_url"`
					} `json:"push"`
				} `json:"target"`
			}
			must(json.Unmarshal(read(statePath), &s))
			if tc.name != "absolute-C" {
				if s.Target != nil || !strings.Contains(model.feedback(), "宿主未提供实际执行目录") {
					t.Fatalf("hidden workdir must be rejected: %s", read(statePath))
				}
				return
			}
			if s.Target == nil || s.Target.Push.Repository != actual {
				t.Fatalf("review disagrees with actual repository %s: %s; %s", actual, read(statePath), model.feedback())
			}
			if s.Target.Push.URL != git(actual, "remote", "get-url", "--push", "origin") {
				t.Fatal("wrong push destination")
			}
		})
	}
	t.Run("retarget-single-use", func(t *testing.T) {
		selected := filepath.Join(workspace, "selected")
		linkDir(a, selected)
		call := action{"exec_command", object{"cmd": "git -C '" + filepath.ToSlash(selected) + "' push origin main", "workdir": a, "login": false}}
		pushed := func(repo string) bool {
			cmd := exec.Command("git", "-C", repo, "ls-remote", "--exit-code", "origin", "refs/heads/main")
			return cmd.Run() == nil
		}
		model.set(call)
		resume("Review repository A.")
		if pushed(a) {
			t.Fatal("unapproved push executed")
		}
		must(os.Remove(selected))
		linkDir(b, selected)
		model.set(call)
		resume("授权提交")
		if pushed(b) {
			t.Fatal("retargeted directory reused authorization")
		}
		model.set(call, call)
		resume("授权提交")
		if !pushed(b) || pushed(a) || !strings.Contains(model.feedback(), "授权提交") {
			t.Fatal("expected one push to B and a denied repeat", model.feedback())
		}
		if string(read(filepath.Join(b, "push-count"))) != "push\n" ||
			!absent(filepath.Join(a, "push-count")) || !absent(filepath.Join(workspace, "push-count")) {
			t.Fatal("push execution count or repository does not match the grant")
		}
		t.Log("CODEX_LOCAL_PUSH_COUNT: B=1 A=0 session=0")
	})
	if !t.Failed() {
		t.Log("CODEX_DIRECTORY_BINDING_PASSED: native shell, local remotes, retarget revoked, single use")
	}
}
