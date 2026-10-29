package main

import (
	"os"
	"slices"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
)

func nodeCommand(args []any, stdin any, depth int) bool {
	// NODE_OPTIONS makes ordinary inline inspection incomplete.
	if os.Getenv("NODE_OPTIONS") != "" && nodeCommandArgsContainCode(args) {
		return true
	}
	if len(args) == 1 && slices.Contains([]string{"--help", "-h", "--version", "-v"}, text(args[0])) {
		return false
	}
	var source any = stdin
	inline := false
	for i := 0; i < len(args); i++ {
		s, ok := args[i].(string)
		if !ok {
			return false
		}
		switch {
		case s == "--no-warnings" || s == "--input-type=module" || s == "--input-type=commonjs":
		case slices.Contains([]string{"-e", "--eval", "-p", "--print", "-pe", "-ep"}, s):
			if inline || i+1 == len(args) {
				return true
			}
			i++
			source, inline = args[i], true
		case strings.HasPrefix(s, "--eval=") || strings.HasPrefix(s, "--print="):
			if inline {
				return true
			}
			_, source, _ = strings.Cut(s, "=")
			inline = true
		case strings.HasPrefix(s, "-e") || strings.HasPrefix(s, "-p"):
			if inline {
				return true
			}
			source, inline = s[2:], true
		case s == "-" && !inline && i+1 == len(args):
		default:
			return false // Uninspected files and options are not themselves protected operations.
		}
	}
	code, ok := source.(string)
	return ok && (inline || strings.TrimSpace(code) != "") && nodeCreation(code)
}

func nodeCommandArgsContainCode(args []any) bool {
	for _, arg := range args {
		s := text(arg)
		if s == "-e" || s == "-p" || s == "-pe" || s == "-ep" || strings.HasPrefix(s, "--eval") || strings.HasPrefix(s, "--print") || strings.HasPrefix(s, "-e") || strings.HasPrefix(s, "-p") {
			return true
		}
	}
	return false
}

func nodeCreation(source string) bool {
	if len(source) > 262144 {
		return true
	}
	parser := ts.NewParser()
	defer parser.Close()
	if parser.SetLanguage(ts.NewLanguage(javascript.Language())) != nil {
		return true
	}
	parser.SetTimeoutMicros(500000)
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		return true
	}
	defer tree.Close()
	p := nodeSource{code: []byte(source), modules: make(map[string]map[string]string)}
	return tree.RootNode().HasError() || p.creation(tree.RootNode(), 0)
}

type nodeSource struct {
	code    []byte
	modules map[string]map[string]string
}

func (p nodeSource) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(p.code)
}

func (p nodeSource) name(n *ts.Node) string {
	if n == nil {
		return ""
	}
	if n.Kind() == "identifier" || n.Kind() == "property_identifier" || n.Kind() == "shorthand_property_identifier" || n.Kind() == "shorthand_property_identifier_pattern" || n.Kind() == "import" {
		return p.text(n)
	}
	if n.Kind() == "member_expression" && n.ChildByFieldName("optional_chain") == nil {
		return p.name(n.ChildByFieldName("object")) + "." + p.name(n.ChildByFieldName("property"))
	}
	return ""
}

func (p nodeSource) string(n *ts.Node) string {
	if n == nil || n.Kind() != "string" {
		return ""
	}
	var out []byte
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		switch child.Kind() {
		case "string_fragment":
			out = append(out, p.text(child)...)
		case "escape_sequence":
			value := p.text(child)
			if len(value) == 2 {
				out = append(out, value[1])
			} else {
				return ""
			}
		default:
			return ""
		}
	}
	return string(out)
}

func (p nodeSource) childModule(n *ts.Node) string {
	if n == nil || n.Kind() != "call_expression" || !slices.Contains([]string{"require", "import"}, p.name(n.ChildByFieldName("function"))) {
		return ""
	}
	args := n.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() != 1 {
		return ""
	}
	return p.string(args.NamedChild(0))
}

func (p nodeSource) builtin(n *ts.Node) string {
	if n == nil || n.Kind() != "call_expression" || p.name(n.ChildByFieldName("function")) != "process.getBuiltinModule" {
		return ""
	}
	args := n.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() != 1 {
		return ""
	}
	return p.string(args.NamedChild(0))
}

