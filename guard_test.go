package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gofrs/flock"
)

func setup(t *testing.T) Event {
	t.Helper()
	t.Setenv("PLUGIN_DATA", filepath.Join(t.TempDir(), "中文 空间"))
	return Event{Name: "PreToolUse", Session: "test-session", Turn: "turn-1", Cwd: "/项目 空间", Tool: "mcp__github__create_issue", Input: map[string]any{"repository_full_name": "example/project", "title": "三思 🌙", "body": "Full body"}}
}

func call(t *testing.T, e Event) map[string]any {
	t.Helper()
	r, err := handle(e)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func blocked(t *testing.T, r map[string]any) {
	t.Helper()
	out, ok := r["hookSpecificOutput"].(map[string]any)
	if !ok || out["permissionDecision"] != "deny" {
		t.Fatalf("expected denial: %v", r)
	}
}

func state(t *testing.T, e Event) State {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("PLUGIN_DATA"), e.Session+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var s State
	if err = decode(bytes.NewReader(data), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func prompt(t *testing.T, e Event, message string) {
	t.Helper()
	e.Name = "UserPromptSubmit"
	e.Prompt = message
	call(t, e)
}

func grant(t *testing.T, e Event) Event {
	t.Helper()
	blocked(t, call(t, e))
	e.Turn = "turn-2"
	prompt(t, e, "授权提交")
	return e
}

func TestSingleUse(t *testing.T) {
	e := grant(t, setup(t))
	if r := call(t, e); len(r) != 0 {
		t.Fatalf("expected allowed: %v", r)
	}
	if s := state(t, e); s != (State{}) {
		t.Fatal("must consume before actual tool result")
	}
	blocked(t, call(t, e))
}

func TestExactIndependentMessage(t *testing.T) {
	e := setup(t)
	blocked(t, call(t, e))
	id := state(t, e).Request
	for _, message := range []string{"继续", "修好", "同意提交", "最后提交", "需求完成后授权提交", "\"授权提交\"", "授权提交\n再做一个", " 授权提交", "授权提交\n", "授权提交 " + id, "授权提交 " + strings.Repeat("0", 32)} {
		t.Run(message, func(t *testing.T) { e.Turn = "turn-2"; prompt(t, e, message); blocked(t, call(t, e)) })
	}
	e.Turn = "turn-1"
	prompt(t, e, "授权提交")
	blocked(t, call(t, e))
	e.Input.(map[string]any)["user_approved"] = true
	blocked(t, call(t, e))
}

func TestBindingAndRevocation(t *testing.T) {
	changes := map[string]func(*Event){
		"repo":        func(e *Event) { e.Input.(map[string]any)["repository_full_name"] = "other/repo" },
		"title":       func(e *Event) { e.Input.(map[string]any)["title"] = "Changed" },
		"body":        func(e *Event) { e.Input.(map[string]any)["body"] = "Changed" },
		"nested":      func(e *Event) { e.Input.(map[string]any)["extra"] = map[string]any{"user_approved": true} },
		"tool":        func(e *Event) { e.Tool = "mcp__other__create_issue" },
		"cwd":         func(e *Event) { e.Cwd = "/other" },
		"turn":        func(e *Event) { e.Turn = "turn-3" },
		"session":     func(e *Event) { e.Session = "other" },
		"new message": func(e *Event) { prompt(t, *e, "继续") },
		"interrupt":   func(e *Event) { v := *e; v.Name = "Interrupt"; call(t, v) },
		"resume":      func(e *Event) { v := *e; v.Name = "SessionStart"; v.Source = "resume"; call(t, v) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) { e := grant(t, setup(t)); change(&e); blocked(t, call(t, e)) })
	}
	t.Run("compaction", func(t *testing.T) {
		e := grant(t, setup(t))
		v := e
		v.Name = "SessionStart"
		v.Source = "compact"
		call(t, v)
		if len(call(t, e)) != 0 {
			t.Fatal("same-turn compaction revoked grant")
		}
	})
	t.Run("other chat prompt", func(t *testing.T) {
		e := setup(t)
		blocked(t, call(t, e))
		v := e
		v.Session = "other"
		v.Turn = "turn-2"
		prompt(t, v, "授权提交")
		e.Turn = "turn-2"
		blocked(t, call(t, e))
	})
}

func TestStructuredVariants(t *testing.T) {
	for _, tool := range []string{"mcp__github__create_pull_request", "github.create_pr", "mcp__github.create_issue", "github.create_issue", "mcp__github__issue_write", "mcp__github__pull_request_write"} {
		t.Run(tool, func(t *testing.T) {
			e := setup(t)
			e.Tool = tool
			e.Input = map[string]any{"owner": "example", "repo": "project", "head": "fix", "base": "main", "draft": true, "action": "create"}
			e = grant(t, e)
			if len(call(t, e)) != 0 {
				t.Fatal("grant did not work")
			}
		})
	}
	for _, field := range []string{"head", "base", "draft"} {
		t.Run(field, func(t *testing.T) {
			e := setup(t)
			e.Tool = "github.create_pull_request"
			e.Input.(map[string]any)[field] = "old"
			e = grant(t, e)
			id := state(t, e).Request
			e.Input.(map[string]any)[field] = "new"
			blocked(t, call(t, e))
			if state(t, e).Request == id {
				t.Fatal("changed request kept id")
			}
		})
	}
}

func TestReadAndLocalWork(t *testing.T) {
	e := setup(t)
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"mcp__github__list_issues", map[string]any{}},
		{"mcp__github__list_repositories", map[string]any{}},
		{"mcp__github__repository_write", map[string]any{"method": "update"}},
		{"mcp__github__issue_write", map[string]any{"method": "update"}},
		{"apply_patch", map[string]any{"command": "documentation mentioning gh pr create"}},
		{"Bash", map[string]any{"command": "gh api repos/a/b/issues"}},
		{"exec_command", map[string]any{"cmd": "git diff"}},
		{"shell", map[string]any{"command": []any{"sh", "-c", "gh issue list"}}},
	} {
		e.Tool = tc.tool
		e.Input = tc.args
		if len(call(t, e)) != 0 {
			t.Fatal(tc.tool)
		}
	}
	if _, err := os.Stat(os.Getenv("PLUGIN_DATA")); !os.IsNotExist(err) {
		t.Fatal("reads wrote state")
	}
	e.Tool = "shell"
	e.Input = map[string]any{"command": []any{"sh", "-c", "gh pr create"}}
	blocked(t, call(t, e))
}

