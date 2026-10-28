package main

import (
	stdcontext "context"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

// Git object IDs are read from Git, not calculated by the guard. The same command
// must still refer to the same repository, destination and commits when retried.
type PushTarget struct {
	Repository  string `json:"repository"`
	Remote      string `json:"remote"`
	URL         string `json:"destination_url"`
	Source      string `json:"source"`
	Commit      string `json:"commit"`
	Destination string `json:"destination_ref"`
	Force       bool   `json:"force"`
	Lease       bool   `json:"force_with_lease"`
	KnownRemote string `json:"local_remote_tracking_commit,omitempty"`
	Commits     string `json:"commits"`
	Changes     string `json:"changes"`
}

func gitPushCommand(args []any) bool {
	for i := 0; i < len(args); i++ {
		s, ok := args[i].(string)
		if !ok {
			return true
		}
		switch {
		case s == "-C" || s == "-c" || s == "--git-dir" || s == "--work-tree" || s == "--config-env":
			i++
			if s == "-c" && i < len(args) && strings.HasPrefix(text(args[i]), "alias.") {
				_, value, _ := strings.Cut(text(args[i]), "=")
				if strings.Contains(value, "push") {
					return true
				}
			}
		case s == "--help" || s == "-h" || s == "--version":
			return false
		case strings.HasPrefix(s, "-"):
			continue
		default:
			return s == "push" && !(len(args) == i+2 && (args[i+1] == "--help" || args[i+1] == "-h"))
		}
	}
	return false
}

func singleCommand(command any) []string {
	var argv []any
	switch v := command.(type) {
	case []any:
		argv = v
	case string:
		file, err := syntax.NewParser().Parse(strings.NewReader(v), "")
		if err != nil || len(file.Stmts) != 1 {
			return nil
		}
		stmt := file.Stmts[0]
		if stmt.Background || stmt.Negated || stmt.Coprocess || len(stmt.Redirs) != 0 {
			return nil
		}
		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok || len(call.Assigns) != 0 {
			return nil
		}
		for _, w := range call.Args {
			argv = append(argv, literal(w, true))
		}
	}
	var out []string
	for _, arg := range argv {
		s, ok := arg.(string)
		if !ok || strings.ContainsAny(s, "\x00\r\n") {
			return nil
		}
		out = append(out, s)
	}
	return out
}

func inspectPush(e Event, command any) (*PushTarget, error) {
	argv := singleCommand(command)
	if len(argv) < 2 || strings.TrimSuffix(strings.ToLower(filepath.Base(argv[0])), ".exe") != "git" {
		return nil, nil
	}
	var anyArgs []any
	for _, a := range argv[1:] {
		anyArgs = append(anyArgs, a)
	}
	if !gitPushCommand(anyArgs) {
		return nil, nil
	}
	fail := func(message string) (*PushTarget, error) { return nil, fmt.Errorf("%s", message) }
	if argv[0] != "git" && argv[0] != "git.exe" {
		return fail("请使用系统 PATH 中的 git，不执行请求参数指定的程序来核对状态。")
	}
	dir := e.Cwd
	if !filepath.IsAbs(dir) {
		return fail("缺少绝对工作目录，不能核对推送仓库。")
	}
	input, _ := e.Input.(map[string]any)
	if workdir := text(input["workdir"]); workdir != "" {
		if filepath.IsAbs(workdir) {
			dir = workdir
		} else {
			// Preserve symlink/.. traversal for the process that opens the directory.
			dir += string(filepath.Separator) + workdir
		}
	}
	var gitDirs []string
	args := argv[1:]
	for len(args) > 0 && args[0] != "push" {
		if args[0] == "--no-pager" {
			args = args[1:]
			continue
		}
		if args[0] != "-C" || len(args) < 3 {
			return fail("推送请使用单独的 git push <remote> <分支>；不支持覆盖 Git 配置或执行环境。")
		}
		// Git applies each -C in order, including empty paths and failures.
		gitDirs = append(gitDirs, "-C", args[1])
		args = args[2:]
	}
	// Codex's canonical Bash hook omits the tool workdir. DSH supplies a verified cwd.
	if e.Tool == "Bash" && input["dsh_tool"] != "bash" && input["dsh_tool"] != "pwsh" &&
		(len(gitDirs) < 2 || !filepath.IsAbs(gitDirs[1])) {
		return fail("宿主未提供实际执行目录，请使用 git -C <绝对仓库路径> push <remote> <分支>。")
	}
	if len(args) == 0 {
		return fail("缺少 push 参数。")
	}
	var positional []string
	force, lease := false, false
	for _, a := range args[1:] {
		switch a {
		case "-u", "--set-upstream", "--porcelain", "--verbose", "-v", "--quiet", "-q", "--no-follow-tags":
		case "--force", "-f":
			force = true
		case "--force-with-lease":
			lease = true
		default:
			if strings.HasPrefix(a, "-") {
				return fail("该推送选项尚不能核对。请拆成单独的 git push <remote> <分支>；批量、删除、标签和自定义传输不放行。")
			}
			positional = append(positional, a)
		}
	}
	if len(positional) != 2 {
		return fail("请明确一个 remote 和一个分支，例如 git push origin main；不使用隐含目标或批量推送。")
	}
	remote, refspec := positional[0], positional[1]
	if strings.HasPrefix(refspec, "+") {
		force = true
		refspec = refspec[1:]
	}
	source, destination, hasDestination := strings.Cut(refspec, ":")
	if source == "" || strings.ContainsAny(refspec, "*?[] ") || strings.Contains(destination, ":") {
		return fail("请明确单个来源提交和目标分支。")
	}
	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 1800*time.Millisecond)
	defer cancel()
	git := func(a ...string) (string, error) {
		cmd := exec.CommandContext(ctx, argv[0], append(append([]string{}, gitDirs...), a...)...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("无法只读核对 Git 状态（%s），本次不放行", a[0])
		}
		return strings.TrimSpace(string(out)), nil
	}
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	remotes, err := git("remote")
	if err != nil {
		return nil, err
	}
	found := false
	for _, r := range strings.Split(remotes, "\n") {
		if r == remote {
			found = true
		}
	}
	if !found {
		return fail("请使用仓库中已配置的 remote 名称，先只读核对它的目标地址。")
	}
	remoteURL, err := git("remote", "get-url", "--push", "--all", remote)
	if err != nil {
		return nil, err
	}
	if remoteURL == "" || strings.ContainsAny(remoteURL, "\r\n") {
		return fail("一个授权只支持一个推送地址。")
	}
	if parsed, err := url.Parse(remoteURL); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User != nil {
		return fail("推送地址含内嵌认证信息，请先改用凭据管理器，避免在审阅中泄露。")
	}
	// These settings can silently add writes beyond the requested branch.
	for _, key := range []string{"remote." + remote + ".mirror", "push.followTags", "push.recurseSubmodules"} {
		value, configErr := git("config", "--get", key)
		if configErr == nil && value != "false" && value != "no" && value != "off" && value != "0" {
			return fail("Git 配置 " + key + " 会扩展推送范围，暂不支持这类推送。")
		}
	}
	if mapping, _ := git("config", "--get-all", "remote."+remote+".push"); !hasDestination && mapping != "" {
		return fail("remote 配置会重映射目标，请写明 来源分支:refs/heads/目标分支 再审阅。")
	}
	fullSource, err := git("rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", source)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(fullSource, "refs/heads/") {
		return fail("当前只支持本地分支（含指向分支的 HEAD）；标签、游离提交和删除需要另行支持。")
	}
	if !hasDestination {
		destination = strings.TrimPrefix(fullSource, "refs/heads/")
	} else if !strings.HasPrefix(destination, "refs/heads/") {
		return fail("refspec 的目标请写全为 refs/heads/分支名，避免同名标签或远端引用歧义。")
	}
	if !strings.HasPrefix(destination, "refs/") {
		destination = "refs/heads/" + destination
	}
	if !strings.HasPrefix(destination, "refs/heads/") {
		return fail("目标必须是分支。")
	}
	if _, err := git("check-ref-format", destination); err != nil {
		return nil, err
	}
	commit, err := git("rev-parse", "--verify", "--end-of-options", fullSource+"^{commit}")
	if err != nil {
		return nil, err
	}
	base, _ := git("rev-parse", "--verify", "--end-of-options", "refs/remotes/"+remote+"/"+strings.TrimPrefix(destination, "refs/heads/")+"^{commit}")
	rangeArg := commit
	if base != "" {
		rangeArg = base + ".." + commit
	}
	commits, err := git("log", "--format=%H %s", rangeArg, "--")
	if err != nil {
		return nil, err
	}
	var changes string
	if base != "" {
		changes, err = git("diff", "--no-ext-diff", "--no-textconv", "--stat", base, commit, "--")
	} else {
		changes, err = git("ls-tree", "-r", "--long", commit, "--")
	}
	if err != nil {
		return nil, err
	}
	return &PushTarget{Repository: root, Remote: remote, URL: remoteURL, Source: fullSource, Commit: commit, Destination: destination, Force: force, Lease: lease, KnownRemote: base, Commits: commits, Changes: changes}, nil
}
