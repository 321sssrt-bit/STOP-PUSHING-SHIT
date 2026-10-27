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

func TestHistoricalCommands(t *testing.T) {
	data, err := os.ReadFile("tests/fixtures/commands.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Command, File string
		Deny                bool
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.File != "" {
				data, err := os.ReadFile(filepath.Join("tests/fixtures", tc.File))
				if err != nil {
					t.Fatal(err)
				}
				tc.Command = string(data)
			}
			if got := shellCreation(tc.Command, 0, ""); got != tc.Deny {
				t.Fatalf("deny=%v, want %v\n%s", got, tc.Deny, tc.Command)
			}
		})
	}
}

func TestShellRegressions(t *testing.T) {
	for _, source := range []string{
		"true # 准备完成\ngh pr create --title Fixture",
		"cat <<'EOF'; gh pr create --title Fixture\nlocal draft\nEOF\n",
		"cat <<'EOF'\nlocal draft\nEOF\ngh issue create --title Fixture",
		"cat <<'EOF'\ngh pr create\nEOF\ntrue # read only\ngh issue list",
	} {
		t.Run(source, func(t *testing.T) {
			want := !strings.Contains(source, "read only")
			if got := shellCreation(source, 0, ""); got != want {
				t.Fatalf("deny=%v want %v", got, want)
			}
		})
	}
}

