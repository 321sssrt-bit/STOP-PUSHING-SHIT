package main

import (
	"runtime"
	"slices"
	"strconv"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

func walkPython(node *ts.Node, visit func(*ts.Node) bool, depth int) bool {
	if node == nil {
		return false
	}
	if depth > 256 || visit(node) {
		return true
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if walkPython(node.NamedChild(i), visit, depth+1) {
			return true
		}
	}
	return false
}

type pythonSource struct {
	code         []byte
	bindings     map[string][]*ts.Node
	imports      map[string]string
	literalError bool
}

type pythonNone struct{}
type pythonInteger string

func (p pythonSource) source(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(p.code)
}

func (p pythonSource) function(n *ts.Node) string {
	if n == nil {
		return ""
	}
	if n.Kind() == "identifier" {
		s := p.source(n)
		if alias, ok := p.imports[s]; ok {
			return alias
		}
		return s
	}
	if n.Kind() == "attribute" {
		return p.function(n.ChildByFieldName("object")) + "." + p.source(n.ChildByFieldName("attribute"))
	}
	return ""
}

func (p *pythonSource) value(n *ts.Node, depth int) any {
	if n == nil || depth > 32 {
		return nil
	}
	switch n.Kind() {
	case "none":
		return pythonNone{}
	case "true", "false":
		return n.Kind() == "true"
	case "identifier":
		values := p.bindings[p.source(n)]
		if len(values) > 1 {
			p.literalError = true // Do not guess which assignment reaches an execution call.
		} else if len(values) == 1 {
			return p.value(values[0], depth+1)
		}
	case "assignment":
		return p.value(n.ChildByFieldName("right"), depth+1)
	case "augmented_assignment":
		p.literalError = true
	case "integer":
		return pythonInteger(p.source(n))
	case "dictionary":
		values := map[string]any{}
		for i := uint(0); i < n.NamedChildCount(); i++ {
			pair := n.NamedChild(i)
			if pair.Kind() == "comment" {
				continue
			}
			if pair.Kind() != "pair" {
				return nil
			}
			key, ok := p.value(pair.ChildByFieldName("key"), depth+1).(string)
			if !ok {
				return nil
			}
			values[key] = p.value(pair.ChildByFieldName("value"), depth+1)
		}
		return values
	case "list", "tuple":
		values := []any{}
		for i := uint(0); i < n.NamedChildCount(); i++ {
			if child := n.NamedChild(i); child.Kind() != "comment" {
				values = append(values, p.value(child, depth+1))
			}
		}
		return values
	case "parenthesized_expression":
		return p.value(n.NamedChild(0), depth+1)
	case "binary_operator":
		if p.source(n.ChildByFieldName("operator")) != "+" {
			return nil
		}
		left, right := p.value(n.ChildByFieldName("left"), depth+1), p.value(n.ChildByFieldName("right"), depth+1)
		if l, ok := left.(string); ok {
			if r, ok := right.(string); ok {
				if len(l)+len(r) > 262144 {
					p.literalError = true
					return nil
				}
				return l + r
			}
		}
		if l, ok := left.([]any); ok {
			if r, ok := right.([]any); ok {
				return append(append([]any{}, l...), r...)
			}
		}
	case "concatenated_string":
		var out strings.Builder
		for i := uint(0); i < n.NamedChildCount(); i++ {
			v, ok := p.value(n.NamedChild(i), depth+1).(string)
			if !ok {
				return nil
			}
			out.WriteString(v)
			if out.Len() > 262144 {
				p.literalError = true
				return nil
			}
		}
		return out.String()
	case "string":
		start := p.source(n.NamedChild(0))
		raw := strings.Contains(strings.ToLower(start), "r")
		format := strings.Contains(strings.ToLower(start), "f")
		var out strings.Builder
		for i := uint(1); i+1 < n.NamedChildCount(); i++ {
			part := n.NamedChild(i)
			if part.Kind() == "interpolation" {
				if part.ChildByFieldName("format_specifier") != nil || part.ChildByFieldName("type_conversion") != nil {
					return nil
				}
				value := p.value(part.ChildByFieldName("expression"), depth+1)
				if integer, ok := value.(pythonInteger); ok {
					value = string(integer)
				}
				v, ok := value.(string)
				if !ok {
					return nil
				}
				out.WriteString(v)
			} else {
				s := p.source(part)
				if !raw {
					var ok bool
					s, ok = pythonUnescape(s)
					if !ok {
						p.literalError = true
						return nil
					}
				}
				if format {
					s = strings.ReplaceAll(strings.ReplaceAll(s, "{{", "{"), "}}", "}")
				}
				out.WriteString(s)
			}
			if out.Len() > 262144 {
				p.literalError = true
				return nil
			}
		}
		return out.String()
	}
	return nil
}

// The executable selects the program; argv[0] is only its display name.
func pythonExecCreation(executable any, command any, stdin any, depth int) bool {
	file, ok := executable.(string)
	argv, list := command.([]any)
	if !ok || file == "" || !list || len(argv) == 0 || text(argv[0]) == "" {
		return true
	}
	for _, arg := range argv {
		if _, ok := arg.(string); !ok {
			return true
		}
	}
	return argvCreation(append([]any{file}, argv[1:]...), stdin, depth+1)
}

func pythonOSExecCreation(name string, args []any, kwargs map[string]any, depth int) bool {
	list := strings.HasPrefix(name, "os.spawnl") || strings.HasPrefix(name, "os.execl")
	spawn := strings.HasPrefix(name, "os.spawn")
	posix := strings.HasPrefix(name, "os.posix_spawn")
	hasEnv := strings.HasSuffix(name, "e") || posix
	if list {
		fileIndex, end := 0, len(args)
		if spawn {
			fileIndex = 1
		}
		if hasEnv {
			end-- // The final env mapping is not part of argv.
		}
		if len(kwargs) != 0 || end < fileIndex+2 {
			return true
		}
		return pythonExecCreation(args[fileIndex], args[fileIndex+1:end], "", depth)
	}
	keys := []string{"file", "args"}
	if name == "os.execv" || name == "os.execve" || posix {
		keys = []string{"path", "argv"}
	}
	if spawn {
		keys = append([]string{"mode"}, keys...)
	}
	if hasEnv {
		keys = append(keys, "env")
	}
	if len(args) > len(keys) {
		return true
	}
	values := make([]any, len(keys))
	for i, key := range keys {
		v, provided := kwargs[key]
		if i < len(args) {
			if provided {
				return true
			}
			v = args[i]
		} else if !provided {
			return true
		}
		values[i] = v
	}
	for key := range kwargs {
		if !slices.Contains(keys, key) && !(posix && slices.Contains([]string{"file_actions", "setpgroup", "resetids", "setsid", "setsigmask", "setsigdef", "scheduler"}, key)) {
			return true
		}
	}
	fileIndex := 0
	if spawn {
		fileIndex = 1
	}
	return pythonExecCreation(values[fileIndex], values[fileIndex+1], "", depth)
}

func pythonSubprocessCreation(args []any, kwargs map[string]any, depth int) bool {
	get := func(i int, key string, fallback any) any {
		v, provided := kwargs[key]
		if i < len(args) {
			if provided {
				return nil
			}
			return args[i]
		}
		if provided {
			return v
		}
		return fallback
	}
	command := get(0, "args", nil)
	executable := get(2, "executable", pythonNone{})
	shell := get(8, "shell", false)
	useShell := false
	switch v := shell.(type) {
	case bool:
		useShell = v
	case pythonNone:
	case pythonInteger:
		n, err := strconv.ParseInt(string(v), 0, 64)
		if err != nil {
			return true
		}
		useShell = n != 0
	case string:
		useShell = v != ""
	default:
		return true
	}
	var stdin any = ""
	if input, provided := kwargs["input"]; provided {
		stdin = input
	}
	_, defaultExecutable := executable.(pythonNone)
	if useShell {
		var argv []any
		switch v := command.(type) {
		case string:
			argv = []any{v}
		case []any:
			argv = v
		default:
			return true
		}
		if len(argv) == 0 {
			return true
		}
		var words []string
		for _, arg := range argv {
			word, ok := arg.(string)
			if !ok {
				return true
			}
			words = append(words, word)
		}
		if defaultExecutable {
			// Windows passes a command line, whereas POSIX uses only args[0] as shell code.
			if runtime.GOOS == "windows" && len(words) > 1 && windowsCreation(strings.Join(words, " "), depth+1) {
				return true
			}
			return shellCreation(words[0], depth+1, stdin)
		}
		return pythonExecCreation(executable, append([]any{"shell", "-c"}, argv...), stdin, depth)
	}
	switch v := command.(type) {
	case []any:
		if defaultExecutable {
			if len(v) == 0 {
				return true
			}
			executable = v[0]
		}
		return pythonExecCreation(executable, v, stdin, depth)
	case string:
		if !defaultExecutable {
			if runtime.GOOS == "windows" {
				// shortcut: overridden string command lines need Windows argv decoding to allow reads.
				return true
			}
			return pythonExecCreation(executable, []any{v}, stdin, depth)
		}
		return shellCreation(v, depth+1, stdin)
	default:
		return true
	}
}

func pythonUnescape(s string) (string, bool) {
	var out strings.Builder
	for len(s) > 0 {
		if s[0] != '\\' {
			out.WriteByte(s[0])
			s = s[1:]
			continue
		}
		if strings.HasPrefix(s, "\\\r\n") {
			s = s[3:]
			continue
		}
		if strings.HasPrefix(s, "\\\n") {
			s = s[2:]
			continue
		}
		if strings.HasPrefix(s, `\'`) || strings.HasPrefix(s, `\"`) {
			out.WriteByte(s[1])
			s = s[2:]
			continue
		}
		if len(s) > 1 && s[1] >= '0' && s[1] <= '7' {
			i := 2
			for i < len(s) && i < 4 && s[i] >= '0' && s[i] <= '7' {
				i++
			}
			v, _ := strconv.ParseUint(s[1:i], 8, 32)
			out.WriteRune(rune(v))
			s = s[i:]
			continue
		}
		r, _, tail, err := strconv.UnquoteChar(s, 0)
		if err != nil {
			if len(s) > 1 && !strings.ContainsRune("xuUN", rune(s[1])) {
				out.WriteString(s[:2])
				s = s[2:]
				continue
			}
			return "", false
		}
		out.WriteRune(r)
		s = tail
	}
	return out.String(), true
}

func pythonCreation(source string, depth int) bool {
	if depth > 8 || len(source) > 262144 {
		return true
	}
	parser := ts.NewParser()
	defer parser.Close()
	if parser.SetLanguage(ts.NewLanguage(python.Language())) != nil {
		return true
	}
	parser.SetTimeoutMicros(500000)
	p := pythonSource{code: []byte(source), bindings: make(map[string][]*ts.Node), imports: make(map[string]string)}
	tree := parser.Parse(p.code, nil)
	if tree == nil {
		return true
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return true
	}
	if walkPython(root, func(n *ts.Node) bool {
		switch n.Kind() {
		case "assignment", "augmented_assignment":
			left := n.ChildByFieldName("left")
			if left != nil && left.Kind() == "identifier" {
				p.bindings[p.source(left)] = append(p.bindings[p.source(left)], n)
			}
		case "import_statement", "import_from_statement":
			prefix := ""
			if n.Kind() == "import_from_statement" {
				prefix = p.source(n.ChildByFieldName("module_name")) + "."
			}
			for i := uint(0); i < n.NamedChildCount(); i++ {
				if n.FieldNameForNamedChild(uint32(i)) != "name" {
					continue
				}
				item := n.NamedChild(i)
				name, alias := p.source(item), p.source(item)
				if item.Kind() == "aliased_import" {
					name = p.source(item.ChildByFieldName("name"))
					alias = p.source(item.ChildByFieldName("alias"))
				}
				p.imports[alias] = prefix + name
			}
		}
		return false
	}, 0) {
		return true
	}
	return walkPython(root, func(n *ts.Node) bool {
		if n.Kind() != "call" {
			return false
		}
		name := p.function(n.ChildByFieldName("function"))
		var args []any
		kwargs := map[string]any{}
		p.literalError = false
		list := n.ChildByFieldName("arguments")
		if list == nil {
			return false
		}
		for i := uint(0); i < list.NamedChildCount(); i++ {
			a := list.NamedChild(i)
			if a.Kind() == "comment" {
				continue
			}
			if a.Kind() == "list_splat" || a.Kind() == "dictionary_splat" {
				p.literalError = true
			}
			if a.Kind() == "keyword_argument" {
				key := p.source(a.ChildByFieldName("name"))
				if _, duplicate := kwargs[key]; duplicate {
					p.literalError = true
				}
				kwargs[key] = p.value(a.ChildByFieldName("value"), 0)
			} else {
				args = append(args, p.value(a, 0))
			}
		}
		argument := func(i int, key string) any {
			if i < len(args) {
				return args[i]
			}
			return kwargs[key]
		}
		switch name {
		case "subprocess.run", "subprocess.call", "subprocess.check_call", "subprocess.check_output", "subprocess.Popen":
			return p.literalError || pythonSubprocessCreation(args, kwargs, depth)
		case "os.system", "os.popen":
			if p.literalError {
				return true
			}
			key := "command"
			_, cmdKeyword := kwargs["cmd"]
			if name == "os.popen" || cmdKeyword {
				key = "cmd"
			}
			command := argument(0, key)
			var stdin any = ""
			if input, provided := kwargs["input"]; provided {
				stdin = input
			}
			switch v := command.(type) {
			case string:
				return shellCreation(v, depth+1, stdin)
			case []any:
				return argvCreation(v, stdin, depth+1)
			default:
				return true
			}
		case "os.spawnl", "os.spawnle", "os.spawnlp", "os.spawnlpe", "os.execl", "os.execle", "os.execlp", "os.execlpe",
			"os.spawnv", "os.spawnve", "os.spawnvp", "os.spawnvpe", "os.execv", "os.execve", "os.execvp", "os.execvpe", "os.posix_spawn", "os.posix_spawnp":
			return p.literalError || pythonOSExecCreation(name, args, kwargs, depth)
		}
		if strings.HasPrefix(name, "requests.") || strings.HasPrefix(name, "httpx.") {
			if p.literalError {
				return true
			}
			parts := strings.Split(name, ".")
			method := strings.ToUpper(parts[len(parts)-1])
			url := argument(0, "url")
			dataIndex := 1
			if method == "REQUEST" {
				method = strings.ToUpper(text(argument(0, "method")))
				url = argument(1, "url")
				dataIndex = 2
			}
			switch method {
			case "POST", "PUT", "PATCH", "GET", "HEAD", "":
				if method != "GET" && method != "HEAD" && graphqlEndpoint(url) {
					payload := argument(dataIndex, "data")
					if _, provided := kwargs["data"]; !provided && len(args) <= dataIndex {
						payload = kwargs["json"]
					}
					return graphqlCreation(payload)
				}
				return restCreation(url, method, false)
			}
		}
		if strings.HasSuffix(name, ".graphql") || strings.HasSuffix(name, ".execute_graphql") {
			if p.literalError {
				return true
			}
			for _, a := range args {
				if mutationPattern.MatchString(text(a)) {
					return true
				}
			}
			query, provided := kwargs["query"]
			if !provided {
				query = argument(0, "query")
			}
			return graphqlCreation(query)
		}
		return false
	}, 0)
}
