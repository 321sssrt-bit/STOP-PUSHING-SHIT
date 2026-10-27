package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/gofrs/flock"
)

const rule = "单次授权插件：创建 GitHub 仓库（含 fork、模板）、issue、PR 和 git push 都须先审阅再授权。创建操作用结构化 GitHub 工具；push 用单独一条 git push 命令，明确 remote 和分支。先尝试一次，让钩子拦下并返回完整参数，再向用户完整展示。新建/模板仓库必须明确 name 和 private 布尔值；fork 须核实源仓库及继承的可见性。推送须展示仓库、目标地址、分支、提交和改动。让用户另发一条消息：授权提交。不需要编号，只授权当前这一笔原始参数一次；参数或推送内容变化须重新授权，失败也消耗授权。修复、继续、发布等笼统指示不代替授权。不要用脚本、浏览器、子代理或修改插件/状态绕过。失败或结果不明时先只读核实远端，不能自动重试。"

type Event struct {
	Name    string `json:"hook_event_name"`
	Session string `json:"session_id"`
	Turn    string `json:"turn_id"`
	Cwd     string `json:"cwd"`
	Source  string `json:"source"`
	Prompt  string `json:"prompt"`
	Tool    string `json:"tool_name"`
	Input   any    `json:"tool_input"`
}

type Target struct {
	Tool  string         `json:"tool_name"`
	Input map[string]any `json:"tool_input"`
	Cwd   string         `json:"cwd"`
	Push  *PushTarget    `json:"push,omitempty"`
}

type State struct {
	Target     *Target `json:"target,omitempty"`
	Request    string  `json:"request_id,omitempty"`
	Created    string  `json:"created_turn,omitempty"`
	Authorized string  `json:"authorized_turn,omitempty"`
}

var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var requestPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var repoPattern = regexp.MustCompile(`^[^/\s]+/[^/\s]+$`)
var createPattern = regexp.MustCompile(`(?:^|[_.])create_(issue|pull_request|pr)$`)
var repositoryCreatePattern = regexp.MustCompile(`(?:^|[_.])create_(?:repository|repo)(?:_from_template)?$`)
var repositoryForkPattern = regexp.MustCompile(`(?:^|[_.])fork_(?:repository|repo)$`)

func context(event, message string) map[string]any {
	return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": message}}
}

func deny(message string) map[string]any {
	return map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": message,
	}}
}

func updateState(e Event, change func(*State) map[string]any) (map[string]any, error) {
	root := os.Getenv("PLUGIN_DATA")
	if root == "" {
		root = os.Getenv("CLAUDE_PLUGIN_DATA")
	}
	if root == "" || !sessionPattern.MatchString(e.Session) {
		return nil, fmt.Errorf("missing PLUGIN_DATA or valid session_id")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(root, e.Session)
	lock := flock.New(path+".lock", flock.SetPermissions(0600))
	defer lock.Close()
	locked, err := lock.TryLock()
	if err != nil || !locked {
		return nil, fmt.Errorf("state lock unavailable: %v", err)
	}
	state := &State{}
	in, err := os.Open(path + ".json")
	if err == nil {
		err = decode(in, &state)
		in.Close()
	} else if os.IsNotExist(err) {
		err = nil
	}
	if err != nil || state == nil {
		return nil, fmt.Errorf("invalid state: %v", err)
	}
	if *state != (State{}) && (state.Target == nil || !requestPattern.MatchString(state.Request) || state.Created == "") {
		return nil, fmt.Errorf("incomplete pending request")
	}
	result := change(state)
	temp, err := os.CreateTemp(root, ".guard-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(temp.Name())
	err = json.NewEncoder(temp).Encode(state)
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path+".json")
	}
	return result, err
}

func creationKind(name string, args map[string]any) string {
	name = strings.ToLower(name)
	if repositoryCreatePattern.MatchString(name) {
		return "仓库"
	}
	if repositoryForkPattern.MatchString(name) {
		return "仓库 fork"
	}
	if match := createPattern.FindStringSubmatch(name); match != nil {
		if match[1] == "issue" {
			return "issue"
		}
		return "PR"
	}
	method, ok := args["method"]
	if !ok {
		method = args["action"]
	}
	if method == "create" {
		if strings.HasSuffix(name, "repository_write") || strings.HasSuffix(name, "repo_write") {
			return "仓库"
		}
		if strings.HasSuffix(name, "issue_write") {
			return "issue"
		}
		if strings.HasSuffix(name, "pull_request_write") {
			return "PR"
		}
	}
	return ""
}

