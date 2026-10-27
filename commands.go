package main

import (
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

var mutationPattern = regexp.MustCompile(`\b(?:create(?:Issue|PullRequest|Repository)|cloneTemplateRepository)\b`)
var endpointPattern = regexp.MustCompile(`(?:^|/)(?:repos/[^/]+/[^/]+/(?:issues|pulls|forks|generate)|user/repos|orgs/[^/]+/repos)$`)
var pythonPattern = regexp.MustCompile(`^python(?:\d+(?:\.\d+)*)?$`)
var assignmentPattern = regexp.MustCompile(`^[A-Za-z_]\w*=`)

// Keep tilde paths literal; the library otherwise falls back to OS user lookup.
var staticEnvironment = expand.FuncEnviron(func(name string) string {
	if user, ok := strings.CutPrefix(name, "HOME "); ok {
		return "~" + user
	}
	return ""
})

func text(v any) string { s, _ := v.(string); return s }

func graphqlEndpoint(endpoint any) bool {
	u, err := url.Parse(text(endpoint))
	return err == nil && path.Base(strings.TrimRight(u.Path, "/")) == "graphql"
}

func graphqlCreation(payload any) bool {
	switch v := payload.(type) {
	case string:
		s := strings.TrimPrefix(v, "query=")
		return strings.HasPrefix(s, "@") || mutationPattern.MatchString(s)
	case map[string]any:
		return graphqlCreation(v["query"])
	default:
		return true
	}
}

func restCreation(endpoint any, method string, github bool) bool {
	if slices.Contains([]string{"GET", "HEAD", "OPTIONS"}, method) {
		return false
	}
	s, ok := endpoint.(string)
	if !ok || strings.Contains(s, "$") {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return true
	}
	p := strings.TrimRight(u.Path, "/")
	return endpointPattern.MatchString(p) || (github && p == "")
}

func apiCreation(args []any, curl bool) bool {
	method := "GET"
	explicit := false
	var endpoints, data []any
	dataFlags := []string{"--field", "--raw-field", "--input", "-f", "-F"}
	shortData := []string{"-f", "-F"}
	if curl {
		dataFlags = []string{"--data", "--data-raw", "--data-binary", "--data-urlencode", "--json", "--form", "-F", "-d"}
		shortData = []string{"-F", "-d"}
	}
	for i := 0; i < len(args); i++ {
		s, ok := args[i].(string)
		if !ok {
			endpoints = append(endpoints, nil)
			continue
		}
		flag, v, equal := strings.Cut(s, "=")
		value := func() any {
			if equal {
				return v
			}
			i++
			if i < len(args) {
				return args[i]
			}
			return nil
		}
		switch {
		case slices.Contains([]string{"--method", "--request", "-X"}, flag):
			method, explicit = strings.ToUpper(text(value())), true
		case strings.HasPrefix(s, "-X") && len(s) > 2:
			method, explicit = strings.ToUpper(s[2:]), true
		case slices.Contains(dataFlags, flag):
			v := value()
			if flag == "--input" {
				v = nil
			}
			data = append(data, v)
		case len(s) > 2 && slices.Contains(shortData, s[:2]):
			data = append(data, s[2:])
		case curl && flag == "--url":
			endpoints = append(endpoints, value())
		case slices.Contains([]string{"--header", "-H", "--jq", "-q", "--template", "--hostname", "--output", "-o", "--user", "-u"}, flag):
			value()
		case curl && slices.Contains([]string{"--get", "-G", "--head", "-I"}, s):
			method, explicit = "GET", true
		case !strings.HasPrefix(s, "-"):
			endpoints = append(endpoints, s)
		}
	}
	if !explicit && len(data) > 0 {
		method = "POST"
	}
	if len(endpoints) == 0 {
		endpoints = []any{nil}
	}
	for _, endpoint := range endpoints {
		if graphqlEndpoint(endpoint) {
			for _, payload := range data {
				if graphqlCreation(payload) {
					return true
				}
			}
		} else if restCreation(endpoint, method, !curl) {
			return true
		}
	}
	return false
}

func ghRepositoryCreation(args []any, fork bool) bool {
	valueFlags := []string{"--description", "-d", "--homepage", "-h", "--gitignore", "-g", "--license", "-l", "--remote", "-r", "--source", "-s", "--team", "-t", "--template", "-p"}
	switches := []string{"--add-readme", "--clone", "-c", "--disable-issues", "--disable-wiki", "--include-all-branches", "--internal", "--private", "--public", "--push"}
	if fork {
		valueFlags = []string{"--org", "--fork-name", "--remote-name"}
		switches = []string{"--clone", "-c", "--remote", "--default-branch-only"}
	}
	for i := 0; i < len(args); i++ {
		s, ok := args[i].(string)
		if !ok || s == "--" {
			return true
		}
		flag, _, equal := strings.Cut(s, "=")
		if slices.Contains(valueFlags, flag) {
			if !equal {
				i++ // --help used as a flag value is not a help request.
			}
		} else if s == "--help" || (fork && s == "-h") {
			return false
		} else if strings.HasPrefix(s, "-") && !equal && !slices.Contains(switches, s) {
			return true
		}
	}
	return true
}

func ghCreation(args []any) bool {
	selected, creating := false, false
	for i := 0; i < len(args); i++ {
		s, ok := args[i].(string)
		if !ok {
			return true
		}
		flag, _, equal := strings.Cut(s, "=")
		if slices.Contains([]string{"--repo", "-R", "--hostname"}, flag) ||
			(creating && slices.Contains([]string{"--title", "-t", "--body", "-b", "--body-file", "-F", "--head", "-H", "--base", "-B", "--reviewer", "-r", "--assignee", "-a", "--label", "-l", "--milestone", "-m", "--project", "-p", "--template", "-T", "--recover", "--attach", "--type", "--parent", "--blocked-by", "--blocking"}, flag)) {
			if !equal {
				i++ // A value equal to --help is data, not a help request.
				if i == len(args) {
					return true
				}
			}
			continue
		}
		if strings.HasPrefix(s, "-R") && len(s) > 2 {
			continue
		}
		if s == "--help" || s == "-h" {
			return false
		}
		if creating {
			if equal || slices.Contains([]string{"--draft", "-d", "--web", "-w", "--editor", "-e", "--fill", "-f", "--fill-first", "--fill-verbose", "--dry-run", "--no-maintainer-edit"}, s) {
				continue
			}
			return true
		}
		if !selected {
			if s == "api" {
				return apiCreation(args[i+1:], false)
			}
			if s == "repo" && i+1 < len(args) {
				action, ok := args[i+1].(string)
				if action == "create" || action == "new" || action == "fork" {
					return ghRepositoryCreation(args[i+2:], action == "fork")
				}
				return !ok || (strings.HasPrefix(action, "-") && action != "--help" && action != "-h")
			}
			if s != "pr" && s != "issue" {
				return strings.HasPrefix(s, "-") && s != "--version"
			}
			selected = true
		} else {
			if s != "create" && s != "new" {
				return strings.HasPrefix(s, "-")
			}
			creating = true
		}
	}
	return creating
}

// stdin is a known string (empty when absent), or nil for supplied, unknown input.
func argvCreation(argv []any, stdin any, depth int) bool {
	if depth > 8 {
		return true
	}
	for len(argv) > 0 && assignmentPattern.MatchString(text(argv[0])) {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return false
	}
	if _, ok := argv[0].(string); !ok {
		return true
	}
	exe := strings.ToLower(path.Base(strings.ReplaceAll(text(argv[0]), `\`, "/")))
	exe = strings.TrimSuffix(strings.TrimSuffix(exe, ".exe"), ".cmd")
	args := argv[1:]
	switch exe {
	case "env", "command", "exec", "sudo":
		valueFlags, switches := []string{}, []string{}
		switch exe {
		case "env":
			valueFlags, switches = []string{"-u", "--unset", "-C", "--chdir"}, []string{"-i", "--ignore-environment", "-0", "--null"}
		case "command":
			switches = []string{"-p"}
		case "exec":
			valueFlags, switches = []string{"-a"}, []string{"-c", "-l"}
		case "sudo":
			valueFlags, switches = []string{"-u", "--user", "-g", "--group"}, []string{"-n", "-E", "-H", "-k"}
		}
		for len(args) > 0 {
			s, ok := args[0].(string)
			if !ok {
				return true
			}
			if s == "--" {
				args = args[1:]
				break
			}
			if assignmentPattern.MatchString(s) {
				args = args[1:]
				continue
			}
			if !strings.HasPrefix(s, "-") {
				break
			}
			if exe == "command" && (s == "-v" || s == "-V") {
				return false
			}
			flag, _, equal := strings.Cut(s, "=")
			args = args[1:]
			if slices.Contains(valueFlags, flag) {
				if !equal {
					if len(args) == 0 {
						return true
					}
					args = args[1:]
				}
			} else if !slices.Contains(switches, s) {
				return true
			}
		}
		return argvCreation(args, stdin, depth+1)
	case "gh":
		return ghCreation(args)
	case "git":
		return gitPushCommand(args)
	case "eval":
		var source []string
		for _, arg := range args {
			s, ok := arg.(string)
			if !ok {
				return true
			}
			source = append(source, s)
		}
		return shellCreation(strings.Join(source, " "), depth+1, stdin)
	case "curl":
		for _, a := range args {
			if strings.HasPrefix(strings.ToLower(text(a)), "-method") || strings.HasPrefix(strings.ToLower(text(a)), "-uri") {
				return powershellAPI(args)
			}
		}
		return apiCreation(args, true)
	case "invoke-restmethod", "invoke-webrequest", "irm", "iwr":
		return powershellAPI(args)
	case "powershell", "pwsh", "cmd":
		for i, a := range args {
			if slices.Contains([]string{"-command", "-c", "/c", "/k"}, strings.ToLower(text(a))) {
				if i+2 != len(args) {
					return true
				}
				command, ok := args[i+1].(string)
				return !ok || windowsCreation(command, depth+1)
			}
		}
	case "sh", "bash", "zsh", "dash":
		for i, a := range args {
			if s := text(a); strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "--") && strings.Contains(s, "c") {
				return i+1 == len(args) || args[i+1] == nil || shellCreation(text(args[i+1]), depth+1, stdin)
			}
		}
		return stdin == nil || shellCreation(text(stdin), depth+1, "")
	case "ssh":
		i := 0
		for i < len(args) && strings.HasPrefix(text(args[i]), "-") {
			option := text(args[i])
			i++
			if slices.Contains([]string{"-o", "-i", "-p", "-l", "-F", "-J", "-S", "-b", "-c"}, option) {
				i++
			}
		}
		var remote []string
		for j := i + 1; j < len(args); j++ {
			s, ok := args[j].(string)
			if !ok {
				return true
			}
			remote = append(remote, s)
		}
		return len(remote) > 0 && shellCreation(strings.Join(remote, " "), depth+1, stdin)
	default:
		if exe == "py" || pythonPattern.MatchString(exe) {
			for i, a := range args {
				if a == "-c" {
					return i+1 == len(args) || args[i+1] == nil || pythonCreation(text(args[i+1]), depth+1)
				}
				if a == "-" {
					return stdin == nil || pythonCreation(text(stdin), depth+1)
				}
			}
		}
	}
	return false
}

func literal(word *syntax.Word, argument bool) any {
	if word == nil {
		return nil
	}
	known := true
	syntax.Walk(word, func(n syntax.Node) bool {
		switch n.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst, *syntax.ExtGlob:
			known = false
		}
		return known
	})
	if !known {
		return nil
	}
	if argument {
		fields, err := expand.Fields(&expand.Config{Env: staticEnvironment}, word)
		if err != nil || len(fields) != 1 {
			return nil
		}
		return fields[0]
	}
	s, err := expand.Literal(&expand.Config{Env: staticEnvironment}, word)
	if err != nil {
		return nil
	}
	return s
}

func shellCreation(source string, depth int, stdin any) bool {
	if depth > 8 || len(source) > 262144 {
		return true
	}
	if windowsPath.MatchString(source) {
		return windowsCreation(source, depth)
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(source), "")
	if err != nil {
		if windowsSyntax.MatchString(source) {
			return windowsCreation(source, depth)
		}
		return true
	}
	return shellNodeCreation(file, depth, stdin)
}

func shellNodeCreation(node syntax.Node, depth int, stdin any) bool {
	if depth > 8 {
		return true
	}
	blocked := false
	syntax.Walk(node, func(n syntax.Node) bool {
		if blocked {
			return false
		}
		if pipe, ok := n.(*syntax.BinaryCmd); ok && (pipe.Op == syntax.Pipe || pipe.Op == syntax.PipeAll) {
			blocked = shellNodeCreation(pipe.X, depth+1, stdin) || shellNodeCreation(pipe.Y, depth+1, nil)
			return false
		}
		stmt, ok := n.(*syntax.Stmt)
		if !ok {
			return true
		}
		input := stdin
		for _, r := range stmt.Redirs {
			if r.N != nil && r.N.Value != "0" {
				continue
			}
			if r.Hdoc != nil {
				input = literal(r.Hdoc, false)
			}
			if r.Op == syntax.WordHdoc {
				input = literal(r.Word, false)
			}
			if r.Op == syntax.RdrIn || r.Op == syntax.RdrInOut || r.Op == syntax.DplIn {
				input = nil
			}
		}
		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok {
			if stmt.Cmd != nil && len(stmt.Redirs) > 0 {
				blocked = shellNodeCreation(stmt.Cmd, depth+1, input)
				for _, r := range stmt.Redirs {
					blocked = blocked || shellNodeCreation(r, depth+1, stdin)
				}
				return false
			}
			return true
		}
		args := make([]any, len(call.Args))
		for i, w := range call.Args {
			args[i] = literal(w, true)
		}
		blocked = argvCreation(args, input, depth)
		return !blocked
	})
	return blocked
}