func TestMain(m *testing.M) {
	if os.Getenv("SPS_TEST_HOOK") == "1" {
		main()
	}
	os.Exit(m.Run())
}

func process(payload []byte) ([]byte, int) {
	exe, _ := os.Executable()
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "SPS_TEST_HOOK=1")
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return out, e.ExitCode()
		}
		return out, -1
	}
	return out, 0
}

func TestConcurrentConsumption(t *testing.T) {
	e := grant(t, setup(t))
	payload, _ := json.Marshal(e)
	var group sync.WaitGroup
	results := make(chan bool, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			out, code := process(payload)
			results <- code == 0 && strings.TrimSpace(string(out)) == "{}"
		}()
	}
	group.Wait()
	close(results)
	allowed := 0
	for ok := range results {
		if ok {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("allowed %d calls", allowed)
	}
}

func TestBusyLock(t *testing.T) {
	e := grant(t, setup(t))
	l := flock.New(filepath.Join(os.Getenv("PLUGIN_DATA"), e.Session+".lock"))
	defer l.Close()
	if ok, err := l.TryLock(); err != nil || !ok {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(e)
	if out, code := process(payload); code != 2 {
		t.Fatalf("lock contention: %d %s", code, out)
	}
	if err := l.Unlock(); err != nil {
		t.Fatal(err)
	}
	if len(call(t, e)) != 0 {
		t.Fatal("lock contention consumed grant")
	}
}

func TestFailuresAndUnicode(t *testing.T) {
	e := setup(t)
	e.Input.(map[string]any)["body"] = strings.Repeat("正文🌙", 12000)
	payload, _ := json.Marshal(e)
	if out, code := process(payload); code != 0 {
		t.Fatalf("unicode: %d %s", code, out)
	}
	if state(t, e).Target.Input["body"] != e.Input.(map[string]any)["body"] {
		t.Fatal("unicode payload changed")
	}
	for _, input := range []string{"not-json", "[]", "null", "{}", "{} {}", `{"hook_event_name":1}`, `{"hook_event_name":"PreToolUse","tool_name":"shell","tool_input":{}}`} {
		if _, code := process([]byte(input)); code != 2 {
			t.Fatalf("invalid input allowed: %s", input)
		}
	}
	for _, bad := range []string{"broken", "[]", "null", `{"authorized_turn":"turn-2"}`, `{"target":{},"request_id":"not-random","created_turn":"turn-1"}`} {
		if err := os.WriteFile(filepath.Join(os.Getenv("PLUGIN_DATA"), e.Session+".json"), []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, code := process(payload); code != 2 {
			t.Fatalf("invalid state allowed: %s", bad)
		}
	}
}

func TestLauncher(t *testing.T) {
	e := setup(t)
	root := filepath.Join(t.TempDir(), "插件 中文 空格")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	name := "issue-pr-guard"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, "bin", name)
	if err := os.WriteFile(path, binary, 0700); err != nil {
		t.Fatal(err)
	}
	launcher := "run.sh"
	if runtime.GOOS == "windows" {
		launcher = "run.ps1"
	}
	if err := os.Mkdir(filepath.Join(root, "hooks"), 0700); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join("plugins", "codex", "hooks", launcher))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "hooks", launcher), script, 0700); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(e)
	invoke := func() ([]byte, int) {
		cmd := exec.Command("/bin/sh", filepath.Join(root, "hooks", launcher))
		if runtime.GOOS == "windows" {
			cmd = exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(root, "hooks", launcher))
		}
		cmd.Env = append(os.Environ(), "PLUGIN_ROOT="+root, "SPS_TEST_HOOK=1")
		if runtime.GOOS != "windows" {
			cmd.Env = append(cmd.Env, "PATH=/nonexistent")
		}
		cmd.Stdin = bytes.NewReader(payload)
		out, err := cmd.Output()
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				return out, e.ExitCode()
			}
			t.Fatal(err)
		}
		return out, 0
	}
	if out, code := invoke(); code != 0 || !strings.Contains(string(out), `"permissionDecision":"deny"`) {
		t.Fatalf("launcher: %d %s", code, out)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if out, code := invoke(); code != 2 {
		t.Fatalf("missing program allowed: %d %s", code, out)
	}
	if runtime.GOOS != "windows" {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
		if out, code := invoke(); code != 2 {
			t.Fatalf("crashed program allowed: %d %s", code, out)
		}
	}
}

func TestStateWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions")
	}
	e := grant(t, setup(t))
	root := os.Getenv("PLUGIN_DATA")
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0700)
	payload, _ := json.Marshal(e)
	if out, code := process(payload); code != 2 {
		t.Fatalf("unpersisted consumption allowed: %d %s", code, out)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if len(call(t, e)) != 0 {
		t.Fatal("failed write consumed authorization")
	}
}

func TestHookConfiguration(t *testing.T) {
	var config struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type, Command, CommandWindows string
				Async                         bool
			}
		}
	}
	b, err := os.ReadFile("plugins/codex/hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &config); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "SubagentStart", "UserPromptSubmit", "Interrupt", "PreToolUse"} {
		groups := config.Hooks[event]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatal(event)
		}
		h := groups[0].Hooks[0]
		if h.Async || h.Type != "command" || !strings.Contains(h.Command, "run.sh") || !strings.Contains(h.CommandWindows, "run.ps1") {
			t.Fatal(event, h)
		}
	}
}

func TestStateUnavailable(t *testing.T) {
	for _, session := range []string{"", "../outside", "聊天"} {
		t.Run(session, func(t *testing.T) {
			e := setup(t)
			e.Session = session
			payload, _ := json.Marshal(e)
			if _, code := process(payload); code != 2 {
				t.Fatal("invalid session allowed")
			}
		})
	}
	t.Run("missing data directory", func(t *testing.T) {
		e := setup(t)
		t.Setenv("PLUGIN_DATA", "")
		t.Setenv("CLAUDE_PLUGIN_DATA", "")
		payload, _ := json.Marshal(e)
		if _, code := process(payload); code != 2 {
			t.Fatal("missing state location allowed")
		}
	})
	t.Run("not a directory", func(t *testing.T) {
		e := setup(t)
		path := os.Getenv("PLUGIN_DATA")
		if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(e)
		if _, code := process(payload); code != 2 {
			t.Fatal("state I/O error allowed")
		}
	})
	t.Run("oversized input", func(t *testing.T) {
		setup(t)
		if _, code := process([]byte(strings.Repeat(" ", maxJSON+1))); code != 2 {
			t.Fatal("oversized input accepted")
		}
	})
}

func TestMissingBinding(t *testing.T) {
	for _, field := range []string{"input", "turn", "cwd", "repo"} {
		t.Run(field, func(t *testing.T) {
			e := setup(t)
			switch field {
			case "input":
				e.Input = nil
			case "turn":
				e.Turn = ""
			case "cwd":
				e.Cwd = ""
			case "repo":
				e.Input = map[string]any{"title": "x"}
			}
			blocked(t, call(t, e))
		})
	}
}

func TestInvalidChangedRequestRevokesGrant(t *testing.T) {
	for _, field := range []string{"repo", "input", "cwd", "turn"} {
		t.Run(field, func(t *testing.T) {
			e := grant(t, setup(t))
			changed := e
			switch field {
			case "repo":
				changed.Input = map[string]any{"title": "changed"}
			case "input":
				changed.Input = nil
			case "cwd":
				changed.Cwd = ""
			case "turn":
				changed.Turn = ""
			}
			blocked(t, call(t, changed))
			blocked(t, call(t, e))
		})
	}
}
