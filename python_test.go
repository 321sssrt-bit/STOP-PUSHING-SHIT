package main

import (
	"fmt"
	"strings"
	"testing"
)

func checkPythonExecution(t *testing.T, source string, deny bool) {
	t.Helper()
	for _, entry := range []struct {
		name string
		got  bool
	}{
		{"python", pythonCreation(source, 0)},
		{"shell", shellCreation("python3 -c '"+strings.ReplaceAll(source, "'", "'\\''")+"'", 0, "")},
		{"heredoc", shellCreation("python3 - <<'PY'\n"+source+"\nPY\n", 0, "")},
	} {
		if entry.got != deny {
			t.Errorf("%s: deny=%v, want %v\n%s", entry.name, entry.got, deny, source)
		}
	}
}

func TestPythonOSExecution(t *testing.T) {
	for _, name := range []string{
		"spawnl", "spawnle", "spawnlp", "spawnlpe", "spawnv", "spawnve", "spawnvp", "spawnvpe",
		"execl", "execle", "execlp", "execlpe", "execv", "execve", "execvp", "execvpe", "posix_spawn", "posix_spawnp",
	} {
		for _, deny := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/create=%v", name, deny), func(t *testing.T) {
				args := `"display-name", "issue", "list"`
				if deny {
					args = `"display-name", "pr", "create"`
				}
				if strings.Contains(name, "v") || strings.HasPrefix(name, "posix_") {
					args = "[" + args + "]"
				}
				args = `"/tmp/gh", ` + args
				if strings.HasPrefix(name, "spawn") {
					args = "os.P_WAIT, " + args
				}
				if strings.HasSuffix(name, "e") || strings.HasPrefix(name, "posix_") {
					args += ", os.environ"
				}
				checkPythonExecution(t, "import os; os."+name+"("+args+")", deny)
			})
		}
	}
}

func TestPythonExecutionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		deny         bool
	}{
		{"short-execlp", `os.execlp("true", "display")`, false},
		{"short-execle", `os.execle("/bin/true", "display", os.environ)`, false},
		{"display-name-is-not-program", `os.execvp("true",["gh","pr","create"])`, false},
		{"keywords-execvp", `os.execvp(file="gh", args=["display", "issue", "list"])`, false},
		{"keywords-execvpe", `os.execvpe(file="gh", args=["display", "pr", "create"], env=os.environ)`, true},
		{"keywords-execve", `os.execve(path="gh", argv=["display", "issue", "list"], env=os.environ)`, false},
		{"keywords-spawnvpe", `os.spawnvpe(mode=os.P_WAIT, file="gh", args=["display", "issue", "list"], env=os.environ)`, false},
		{"comment-call", "os.execlp(\n# comment\n'gh','display','pr','create')", true},
		{"comment-list", "os.execvp('gh', [\n# comment\n'display','pr','create'])", true},
		{"comment-read", "os.execvp('gh', [\n# comment\n'display','issue','list'])", false},
		{"env-graphql", `os.spawnlpe(os.P_WAIT,"gh","display","api","graphql","-f","query=query { viewer { login } }",os.environ)`, false},
		{"env-ssh", `os.execle("/usr/bin/ssh","display","fixture-host","gh issue list",os.environ)`, false},
		{"env-cmd", `os.spawnle(os.P_WAIT,"cmd.exe","display","/d","/c","gh issue list",os.environ)`, false},
		{"unknown-file", `os.execlp(command,"gh","issue","list")`, true},
		{"unknown-argv", `os.execvp("gh",command)`, true},
		{"missing-argv0", `os.execlp("gh")`, true},
		{"missing-env", `os.execve("gh",["gh","issue","list"])`, true},
		{"extra-arg", `os.execvp("gh",["gh","issue","list"],extra)`, true},
		{"empty-argv", `os.execvp("gh",[])`, true},
		{"starred-args", `os.execlp("gh",*args)`, true},
		{"alias-concat", `from os import execvp as run; run("g"+"h",["display","pr"]+["create"])`, true},
		{"nested-shell", `os.execvp("sh",["display","-c","gh pr create"])`, true},
		{"subprocess-executable", `subprocess.run(["display","pr","create"],executable="gh")`, true},
		{"subprocess-executable-read", `subprocess.run(["display","issue","list"],executable="gh")`, false},
		{"popen-executable", `subprocess.Popen(["display","pr","create"],-1,"gh")`, true},
		{"subprocess-executable-unknown", `subprocess.run(["gh","issue","list"],executable=command)`, true},
		{"subprocess-shell-list", `subprocess.run(["gh pr create"],shell=True)`, true},
		{"subprocess-shell-list-read", `subprocess.run(["gh issue list"],shell=True)`, false},
		{"subprocess-shell-positional", `subprocess.Popen(["gh pr create"],-1,None,None,None,None,None,True,True)`, true},
		{"subprocess-shell-dynamic", `subprocess.run(["gh issue list"],shell=flag)`, true},
		{"subprocess-shell-kwargs", `subprocess.run(["gh issue list"],**options)`, true},
		{"subprocess-shell-false", `subprocess.run(["gh","issue","list"],shell=False)`, false},
		{"subprocess-shell-zero", `subprocess.run(["gh","pr","create"],shell=0)`, true},
		{"subprocess-shell-string-zero", `subprocess.run(["gh pr create"],shell="0")`, true},
		{"subprocess-executable-none", `subprocess.run(["gh","issue","list"],executable=None)`, false},
		{"system-command-read", `os.system(command="gh issue list")`, false},
		{"system-cmd-read", `os.system(cmd="gh issue list")`, false},
		{"system-cmd-create", `os.system(cmd="gh pr create")`, true},
	} {
		t.Run(tc.name, func(t *testing.T) { checkPythonExecution(t, tc.source, tc.deny) })
	}
}
