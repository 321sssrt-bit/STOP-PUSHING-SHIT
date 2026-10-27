package host_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type object = map[string]any
type action struct {
	name string
	args object
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string) []byte { b, e := os.ReadFile(path); must(e); return b }
func write(path string, b []byte) {
	must(os.MkdirAll(filepath.Dir(path), 0700))
	must(os.WriteFile(path, b, 0700))
}
func encode(v any) string { b, e := json.Marshal(v); must(e); return string(b) }
func appendMarker(path, value string) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	must(e)
	_, e = fmt.Fprintln(f, value)
	must(e)
	must(f.Close())
}
func absent(path string) bool { _, e := os.Stat(path); return os.IsNotExist(e) }
func quote(s string) string   { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// The test binary also supplies fake tools. It has no GitHub or SSH client.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "fixture-mcp" {
		fakeMCP(os.Args[2])
		os.Exit(0)
	}
	exe, _ := os.Executable()
	switch strings.TrimSuffix(filepath.Base(exe), ".exe") {
	case "gh":
		method := "CREATE"
		for i, a := range os.Args {
			if a == "--method" && i+1 < len(os.Args) {
				method = os.Args[i+1]
			}
		}
		appendMarker(filepath.Join(filepath.Dir(filepath.Dir(exe)), "shell-called"), method)
		fmt.Println("[]")
		os.Exit(0)
	case "ssh":
		if !reflect.DeepEqual(os.Args[1:], []string{"-o", "BatchMode=yes", "fixture-host", "python3 -"}) {
			panic("unexpected fake SSH arguments")
		}
		cmd := exec.Command(os.Getenv("SPS_TEST_PYTHON"), "-")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		must(cmd.Run())
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeMCP(marker string) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var req object
		must(json.Unmarshal(scanner.Bytes(), &req))
		if req["id"] == nil {
			continue
		}
		result := object{}
		switch req["method"] {
		case "initialize":
			result = object{"protocolVersion": "2024-11-05", "capabilities": object{"tools": object{}}, "serverInfo": object{"name": "fixture", "version": "1"}}
		case "tools/list":
			result = object{"tools": []any{
				object{"name": "create_issue", "description": "Local fixture; only writes a temporary marker.", "inputSchema": object{"type": "object", "properties": object{"repository_full_name": object{"type": "string"}, "title": object{"type": "string"}, "body": object{"type": "string"}}, "required": []string{"repository_full_name", "title", "body"}}},
				object{"name": "create_repository", "description": "Local fixture; only writes a temporary marker.", "inputSchema": object{"type": "object", "properties": object{"name": object{"type": "string"}, "private": object{"type": "boolean"}, "description": object{"type": "string"}}, "required": []string{"name", "private"}}},
			}}
		case "tools/call":
			appendMarker(marker, "called")
			args := req["params"].(map[string]any)["arguments"].(map[string]any)
			failed := args["body"] == "fixture failure"
			result = object{"content": []any{object{"type": "text", "text": "Local fixture result."}}, "isError": failed}
		}
		fmt.Println(encode(object{"jsonrpc": "2.0", "id": req["id"], "result": result}))
	}
	must(scanner.Err())
}

type fixtureModel struct {
	sync.Mutex
	actions  []action
	index    int
	requests []object
}

