package main

import (
	"regexp"
	"strings"
)

var windowsSyntax = regexp.MustCompile("(?i)^\\s*(?:&\\s|[a-z]:\\\\|invoke-(?:restmethod|webrequest)\\b|iwr\\b|irm\\b|write-output\\b|function\\b)|`\\r?\\n|(?i)^curl\\s+-[a-z]*(?:method|uri)\\b")
var splatPattern = regexp.MustCompile(`^@\w+$`)
var windowsPath = regexp.MustCompile(`^\s*[A-Za-z]:\\`)

func dynamicWindowsCommand(args []any) bool {
	if len(args) == 0 {
		return false
	}
	command := text(args[0])
	return strings.Contains(command, "%") || strings.HasPrefix(command, "$") ||
		strings.HasPrefix(command, "@") || strings.HasPrefix(command, "!")
}

func powershellAPI(args []any) bool {
	method := "GET"
	var endpoint any
	for i, arg := range args {
		s := text(arg)
		flag, value, colon := strings.Cut(s, ":")
		if strings.EqualFold(flag, "-method") || strings.EqualFold(flag, "-uri") {
			var v any = value
			if !colon {
				v = nil
				if i+1 < len(args) {
					v = args[i+1]
				}
			}
			if strings.EqualFold(flag, "-method") {
				method = strings.ToUpper(text(v))
			} else {
				endpoint = v
			}
		} else if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
			endpoint = s
		} else if splatPattern.MatchString(s) {
			method = ""
		}
	}
	if strings.HasSuffix(strings.TrimRight(text(endpoint), "/"), "graphql") {
		if method == "GET" {
			return false
		}
		for _, a := range args {
			if mutationPattern.MatchString(text(a)) || strings.HasPrefix(text(a), "$") {
				return true
			}
		}
		return false
	}
	return restCreation(endpoint, method, false)
}

func windowsCreation(source string, depth int) bool {
	if depth > 8 || len(source) > 262144 {
		return true
	}
	// shortcut: only visible PowerShell/CMD command forms; use a full grammar if coverage expands.
	var args []any
	var word strings.Builder
	quoted := byte(0)
	started := false
	flush := func() {
		if started {
			args = append(args, word.String())
			word.Reset()
			started = false
		}
	}
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '`' && quoted != '\'' {
			if i+1 == len(source) {
				return true
			}
			i++
			if source[i] == '\r' && i+1 < len(source) && source[i+1] == '\n' {
				i++
			}
			if source[i] != '\n' {
				word.WriteByte(source[i])
				started = true
			}
			continue
		}
		if quoted != 0 {
			if c == quoted {
				if i+1 < len(source) && source[i+1] == quoted {
					word.WriteByte(c)
					i++
				} else {
					quoted = 0
				}
			} else {
				word.WriteByte(c)
			}
			continue
		}
		switch c {
		case '\'', '"':
			quoted = c
			started = true
		case '#':
			for i < len(source) && source[i] != '\n' {
				i++
			}
			flush()
			if dynamicWindowsCommand(args) || argvCreation(args, "", depth) {
				return true
			}
			args = nil
		case ' ', '\t', '\r':
			flush()
		case ';', '\n', '&', '|', '(', ')', '{', '}':
			flush()
			if dynamicWindowsCommand(args) || argvCreation(args, "", depth) {
				return true
			}
			args = nil
		default:
			word.WriteByte(c)
			started = true
		}
	}
	flush()
	return quoted != 0 || dynamicWindowsCommand(args) || argvCreation(args, "", depth)
}