func TestSafeShellControls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell controls run on macOS/Linux")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nprintf 'called\\n' >> \"$SPS_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"true # 准备完成\ngh pr create --title Fixture",
		"cat <<'EOF'; gh pr create --title Fixture\nlocal draft\nEOF\n",
		"gh pr new --title Fixture",
		"gh --repo=example/project pr create",
		"command -- gh pr create",
		"bash --norc -c 'gh pr create'",
		"eval 'gh pr create'",
		"printf 'gh pr create\\n' | sh",
		"sh <<EOF\nnote=$SPS_MARKER\ngh pr create\nEOF\n",
	} {
		cmd := exec.Command("/bin/sh", "-c", source)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "SPS_MARKER="+marker)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("control: %v %s", err, out)
		}
		if data, err := os.ReadFile(marker); err != nil || string(data) != "called\n" {
			t.Fatalf("control didn't create marker: %q %v", data, err)
		}
		if !shellCreation(source, 0, "") {
			t.Fatal("executable creation was missed")
		}
		if err := os.Remove(marker); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuditRegressions(t *testing.T) {
	for _, tc := range []struct {
		source string
		deny   bool
	}{
		{`gh pr new --title X`, true},
		{`gh issue new --title X`, true},
		{`gh --repo=example/project pr create`, true},
		{`gh -Rexample/project issue create`, true},
		{`gh pr --repo example/project create`, true},
		{`action=create; gh pr "$action"`, true},
		{`gh pr create --body --help`, true},
		{`gh pr create --help`, false},
		{`gh pr create --title X --help`, false},
		{`gh --repo=example/project issue list`, false},
		{`env -i PATH=/usr/bin gh pr create`, true},
		{`env -u TOKEN gh issue create`, true},
		{`env -i gh issue list`, false},
		{`command -- gh pr create`, true},
		{`command -v gh`, false},
		{`command -p -- gh issue list`, false},
		{`exec -a fixture gh pr create`, true},
		{`sudo -n -u fixture gh pr create`, true},
		{`bash --norc -c 'gh pr create'`, true},
		{`bash --norc -c 'gh issue list'`, false},
		{`powershell.exe -NoProfile -Command "gh pr create" ignored`, true},
		{`powershell.exe -NoProfile -Command "function gh { Write-Output called }; gh pr create" ignored`, true},
		{`pwsh -c 'gh pr create' ignored`, true},
		{`cmd.exe /d /c "gh pr create" ignored`, true},
		{`cmd.exe /d /k "gh pr create"`, true},
		{`cmd.exe /d /c "%CMD%"`, true},
		{`cmd.exe /d /c "%CMD% # comment"`, true},
		{`cmd.exe /d /c "echo ready & %CMD%"`, true},
		{`cmd.exe /d /v:on /c "!CMD!"`, true},
		{`powershell.exe -Command '& $cmd'`, true},
		{`pwsh -Command '@cmd'`, true},
		{`bash -c '$0 $1 $2' gh pr create`, true},
		{`sh -c '$0 $1 $2' gh pr create`, true},
		{`bash -c '"$@"' ignored gh pr create`, true},
		{`$CMD pr create`, true},
		{`$(printf gh) pr create`, true},
		{`bash -c 'gh issue list' ignored`, false},
		{`bash -c 'gh issue list'`, false},
		{`gh issue list`, false},
		{`cmd.exe /d /c "gh issue list"`, false},
		{`cmd.exe /d /k "gh issue list"`, false},
		{`powershell.exe -Command 'Invoke-RestMethod -Method GET -Uri https://api.github.com/repos/a/b/issues'`, false},
		{`powershell.exe -Command 'Write-Output "$cmd"'`, false},
		{`cmd.exe /d /c "echo %CMD%"`, false},
		{`eval 'gh pr create'`, true},
		{`eval 'gh issue list'`, false},
		{`printf 'gh pr create\n' | sh`, true},
		{`printf 'gh pr create\n' | sh -c sh`, true},
		{`printf 'local draft' | cat`, false},
		{"printf x | sh <<'EOF'\ngh issue list\nEOF\n", false},
		{"{ sh; } <<'EOF'\ngh pr create\nEOF\n", true},
		{"{ sh; } <<'EOF'\ngh issue list\nEOF\n", false},
		{"sh <<EOF\nnote=$NOTE\ngh pr create\nEOF\n", true},
		{"python3 - <<PY\nimport subprocess\nnote='$NOTE'\nsubprocess.run(['gh','pr','create'])\nPY\n", true},
		{"cat <<EOF\n$NOTE\ngh pr create\nEOF\n", false},
		{`sh <<< "$SCRIPT"`, true},
		{`sh < script.txt`, true},
		{`printf "import subprocess\nsubprocess.run(['gh','pr','create'])\n" | python3 -`, true},
		{`python3 -c "import subprocess; a=b='gh'; subprocess.run([a,'pr','create'])"`, true},
		{`python3 -c "import subprocess; a=b='gh'; subprocess.run([b,'issue','list'])"`, false},
		{`python3 -c "import subprocess; a=['gh','pr','create']; a+=['--title','X']; subprocess.run(a)"`, true},
		{`python3 -c "import subprocess; a=['gh']; a+=['pr','create']; subprocess.run(a)"`, true},
		{`python3 -c "import subprocess; a='gh'; subprocess.run([a,'pr','create']); a='noop'"`, true},
		{`python3 -c "import subprocess; subprocess.run(['python3','-'],input=get_script())"`, true},
		{`python3 -c 'import subprocess,sys; subprocess.run(sys.argv[1:])' gh pr create`, true},
		{`CMD='gh pr create' python3 -c 'import os,subprocess; subprocess.run(os.environ["CMD"], shell=True)'`, true},
		{`python3 -c 'import subprocess; subprocess.call(command)'`, true},
		{`python3 -c 'import subprocess; subprocess.check_call(args=command)'`, true},
		{`python3 -c 'import subprocess; subprocess.check_output(command)'`, true},
		{`python3 -c 'import subprocess; subprocess.Popen(command)'`, true},
		{`python3 -c 'import os; os.system(command)'`, true},
		{`python3 -c 'import os; os.popen(cmd=command)'`, true},
		{`python3 -c 'import subprocess; subprocess.run([command,"pr","create"])'`, true},
		{`python3 -c 'import subprocess; subprocess.run(["gh","issue","list"])'`, false},
		{`python3 -c "import os; os.popen(cmd='gh pr create')"`, true},
		{`python3 -c "import os; os.popen(cmd='gh issue list')"`, false},
		{`python3 -c 'import os; os.spawnlp(os.P_WAIT,"gh","gh","pr","create")'`, true},
		{`python3 -c 'import os; os.spawnlpe(os.P_WAIT,"gh","gh","pr","create",os.environ)'`, true},
		{`python3 -c 'import os; os.spawnvp(os.P_WAIT,"gh",["gh","pr","create"])'`, true},
		{`python3 -c 'import os; os.execlp("gh","gh","pr","create")'`, true},
		{`python3 -c 'import os; os.execve("gh",["gh","pr","create"],os.environ)'`, true},
		{`python3 -c 'import os; os.execvp("gh",["gh","pr","create"])'`, true},
		{`python3 -c 'import os; os.posix_spawn("/tmp/fake/gh",["gh","pr","create"],os.environ)'`, true},
		{`python3 -c 'import os; os.spawnlp(os.P_WAIT,"gh","gh","issue","list")'`, false},
		{`python3 -c 'import os; os.spawnvp(os.P_WAIT,"gh",["gh","issue","list"])'`, false},
		{`python3 -c 'import os; os.execlp("gh","gh","issue","list")'`, false},
		{`python3 -c 'import os; os.execve("gh",["gh","issue","list"],os.environ)'`, false},
		{`python3 -c 'import os; os.execvp("gh",("gh","issue","list"))'`, false},
		{`python3 -c 'import os; os.posix_spawn("/tmp/fake/gh",["gh","issue","list"],os.environ)'`, false},
		{`python3 -c 'import os; os.spawnvp(os.P_WAIT,command,["gh","pr","create"])'`, true},
		{`python3 -c 'import os; os.execvp("gh",command)'`, true},
		{`python3 -c 'import os; os.posix_spawn(command,["gh","pr","create"],os.environ)'`, true},
		{`gh api graphql -f 'query=query($owner:String!){user(login:$owner){login}}' -F owner=example`, false},
		{`gh api graphql -f "$QUERY"`, true},
		{`python3 -c 'import requests; requests.post("https://api.github.com/graphql",json={"query":"mutation { createIssue(input: {}) { issue { id } } }"})'`, true},
		{`python3 -c 'import httpx; httpx.post("https://api.github.com/graphql",json={"query":"mutation { createPullRequest(input: {}) { pullRequest { id } } }"})'`, true},
		{`python3 -c 'import requests; requests.request("POST","https://api.github.com/graphql",data="mutation { createIssue(input: {}) { issue { id } } }")'`, true},
		{`python3 -c 'import requests; requests.post("https://api.github.com/graphql",json={"query":"query { viewer { login } }"})'`, false},
		{`python3 -c 'import httpx; httpx.post("https://api.github.com/graphql",json=payload)'`, true},
		{`python3 -c 'client.graphql(query="mutation { createIssue(input: {}) { issue { id } } }")'`, true},
		{`python3 -c 'client.graphql("context","mutation { createIssue(input: {}) { issue { id } } }")'`, true},
		{`python3 -c 'client.graphql("context",query="mutation { createIssue(input: {}) { issue { id } } }")'`, true},
		{`python3 -c 'client.graphql(query="query { viewer { login } }")'`, false},
		{`python3 -c 'client.graphql("query { viewer { login } }",{"name":"example"})'`, false},
		{`python3 -c 'client.execute_graphql(query=query)'`, true},
	} {
		t.Run(tc.source, func(t *testing.T) {
			if got := shellCreation(tc.source, 0, ""); got != tc.deny {
				t.Fatalf("deny=%v want %v", got, tc.deny)
			}
		})
	}
}