func (f *fixtureModel) set(actions ...action) {
	f.Lock()
	defer f.Unlock()
	f.actions = actions
	f.index = 0
}
func (f *fixtureModel) feedback() string {
	f.Lock()
	defer f.Unlock()
	if len(f.requests) == 0 {
		return ""
	}
	var outputs []any
	for _, item := range f.requests[len(f.requests)-1]["input"].([]any) {
		if m, ok := item.(map[string]any); ok && m["type"] == "function_call_output" {
			outputs = append(outputs, item)
		}
	}
	return encode(outputs)
}
func (f *fixtureModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var request object
	must(json.NewDecoder(r.Body).Decode(&request))
	f.Lock()
	defer f.Unlock()
	f.requests = append(f.requests, request)
	serial := strconv.Itoa(len(f.requests))
	item := object{"id": "msg_" + serial, "type": "message", "status": "completed", "role": "assistant", "content": []any{object{"type": "output_text", "text": "Fixture complete.", "annotations": []any{}}}}
	if f.index < len(f.actions) {
		a := f.actions[f.index]
		item = object{"id": "fc_" + serial, "type": "function_call", "status": "completed", "call_id": "call_" + serial, "name": a.name, "arguments": encode(a.args)}
		if strings.HasPrefix(a.name, "mcp__fixture__") {
			item["namespace"] = "mcp__fixture"
			item["name"] = strings.TrimPrefix(a.name, "mcp__fixture__")
		}
	}
	f.index++
	response := object{"id": "resp_" + serial, "object": "response", "created_at": 1, "status": "completed", "output": []any{item}, "usage": object{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}
	start := object{"id": "resp_" + serial, "object": "response", "status": "in_progress", "output": []any{}}
	events := []object{{"type": "response.created", "response": start}, {"type": "response.output_item.added", "output_index": 0, "item": item}, {"type": "response.output_item.done", "output_index": 0, "item": item}, {"type": "response.completed", "response": response}}
	var body strings.Builder
	for _, event := range events {
		fmt.Fprintf(&body, "event: %s\ndata: %s\n\n", event["type"], encode(event))
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Content-Length", strconv.Itoa(body.Len()))
	_, err := io.WriteString(w, body.String())
	must(err)
}

func TestCodexHost(t *testing.T) {
	if os.Getenv("SPS_HOST_TEST") != "1" {
		t.Skip("set SPS_HOST_TEST=1 for isolated real Codex verification")
	}
	codex := os.Getenv("CODEX_TEST_BINARY")
	if codex == "" {
		var e error
		codex, e = exec.LookPath("codex")
		must(e)
	}
	root, err := filepath.Abs("..")
	must(err)
	work, err := os.MkdirTemp("", "issue-pr-guard-host-")
	must(err)
	t.Cleanup(func() {
		for i := 0; i < 30; i++ {
			if err = os.RemoveAll(work); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Error("temporary directory cleanup:", err)
	})
	attach, closeGroup, err := processGroup()
	must(err)
	t.Cleanup(closeGroup)
	isolated := filepath.Join(work, "codex")
	workspace := filepath.Join(work, "workspace")
	home := filepath.Join(work, "home")
	for _, path := range []string{isolated, workspace, home} {
		must(os.MkdirAll(path, 0700))
	}
	market := filepath.Join(work, "market")
	plugin := filepath.Join(market, "plugin 三思")
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	for _, file := range []string{".codex-plugin/plugin.json", "hooks/hooks.json", "hooks/run.sh", "hooks/run.ps1"} {
		write(filepath.Join(plugin, file), read(filepath.Join(root, "plugins", "codex", file)))
	}
	write(filepath.Join(plugin, "bin", "issue-pr-guard"+suffix), read(filepath.Join(root, "bin", "issue-pr-guard"+suffix)))
	write(filepath.Join(market, ".agents/plugins/marketplace.json"), []byte(encode(object{"name": "issue-pr-guard-local", "plugins": []any{object{"name": "issue-pr-guard", "source": object{"source": "local", "path": "./plugin 三思"}, "policy": object{"installation": "AVAILABLE", "authentication": "ON_INSTALL"}}}})))
	self, err := os.Executable()
	must(err)
	marker := filepath.Join(work, "mcp-called")
	env := []string{}
	for _, key := range []string{"PATH", "TMPDIR", "LANG", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	env = append(env, "CODEX_HOME="+isolated, "HOME="+home, "GIT_TERMINAL_PROMPT=0")
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "AppData/Roaming"), "LOCALAPPDATA="+filepath.Join(home, "AppData/Local"))
	}
	python := ""
	if runtime.GOOS != "windows" {
		python, err = exec.LookPath("python3")
		must(err)
		env = append(env, "SPS_TEST_PYTHON="+python)
	}
	model := &fixtureModel{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	server := &http.Server{Handler: model}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	config := `model = "fixture"
model_provider = "fixture"
cli_auth_credentials_store = "file"
web_search = "disabled"
[features]
apps = false
remote_plugin = false
shell_snapshot = false
memories = false
daemon_auto_start = false
enable_request_compression = false
[model_providers.fixture]
name = "Local deterministic fixture"
wire_api = "responses"
requires_openai_auth = false
request_max_retries = 0
stream_max_retries = 0
supports_websockets = false
`
	config += "base_url = " + strconv.Quote("http://"+listener.Addr().String()+"/v1") + "\n[mcp_servers.fixture]\ncommand = " + strconv.Quote(self) + "\nargs = " + encode([]string{"fixture-mcp", marker}) + "\ndefault_tools_approval_mode = \"approve\"\n"
	write(filepath.Join(isolated, "config.toml"), []byte(config))
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		out, err := os.CreateTemp(work, "output-")
		must(err)
		defer os.Remove(out.Name())
		defer out.Close()
		cmd := exec.CommandContext(ctx, codex, args...)
		cmd.Dir = workspace
		cmd.Env = env
		cmd.Stdout = out
		cmd.Stderr = out
		must(cmd.Start())
		if err = attach(cmd); err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			t.Fatal(err)
		}
		err = cmd.Wait()
		_, seekErr := out.Seek(0, 0)
		must(seekErr)
		data, readErr := io.ReadAll(out)
		must(readErr)
		if err != nil {
			t.Fatalf("Codex: %v\n%s", err, data)
		}
		return string(data)
	}
	run("plugin", "marketplace", "add", market)
	run("plugin", "add", "issue-pr-guard@issue-pr-guard-local")
	sandbox := "workspace-write"
	if runtime.GOOS == "windows" {
		sandbox = "danger-full-access"
	}
	common := []string{"--no-daemon", "--dangerously-bypass-hook-trust", "-a", "never", "-s", sandbox, "exec"}
	args := object{"repository_full_name": "fixture/repository", "title": "Fixture 三思 🌙", "body": "Never sent to GitHub"}
	model.set(action{"mcp__fixture__create_issue", args})
	output := run(append(common, "--skip-git-repo-check", "--json", "Local guard fixture: attempt one creation.")...)
	if !absent(marker) {
		t.Fatal("unauthorized creation reached fake MCP", output)
	}
	session := ""
	for _, line := range strings.Split(output, "\n") {
		var event object
		if json.Unmarshal([]byte(line), &event) == nil && event["type"] == "thread.started" {
			session, _ = event["thread_id"].(string)
		}
	}
	if session == "" {
		t.Fatal("no session", output)
	}
	var states []string
	must(filepath.WalkDir(isolated, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && d.Name() == session+".json" && strings.Contains(filepath.ToSlash(path), "/plugins/") {
			states = append(states, path)
		}
		return nil
	}))
	if len(states) != 1 {
		t.Fatal("expected one plugin state", states, output)
	}
	pending := func() string {
		var s object
		must(json.Unmarshal(read(states[0]), &s))
		id, _ := s["request_id"].(string)
		if len(id) != 32 {
			t.Fatal("missing request id", s)
		}
		return id
	}
	pending()
	if feedback := model.feedback(); !strings.Contains(feedback, "授权提交") || !strings.Contains(feedback, args["body"].(string)) {
		t.Fatal("missing parameter preview", feedback)
	}
	resume := func(prompt string) string {
		return run(append(common, "resume", "--skip-git-repo-check", "--json", session, prompt)...)
	}
	model.set(action{"mcp__fixture__create_issue", args}, action{"mcp__fixture__create_issue", args})
	resume("授权提交")
	if string(read(marker)) != "called\n" {
		t.Fatal("grant wasn't consumed once")
	}
	failedArgs := object{"repository_full_name": "fixture/repository", "title": "Failure fixture", "body": "fixture failure"}
	model.set(action{"mcp__fixture__create_issue", failedArgs})
	resume("Prepare local failure fixture.")
	pending()
	model.set(action{"mcp__fixture__create_issue", failedArgs}, action{"mcp__fixture__create_issue", failedArgs})
	resume("授权提交")
	if string(read(marker)) != "called\ncalled\n" {
		t.Fatal("failed call reused a grant")
	}
	result := object{"host": strings.TrimSpace(run("--version")), "platform": runtime.GOOS + "/" + runtime.GOARCH, "isolated_plugin_install": "passed", "mcp_without_grant": "blocked", "mcp_one_grant_two_calls": "executed exactly once", "mcp_failed_call_retry": "blocked after one execution", "github_creation_requests": 0, "real_ssh_requests": 0, "model_inference": 0, "harness": "Go"}
	repositoryArgs := object{"name": "fixture-new-repository", "private": true, "description": "Never sent to GitHub"}
	model.set(action{"mcp__fixture__create_repository", repositoryArgs})
	resume("Prepare local repository fixture.")
	if string(read(marker)) != "called\ncalled\n" {
		t.Fatal("unauthorized repository creation reached fake MCP")
	}
	pending()
	if feedback := model.feedback(); !strings.Contains(feedback, "private") || !strings.Contains(feedback, repositoryArgs["name"].(string)) {
		t.Fatal("missing repository parameter preview", feedback)
	}
	model.set(action{"mcp__fixture__create_repository", repositoryArgs}, action{"mcp__fixture__create_repository", repositoryArgs})
	resume("授权提交")
	if string(read(marker)) != "called\ncalled\ncalled\n" {
		t.Fatal("repository grant wasn't consumed once")
	}
	result["repository_without_grant"] = "blocked"
	result["repository_one_grant_two_calls"] = "executed exactly once"
	shellMarker := filepath.Join(workspace, "shell-called")
	checkShell := func(command string, denied bool) {
		model.set(action{"exec_command", object{"cmd": command, "login": false}})
		resume("Local command fixture.")
		if denied {
			if !absent(shellMarker) || !strings.Contains(model.feedback(), "已阻止通过命令行/API") {
				t.Fatal("expected hook denial", model.feedback())
			}
		} else if absent(shellMarker) {
			t.Fatal("read didn't execute", model.feedback())
		}
	}
	control := func(command string) {
		cmd := exec.Command("/bin/sh", "-c", command)
		if runtime.GOOS == "windows" {
			cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
		}
		cmd.Dir = workspace
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("control: %v %s", err, out)
		}
	}
	if runtime.GOOS == "windows" {
		path := "'" + strings.ReplaceAll(shellMarker, "'", "''") + "'"
		gh := "function gh { Add-Content -LiteralPath " + path + " -Value CREATE }; gh pr create --title Fixture"
		ghNew := strings.Replace(gh, "; gh pr create", "; gh pr new", 1)
		rest := "function Invoke-RestMethod { param([string]$Method='GET',[string]$Uri,[string]$Body); Add-Content -LiteralPath " + path + " -Value $Method }; Invoke-RestMethod -Uri https://fixture.invalid/repos/fixture/repository/issues -Method "
		for _, s := range []string{gh, ghNew, rest + "POST -Body '{}'", rest + "GET"} {
			control(s)
		}
		if strings.TrimSpace(strings.ReplaceAll(string(read(shellMarker)), "\r", "")) != "CREATE\nCREATE\nPOST\nGET" {
			t.Fatal("Windows control didn't run")
		}
		must(os.Remove(shellMarker))
		checkShell(gh, true)
		checkShell(ghNew, true)
		fakeGH := filepath.Join(workspace, "fixture-bin", "gh.exe")
		write(fakeGH, read(self))
		env = append(env, "SPS_TEST_CMD="+fakeGH)
		for _, s := range []string{
			`powershell.exe -NoProfile -NonInteractive -Command "` + gh + `" ignored`,
			`cmd.exe /d /c '"` + fakeGH + `" pr create' ignored`,
			`cmd.exe /d /k '"` + fakeGH + `" pr create & exit'`,
			`cmd.exe /d /c '%SPS_TEST_CMD% pr create'`,
			`cmd.exe /d /v:on /c '!SPS_TEST_CMD! pr create'`,
			`powershell.exe -NoProfile -NonInteractive -Command '& $env:SPS_TEST_CMD pr create'`,
		} {
			control(s)
			if strings.TrimSpace(string(read(shellMarker))) != "CREATE" {
				t.Fatal("Windows dynamic control didn't run")
			}
			must(os.Remove(shellMarker))
			checkShell(s, true)
		}
		checkShell(rest+"POST -Body '{}'", true)
		checkShell(rest+"GET", false)
		result["powershell_rest_creation"] = "blocked"
		result["powershell_rest_read"] = "GET executed locally"
		result["windows_dynamic_regressions"] = "6 controls executed; hook blocked each"
	} else {
		bin := filepath.Join(workspace, "fixture-bin")
		for _, name := range []string{"gh", "ssh"} {
			write(filepath.Join(bin, name), read(self))
		}
		fakeGH, fakeSSH := filepath.Join(bin, "gh"), filepath.Join(bin, "ssh")
		nested := func(create bool) string {
			script := "import subprocess\nrepo='fixture/repository'\ngh=" + strconv.Quote(fakeGH) + "\nsubprocess.run([gh,'api','--method','GET','repos/'+repo+'/pulls'],check=True)\n"
			if create {
				script += "subprocess.run([gh,'api','--method','POST','repos/'+repo+'/pulls','--input','fixture.json'],check=True)\n"
			}
			outer := "import subprocess\nscript=" + strconv.Quote(script) + "\nsubprocess.run([" + strconv.Quote(fakeSSH) + ",'-o','BatchMode=yes','fixture-host','python3 -'],input=script,text=True,check=True)\n"
			return quote(python) + " - <<'PY'\n" + outer + "PY\n"
		}
		control(nested(true))
		if string(read(shellMarker)) != "GET\nPOST\n" {
			t.Fatal("nested control didn't run")
		}
		must(os.Remove(shellMarker))
		checkShell(quote(fakeGH)+" pr create --title Fixture", true)
		for _, s := range []string{
			quote(fakeGH) + " repo create fixture-new-repository --private",
			quote(fakeGH) + " repo --help=false create fixture-new-repository --private",
			quote(fakeGH) + " repo fork fixture/repository --clone=false",
			"true # 准备完成\n" + quote(fakeGH) + " pr create --title Fixture",
			"cat <<'EOF'; " + quote(fakeGH) + " pr create --title Fixture\nlocal draft\nEOF\n",
			quote(fakeGH) + " pr new --title Fixture",
			quote(fakeGH) + " --repo=fixture/repository pr create",
			"env -i " + quote(fakeGH) + " pr create",
			"command -- " + quote(fakeGH) + " pr create",
			"bash --norc -c " + quote(quote(fakeGH)+" pr create"),
			"eval " + quote(quote(fakeGH)+" pr create"),
			"printf '%s\\n' " + quote(quote(fakeGH)+" pr create") + " | sh",
			"sh <<EOF\nnote=$PATH\n" + quote(fakeGH) + " pr create\nEOF\n",
			quote(python) + " -c " + quote("import subprocess; a=b="+strconv.Quote(fakeGH)+"; subprocess.run([a,'pr','create'])"),
			"bash -c '$0 $1 $2' " + quote(fakeGH) + " pr create",
			"sh -c '$0 $1 $2' " + quote(fakeGH) + " pr create",
			quote(python) + " -c " + quote("import subprocess,sys; subprocess.run(sys.argv[1:],check=True)") + " " + quote(fakeGH) + " pr create",
			"CMD=" + quote(quote(fakeGH)+" pr create") + " " + quote(python) + " -c " + quote(`import os,subprocess; subprocess.run(os.environ["CMD"],shell=True,check=True)`),
			quote(python) + " -c " + quote("import os; os.spawnlp(os.P_WAIT,"+strconv.Quote(fakeGH)+",'display','pr','create')"),
			quote(python) + " -c " + quote("import os; os.spawnlpe(os.P_WAIT,"+strconv.Quote(fakeGH)+",'display','pr','create',os.environ)"),
			quote(python) + " -c " + quote("import os; os.execvp(file="+strconv.Quote(fakeGH)+",args=['display','pr','create'])"),
			quote(python) + " -c " + quote("import os; pid=os.posix_spawnp("+strconv.Quote(fakeGH)+",['display','pr','create'],os.environ); os.waitpid(pid,0)"),
			quote(python) + " -c " + quote("import os; os.execlp(\n# comment\n"+strconv.Quote(fakeGH)+",'display','pr','create')"),
			quote(python) + " -c " + quote("import subprocess; subprocess.run(['display','pr','create'],executable="+strconv.Quote(fakeGH)+",check=True)"),
			quote(python) + " -c " + quote("import subprocess; subprocess.run(["+strconv.Quote(quote(fakeGH)+" pr create")+"],shell=True,check=True)"),
		} {
			control(s)
			if string(read(shellMarker)) != "CREATE\n" {
				t.Fatal("regression control didn't run")
			}
			must(os.Remove(shellMarker))
			checkShell(s, true)
		}
		checkShell(nested(true), true)
		checkShell(nested(false), false)
		if string(read(shellMarker)) != "GET\n" {
			t.Fatal("nested read didn't run")
		}
		result["comment_newline_regression"] = "control executed; hook blocked"
		result["repository_shell_creations"] = "3 controls executed; hook blocked each"
		result["heredoc_sibling_regression"] = "control executed; hook blocked"
		result["python_ssh_api_creation"] = "blocked before execution"
		result["python_ssh_api_read"] = "GET executed locally"
		result["nested_fixture_without_hook"] = "GET then POST executed locally"
		result["positional_and_python_dynamic_regressions"] = "4 controls executed; hook blocked each"
		result["python_execution_regressions"] = "7 controls executed; hook blocked each"
	}
	result["shell_creation"] = "blocked"
	data, err := json.MarshalIndent(result, "", "  ")
	must(err)
	t.Log(string(data))
	if out := os.Getenv("SPS_HOST_RESULT"); out != "" {
		write(out, append(data, '\n'))
	}
}

func TestFakeToolModes(t *testing.T) {
	// Exercise the fixture protocol without Codex; no network server is needed.
	self, err := os.Executable()
	must(err)
	marker := filepath.Join(t.TempDir(), "marker")
	cmd := exec.Command(self, "fixture-mcp", marker)
	cmd.Stdin = bytes.NewBufferString(encode(object{"id": 1, "method": "tools/call", "params": object{"arguments": object{"body": "fixture failure"}}}) + "\n")
	output, err := cmd.Output()
	must(err)
	if !strings.Contains(string(output), `"isError":true`) || string(read(marker)) != "called\n" {
		t.Fatal(string(output))
	}
}
