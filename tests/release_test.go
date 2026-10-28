package host_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleasePackage(t *testing.T) {
	archive := os.Getenv("SPS_RELEASE_PACKAGE")
	if archive == "" {
		t.Skip("set SPS_RELEASE_PACKAGE to a native release archive")
	}
	z, err := zip.OpenReader(archive)
	must(err)
	defer z.Close()
	files := make(map[string]*zip.File, len(z.File))
	for _, entry := range z.File {
		files[entry.Name] = entry
	}
	var marketplace struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
		} `json:"plugins"`
	}
	var manifest struct {
		Name  string `json:"name"`
		Hooks string `json:"hooks"`
	}
	for name, target := range map[string]any{
		".agents/plugins/marketplace.json": &marketplace,
		"plugin/.codex-plugin/plugin.json": &manifest,
		"plugin/hooks/hooks.json":          new(any),
	} {
		entry := files[name]
		if entry == nil {
			t.Fatal("missing installation manifest", name)
		}
		in, err := entry.Open()
		must(err)
		must(json.NewDecoder(in).Decode(target))
		must(in.Close())
	}
	if len(marketplace.Plugins) != 1 || marketplace.Plugins[0].Name != "issue-pr-guard" || marketplace.Plugins[0].Source.Path != "./plugin" || manifest.Name != "issue-pr-guard" || manifest.Hooks != "./hooks/hooks.json" {
		t.Fatal("invalid installation entrypoint", marketplace, manifest)
	}
	root := filepath.Join(t.TempDir(), "安装包 中文 空格")
	for _, entry := range z.File {
		if !filepath.IsLocal(entry.Name) {
			t.Fatal("unsafe archive path", entry.Name)
		}
		mode := entry.Mode().Perm()
		if strings.HasPrefix(entry.Name, "plugin/bin/") && mode&0111 == 0 {
			t.Fatal("missing executable permission", entry.Name)
		}
		in, err := entry.Open()
		must(err)
		data, err := io.ReadAll(in)
		must(err)
		must(in.Close())
		path := filepath.Join(root, entry.Name)
		must(os.MkdirAll(filepath.Dir(path), 0700))
		must(os.WriteFile(path, data, mode))
		must(os.Chmod(path, mode))
	}
	dataRoot := filepath.Join(root, "状态")
	invoke := func(event object) object {
		cmd := exec.Command("/bin/sh", filepath.Join(root, "plugin/hooks/run.sh"))
		if runtime.GOOS == "windows" {
			cmd = exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(root, "plugin/hooks/run.ps1"))
		}
		cmd.Env = append(os.Environ(), "PLUGIN_ROOT="+filepath.Join(root, "plugin"), "PLUGIN_DATA="+dataRoot)
		if runtime.GOOS != "windows" {
			cmd.Env = append(cmd.Env, "PATH=/nonexistent")
		}
		cmd.Stdin = bytes.NewBufferString(encode(event))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("release launcher: %v %s", err, out)
		}
		var result object
		must(json.Unmarshal(out, &result))
		return result
	}
	denied := func(result object) {
		t.Helper()
		h, ok := result["hookSpecificOutput"].(map[string]any)
		if !ok || h["permissionDecision"] != "deny" {
			t.Fatal("expected denial", result)
		}
	}
	body := strings.Repeat("正文🌙", 12000)
	e := object{"hook_event_name": "PreToolUse", "session_id": "release-check", "turn_id": "one", "cwd": "/项目 中文", "tool_name": "github.create_pull_request", "tool_input": object{"repository_full_name": "example/project", "title": "包验证", "body": body, "head": "feature", "base": "main", "draft": true}}
	denied(invoke(e))
	var state object
	must(json.Unmarshal(read(filepath.Join(dataRoot, "release-check.json")), &state))
	if state["target"].(map[string]any)["tool_input"].(map[string]any)["body"] != body {
		t.Fatal("release corrupted Unicode body")
	}
	invoke(object{"hook_event_name": "UserPromptSubmit", "session_id": "release-check", "turn_id": "two", "prompt": "授权提交"})
	e["turn_id"] = "two"
	if result := invoke(e); len(result) != 0 {
		t.Fatal("exact grant did not work", result)
	}
	denied(invoke(e))
	repoEvent := object{"hook_event_name": "PreToolUse", "session_id": "release-repository", "turn_id": "one", "cwd": root, "tool_name": "github.create_repository", "tool_input": object{"name": "fixture-new-repository", "private": true}}
	denied(invoke(repoEvent))
	var repoState object
	must(json.Unmarshal(read(filepath.Join(dataRoot, "release-repository.json")), &repoState))
	invoke(object{"hook_event_name": "UserPromptSubmit", "session_id": "release-repository", "turn_id": "two", "prompt": "授权提交"})
	repoEvent["turn_id"] = "two"
	if result := invoke(repoEvent); len(result) != 0 {
		t.Fatal("repository grant did not work", result)
	}
	denied(invoke(repoEvent))
	denied(invoke(object{"hook_event_name": "PreToolUse", "session_id": "release-repository", "turn_id": "two", "cwd": root, "tool_name": "exec_command", "tool_input": object{"cmd": "gh repo create fixture-new-repository --private"}}))
	for _, tc := range []struct {
		source string
		deny   bool
	}{
		{`os.spawnlp(os.P_WAIT,"gh","display","pr","create")`, true},
		{`os.execvp("gh",["display","pr","create"])`, true},
		{`os.posix_spawnp("gh",["display","issue","list"],os.environ)`, false},
		{`subprocess.run(["display","pr","create"],executable="gh")`, true},
		{`subprocess.run(["gh pr create"],shell=True)`, true},
		{`subprocess.run(["gh","issue","list"])`, false},
	} {
		result := invoke(object{"hook_event_name": "PreToolUse", "session_id": "release-python", "turn_id": "one", "cwd": root, "tool_name": "exec_command", "tool_input": object{"cmd": "python3 -c 'import os,subprocess; " + tc.source + "'"}})
		h, _ := result["hookSpecificOutput"].(map[string]any)
		if (h["permissionDecision"] == "deny") != tc.deny || (!tc.deny && len(result) != 0) {
			t.Fatalf("Python package check: %s: want deny=%v, got %v", tc.source, tc.deny, result)
		}
	}
}
