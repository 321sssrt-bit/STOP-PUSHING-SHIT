package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNodeCommands(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "")
	for _, command := range []string{
		`node -e "require('child_process').spawnSync('git',['push','origin','main'])"`,
		`node -p "require('node:child_process').execSync('git push origin main')"`,
		`node --eval="require('child_process').execFileSync('gh',['pr','create'])"`,
		`node -ep "require('child_process').fork('submit.js')"`,
		`node -e 'const {spawnSync: run} = require("child_process"); run("git", ["push", "origin", "main"])'`,
		`node -e 'import("node:child_process").then(m => m.execSync("git push origin main"))'`,
		`node -e 'process.getBuiltinModule("child_process").execSync("git push origin main")'`,
		"node <<'JS'\nrequire('child_process').execSync('git push origin main')\nJS\n",
		`node -e 'const cp = await import("node:child_process"); cp.execSync("git", ["push", "origin", "main"])'`,
	} {
		t.Run(command, func(t *testing.T) {
			if !shellCreation(command, 0, "") {
				t.Fatal("uninspected Node execution was allowed")
			}
		})
	}
	for _, command := range []string{
		`node --version`, `node -v`, `node --help`,
		`node -e 'console.log(1+2)'`, `node -p '[1,2,3].length'`,
		`command node -e 'console.log(1+2)'`, `cd /tmp && node --version`,
		`sh -c "node -e 'console.log(1)'"`,
		`node --test`, `node ./ordinary-script.js`,
		`node -e 'console.log(require("fs").readFileSync("go.mod", "utf8"))'`,
		`node -e 'console.log("safe"); unknown()'`,
		`node -e 'require("child" + "_process")["spawn" + "Sync"]("git", ["push", "origin", "main"])'`,
		`node -e 'fetch("https://gitee.com/api/v5/repos/a/b/contents/x", {method:"POST"})'`,
		`node --print="require('child_process').spawn('git',['commit','-am','change'])"`,
		`node -pe "require('child_process').exec('git commit --amend --no-edit')"`,
		`node -e 'const cp = require("child_process"); cp.execSync("git", ["commit", "-m", "test"])'`,
		`node -e 'const cp = require("node:child_process"); cp.execSync("git", ["commit", "-m", "test"])'`,
		`node -e 'const cp = require("unknown"); cp.execSync("git", ["commit", "-m", "test"])'`,
		`node --require ./preload.cjs -e 'console.log(1)'`,
		`node -e 'console.log(1)' --import ./preload.mjs`,
		`node --env-file=.env -e 'console.log(1)'`,
		`NODE_OPTIONS=--max-old-space-size=4096 git status --short`,
		`f() { NODE_OPTIONS=--trace-warnings; }; printf ok`,
		`NODE_OPTIONS='--require=./preload.cjs' node -e 'console.log(1)'`,
		`export "$OPTIONS"; node -e 'console.log(1)'`,
		`env NODE_OPTIONS='--require=./preload.cjs' node -e 'console.log(1)'`,
		"node - <<'JS'\nconsole.log(1+2)\nJS\n",
		"node <<'JS'\nconsole.log(1+2)\nJS\n",
	} {
		t.Run(command, func(t *testing.T) {
			if shellCreation(command, 0, "") {
				t.Fatal("ordinary Node execution was blocked")
			}
		})
	}
}

func TestNodeInheritedPreload(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "--require=./preload.cjs")
	if !shellCreation(`node -e 'console.log(1)'`, 0, "") {
		t.Fatal("inherited preload was ignored")
	}
}

func TestNodeCannotConsumeAuthorization(t *testing.T) {
	e := grant(t, setup(t))
	e.Tool = "exec_command"
	e.Input = map[string]any{"cmd": `node -e "require('child_process').spawnSync('git',['push','origin','main'])"`}
	blocked(t, call(t, e))
	if state(t, e) != (State{}) {
		t.Fatal("Node execution retained an unrelated grant or became authorizable")
	}
	e.Turn = "turn-3"
	prompt(t, e, "授权提交")
	blocked(t, call(t, e))
}

func TestNodeRealExecutionGate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake-git fixture; Windows recognition is covered separately")
	}
	node := os.Getenv("SPS_NODE_BINARY")
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			t.Skip("Node executable unavailable")
		}
	}
	e := setup(t)
	t.Setenv("NODE_OPTIONS", "")
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	fake := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SPS_NODE_MARKER\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	preload := filepath.Join(dir, "submit.cjs")
	code := `require('child_process').spawnSync('git', ['push', 'git@gitee.com:example/project.git', 'main'])`
	if err := os.WriteFile(preload, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+filepath.Dir(node)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SPS_NODE_MARKER", marker)
	for _, source := range []string{
		`node -e "` + code + `"`,
		`node -e "const {execSync: run} = require('node:child_process'); run('git push origin main')"`,
		"node - <<'JS'\n" + code + "\nJS\n",
	} {
		t.Run(source, func(t *testing.T) {
			if out, err := exec.Command("/bin/sh", "-c", source).CombinedOutput(); err != nil {
				t.Fatalf("control execution: %v %s", err, out)
			}
			if data, err := os.ReadFile(marker); err != nil || !strings.Contains(string(data), "push") {
				t.Fatalf("control did not execute fake Git: %q %v", data, err)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			e.Cwd, e.Tool, e.Input = dir, "exec_command", map[string]any{"cmd": source}
			payload, _ := json.Marshal(e)
			out, status := process(payload)
			if status != 0 {
				t.Fatalf("guard process failed: %d %s", status, out)
			}
			var result map[string]any
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			if len(result) == 0 {
				if out, err := exec.Command("/bin/sh", "-c", source).CombinedOutput(); err != nil {
					t.Fatalf("allowed execution: %v %s", err, out)
				}
			}
			blocked(t, result)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("fake Git ran past the guard: %v", err)
			}
		})
	}
	const safe = `node -e 'console.log(1+2)'`
	e.Input = map[string]any{"cmd": safe}
	if len(call(t, e)) != 0 {
		t.Fatal("safe print was blocked")
	}
	if out, err := exec.Command("/bin/sh", "-c", safe).CombinedOutput(); err != nil || string(out) != "3\n" {
		t.Fatalf("safe output: %q %v", out, err)
	}
}