func TestBoundedCommandAnalysis(t *testing.T) {
	for _, tc := range []struct {
		source string
		deny   bool
	}{
		{`gh api graphql --input request.json`, true},
		{`echo "$(gh pr create --title Fixture)"`, true},
		{`cat <<EOF
$(gh pr create --title Fixture)
EOF`, true},
		{`cat <<'EOF'
$(gh pr create --title Fixture)
EOF`, false},
		{`python3 -c "import subprocess; repo='example/project'; subprocess.run(['gh','api',f'repos/{repo}/pulls','-f','title=X'])"`, true},
		{`python3 -c "import subprocess; repo='example/project'; subprocess.run(['gh','api',f'repos/{repo}/pulls','--method','GET'])"`, false},
		{`python3 -c "from subprocess import run as r; r(args=['gh','pr','create'])"`, true},
		{`python3 -c "import subprocess; subprocess.run(['g'+'h','pr','create'])"`, true},
		{`python3 -c "import os; os.system('gh pr create')"`, true},
		{`python3 -c "import httpx; httpx.request('POST','https://api.github.com/repos/a/b/issues')"`, true},
		{`python3 -c "import requests; requests.get('https://api.github.com/repos/a/b/issues')"`, false},
		{`python3 -c 'not valid python ?'`, true},
		{`gh pr create 'unterminated`, true},
		{`node -e "require('child_process').spawnSync('gh',['pr','create'])"`, false},
		{"python3 - <<'PY'\nimport subprocess\nnote='''`\n'''\nsubprocess.run(['gh','pr','create'])\nPY\n", true},
		{`function publish() { gh p\r create; }; publish`, true},
		{`g\h p\r cr\eate`, true},
		{`~fixture/bin/gh pr create`, true},
		{`~fixture/bin/gh issue list`, false},
		{`py.exe -3 -c "import subprocess; subprocess.run(['C:\Tools\gh.exe','pr','create'])"`, true},
		{`python3 -c 'import subprocess; subprocess.run(["\N{LATIN SMALL LETTER G}h","pr","create"])'`, true},
	} {
		t.Run(tc.source, func(t *testing.T) {
			if got := shellCreation(tc.source, 0, ""); got != tc.deny {
				t.Fatalf("deny=%v want %v", got, tc.deny)
			}
		})
	}
	if !shellCreation(strings.Repeat(" ", 262145), 0, "") || !shellCreation("true", 9, "") {
		t.Fatal("analysis budget must fail closed")
	}
}