func rejectUnbound(e Event, message string) (map[string]any, error) {
	return updateState(e, func(s *State) map[string]any {
		*s = State{}
		return deny(message)
	})
}

func handle(e Event) (map[string]any, error) {
	empty := map[string]any{}
	switch e.Name {
	case "SubagentStart":
		return context(e.Name, rule), nil
	case "SessionStart", "Interrupt":
		if e.Name == "Interrupt" || e.Source != "compact" {
			if _, err := updateState(e, func(s *State) map[string]any { s.Authorized = ""; return empty }); err != nil {
				return nil, err
			}
		}
		if e.Name == "SessionStart" {
			return context(e.Name, rule), nil
		}
		return empty, nil
	case "UserPromptSubmit":
		return updateState(e, func(s *State) map[string]any {
			s.Authorized = ""
			if s.Target != nil && e.Prompt == "授权提交" && e.Turn != "" && e.Turn != s.Created {
				s.Authorized = e.Turn
				return context(e.Name, "已授权这一笔原始参数，仅本轮一次。"+rule)
			}
			return empty
		})
	case "PreToolUse":
	default:
		return empty, nil
	}
	args, _ := e.Input.(map[string]any)
	kind := creationKind(e.Tool, args)
	if kind == "" {
		switch e.Tool {
		case "Bash", "exec_command", "shell", "shell_command":
			command, ok := args["command"]
			if !ok {
				command = args["cmd"]
			}
			blocked := false
			switch v := command.(type) {
			case string:
				blocked = shellCreation(v, 0, "")
			case []any:
				blocked = argvCreation(v, "", 0)
			default:
				return nil, fmt.Errorf("missing shell command")
			}
			if blocked {
				push, err := inspectPush(e, command)
				if err != nil {
					return rejectUnbound(e, "已阻止 git push："+err.Error())
				}
				if push != nil {
					return authorizeTarget(e, &Target{Tool: e.Tool, Input: args, Cwd: e.Cwd, Push: push}, "git push")
				}
				return rejectUnbound(e, "已阻止通过命令行/API 创建仓库（含 fork、模板建仓）、issue、PR 或未经核实的 git push。创建操作请改用结构化 GitHub 工具；推送请单独执行 git push <remote> <分支>，展示完整参数后取得单次授权。组合命令、动态参数和脚本内的 push 不能按命令文本放行。")
			}
		}
		return empty, nil
	}
	if args == nil || e.Turn == "" || e.Cwd == "" {
		return rejectUnbound(e, "缺少完整工具参数、turn_id 或 cwd，不能核对单次授权。")
	}
	repo, _ := args["repository_full_name"].(string)
	if repo == "" {
		repo, _ = args["repo_full_name"].(string)
	}
	if repo == "" {
		owner, _ := args["owner"].(string)
		name, _ := args["repo"].(string)
		if owner != "" && name != "" {
			repo = owner + "/" + name
		}
	}
	if kind == "仓库" {
		_, explicitPrivate := args["private"].(bool)
		if strings.TrimSpace(text(args["name"])) == "" || !explicitPrivate {
			return rejectUnbound(e, "请明确提供仓库 name 和 private 布尔值（true 为私有，false 为公开），再请求单次授权。")
		}
	} else if !repoPattern.MatchString(repo) {
		return rejectUnbound(e, "请在创建工具参数中明确提供目标 owner/repo，再请求单次授权。")
	}
	return authorizeTarget(e, &Target{Tool: e.Tool, Input: args, Cwd: e.Cwd}, kind)
}

func authorizeTarget(e Event, target *Target, kind string) (map[string]any, error) {
	if e.Turn == "" || e.Cwd == "" {
		return rejectUnbound(e, "缺少 turn_id 或 cwd，不能核对单次授权。")
	}
	return updateState(e, func(s *State) map[string]any {
		if reflect.DeepEqual(s.Target, target) && s.Authorized == e.Turn {
			*s = State{} // Persist consumption before the tool can run, even if it fails.
			return map[string]any{}
		}
		if !reflect.DeepEqual(s.Target, target) || s.Request == "" {
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				panic(err)
			}
			*s = State{Target: target, Request: hex.EncodeToString(id[:]), Created: e.Turn}
		}
		s.Authorized = ""
		parameters, err := json.MarshalIndent(target, "", "  ")
		if err != nil {
			panic(err)
		}
		return deny("尚未提交。需用户单独授权 " + kind + "。\n请向用户展示下面的完整参数，再请用户另发一条消息：授权提交\n仅授权当前这一笔一次；参数或推送内容变化、执行失败后都要重新授权。\n" + string(parameters))
	})
}