func (p nodeSource) bind(n *ts.Node, modules map[string]map[string]string) bool {
	switch n.Kind() {
	case "variable_declarator":
		name, value := n.ChildByFieldName("name"), n.ChildByFieldName("value")
		if name == nil {
			return true
		}
		module := p.childModule(value)
		if module == "" {
			module = p.builtin(value)
		}
		if module == "" && value != nil && value.Kind() == "await_expression" && value.NamedChildCount() == 1 {
			module = p.childModule(value.NamedChild(0))
		}
		switch name.Kind() {
		case "identifier":
			if module != "" {
				modules[p.name(name)] = map[string]string{"*": module}
			}
		case "object_pattern":
			for i := uint(0); i < name.NamedChildCount(); i++ {
				entry := name.NamedChild(i)
				switch entry.Kind() {
				case "shorthand_property_identifier_pattern":
					if module != "" {
						modules[p.name(entry)] = map[string]string{p.name(entry): module}
					}
				case "pair_pattern":
					if module != "" {
						key := entry.NamedChild(0)
						modules[p.name(entry.NamedChild(1))] = map[string]string{p.name(key): module, "*": module}
					}
				default:
					return true
				}
			}
		default:
			return true
		}
	case "import_statement":
		source := n.ChildByFieldName("source")
		if source == nil {
			for i := uint(0); i < n.NamedChildCount(); i++ {
				if n.NamedChild(i).Kind() == "string" {
					source = n.NamedChild(i)
				}
			}
		}
		module := p.string(source)
		if module == "" {
			return true
		}
		clause := n.ChildByFieldName("import")
		if clause == nil || clause.Kind() != "import_clause" {
			return true
		}
		for i := uint(0); i < clause.NamedChildCount(); i++ {
			entry := clause.NamedChild(i)
			switch entry.Kind() {
			case "identifier":
				modules[p.name(entry)] = map[string]string{"*": module}
			case "namespace_import":
				modules[p.name(entry.NamedChild(entry.NamedChildCount()-1))] = map[string]string{"*": module}
			case "named_imports":
				for j := uint(0); j < entry.NamedChildCount(); j++ {
					specifier := entry.NamedChild(j)
					if specifier.Kind() != "import_specifier" {
						return true
					}
					local := specifier.ChildByFieldName("alias")
					if local == nil {
						local = specifier.ChildByFieldName("name")
					}
					modules[p.name(local)] = map[string]string{p.name(specifier.ChildByFieldName("name")): module}
				}
			default:
				return true
			}
		}
	}
	return false
}

func (p nodeSource) function(n *ts.Node, modules map[string]map[string]string) (string, string) {
	name := p.name(n.ChildByFieldName("function"))
	function := n.ChildByFieldName("function")
	if n.ChildByFieldName("optional_chain") != nil || function.Kind() != "identifier" && function.Kind() != "member_expression" {
		return "", ""
	}
	object, method, ok := strings.Cut(name, ".")
	if ok {
		imports, known := modules[object]
		if !known {
			return "", ""
		}
		module, exists := imports[method]
		if !exists {
			module = imports["*"]
		}
		return module, method
	}
	if imports, known := modules[name]; known {
		for original, module := range imports {
			if original != "*" {
				return module, original
			}
		}
		return imports["*"], name
	}
	return "", ""
}

func nodeArguments(code []byte, n *ts.Node) ([]any, bool) {
	if n == nil {
		return nil, false
	}
	switch n.Kind() {
	case "string":
		p := nodeSource{code: code}
		s := p.string(n)
		if s == "" && len(code[n.StartByte():n.EndByte()]) != 2 {
			return nil, false
		}
		return []any{s}, true
	case "array":
		var args []any
		for i := uint(0); i < n.NamedChildCount(); i++ {
			child := n.NamedChild(i)
			if child.Kind() != "string" {
				return nil, false
			}
			p := nodeSource{code: code}
			s := p.string(child)
			if s == "" && len(code[child.StartByte():child.EndByte()]) != 2 {
				return nil, false
			}
			args = append(args, s)
		}
		return args, true
	default:
		return nil, false
	}
}

func (p nodeSource) childCall(n *ts.Node, module, method string, depth int) bool {
	if module != "child_process" && module != "node:child_process" {
		return false
	}
	if method == "fork" {
		return true
	}
	if !slices.Contains([]string{"exec", "execSync", "execFile", "execFileSync", "spawn", "spawnSync"}, method) {
		return false
	}
	args := n.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() == 0 {
		return true
	}
	command, ok := nodeArguments(p.code, args.NamedChild(0))
	if !ok || len(command) != 1 {
		return true
	}
	if method == "exec" || method == "execSync" {
		return shellCreation(command[0].(string), depth, "")
	}
	if args.NamedChildCount() > 2 {
		return true
	}
	var argv []any
	if args.NamedChildCount() > 1 {
		if args.NamedChild(1).Kind() != "array" {
			return true
		}
		argv, ok = nodeArguments(p.code, args.NamedChild(1))
		if !ok {
			return true
		}
	}
	return argvCreation(append(command, argv...), "", depth)
}

func (p nodeSource) creation(n *ts.Node, depth int) bool {
	if n == nil || depth > 256 {
		return depth > 256
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if p.creation(n.NamedChild(i), depth+1) {
			return true
		}
	}
	if n.Kind() == "call_expression" {
		if module := p.childModule(n); module != "" {
			if p.name(n.ChildByFieldName("function")) == "import" || n.ChildByFieldName("function").Kind() == "import" {
				return true
			}
			return false
		}
		function := n.ChildByFieldName("function")
		if function != nil && function.Kind() == "member_expression" {
			if module := p.childModule(function.ChildByFieldName("object")); module != "" {
				return p.childCall(n, module, p.name(function.ChildByFieldName("property")), depth)
			}
			if module := p.builtin(function.ChildByFieldName("object")); module != "" {
				return true
			}
		}
		if module, method := p.function(n, p.modules); module != "" {
			return p.childCall(n, module, method, depth)
		}
	}
	if n.Kind() == "import_declaration" || n.Kind() == "variable_declarator" || n.Kind() == "import_statement" {
		return p.bind(n, p.modules)
	}
	return false
}
