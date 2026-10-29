# Go 实现与依赖取舍

版本 0.1.0；更新于 2026-10-28，Asia/Hong_Kong。本记录说明实现、依赖和构建方式。

## 复用什么

| 依赖 | 固定版本 | 本项目用途 | 成本与覆盖 |
| --- | --- | --- | --- |
| mvdan.cc/sh/v3 | v3.14.1 | `syntax` 解析语句、注释、重定向、heredoc、命令替换；`expand.Fields` 解码静态参数 | 使用默认 Bash 语法；本版本库另支持 POSIX、mksh、Zsh、Bats，但本项目不声称全方言验收。动态展开不求值，不读目录做 glob |
| tree-sitter/go-tree-sitter | v0.25.0 | Go 调用成熟 C 语法解析器 | CGo 构建成本；运行时静态编入，用户无须 C 工具链 |
| tree-sitter/tree-sitter-javascript | v0.23.1 | 检查 Node 内联源码和已知 stdin 中可识别的 `child_process` 调用 | 不执行 JavaScript，不依赖 Node 运行时 |
| tree-sitter/tree-sitter-python | v0.25.0 | 解析 Python，包括历史脚本的 f-string、别名及内嵌字符串 | 只有 Python grammar 进入程序；不执行代码，不提供 Python VM |
| gofrs/flock | v0.13.1 | 跨平台 `TryLock` | 争用立即拒绝，不等待宿主超时；底层平台细节由库维护 |

运行路径还使用两个传递依赖：`github.com/mattn/go-pointer v0.0.1`、`golang.org/x/sys v0.47.0`；后者也由 Windows 测试的 Job Object 清理直接引用。Go 模块图可能为上游测试下载其他 grammar / 测试库，程序仅引入 Python 和 JavaScript grammar。没有 go-gh、Node、数据库或服务。

PowerShell grammar 的候选 Go 子模块声明与导入路径不一致，最小依赖探测失败；没有修补上游或把它放进最终依赖。最终继续维护既有范围的有限 PowerShell / CMD 识别，并以原有读写样本校验。没有把 Shell 库冒充成多语言解释器。

## 删除和保留的自有逻辑

删除 Python 运行时依赖、`shlex` 分隔命令、正则剥离 heredoc、手写 POSIX / Windows 文件锁分支和 Python 解释器探测启动代码。Shell 语法边界交给成熟解析库；文件锁交给成熟跨平台库。

保留业务必须的工具分类、HTTP / gh 创建判定、参数绑定、授权状态、Python 有限常量求值及 PowerShell 常见写法。Python 字符串解码仍有少量 Go 适配；命名 Unicode 转义不能确定时拒绝已识别执行调用，不宣称完整常量解释器。

Go 的类型、错误处理和语法树适配增加源码量，程序包也大于 Python 包。收益是用户端不装语言运行时、Shell 边界不再由正则和分词器拼接维护；不能用总行数或“改成 Go”本身证明精简成功。

## 开发与构建

当前 Go 运行实现保留在仓库根目录。Codex 清单、hooks、安装说明与 marketplace 配置集中在 `plugins/codex/`；`plugins/dsh/` 提供 DSH 原生接入层，复用同一 Go 程序。原打包器输出 `dist/codex/`，`tools/package-dsh.mjs` 输出 `dist/dsh/`。

开发需要 Go 1.26+ 和 C 编译器；本轮使用临时目录里的 Go 1.27.3、Zig 0.17.0，以及已有 Xcode clang。没有安装系统工具或改动日常 PATH。

```sh
go test -race .
go build -trimpath -o bin/issue-pr-guard .
SPS_HOST_TEST=1 go test ./tests -run TestCodexHost -v
```

宿主测试控制器和假 MCP / SSH / gh 都用 Go，源码在 `tests/host_smoke_test.go`。测试历史 Python 调用链时，在 macOS / Linux 调用已有 Python 3 执行内联样本；它是被检查的目标语言，不是插件或测试控制器的实现。Windows 使用对应的 PowerShell 假命令样本。测试代码不进入安装包，用户不运行它。`CODEX_TEST_BINARY` 可指向已有的真实 Codex 程序。

在 macOS 上准备好 Go 与 Zig，按本轮方式构建五个平台包：

```sh
SPS_GO=go SPS_ZIG=zig sh tools/build-release.sh /tmp/guard-release
go run ./tools/package /tmp/guard-release
```

构建脚本不下载或安装工具。Linux 使用 musl 静态链接；Windows 使用 mingw 目标静态链接非系统运行库；macOS 使用系统库。`-trimpath` 去除本机构建路径，发布包不含源码、测试、旧报告或个人配置。项目的 MIT 许可证 `LICENSE` 和依赖许可 `THIRD_PARTY_NOTICES.txt` 随包保留。

打包器核对 GitHub marketplace 入口与 `plugins/codex/bin/` 中的五平台程序，重新打开每个 ZIP，断言入口、可执行权限、程序字节和文件白名单；它不是安装系统。更新依赖时需同步 `THIRD_PARTY_NOTICES.txt`。

目录或打包路径变更可先运行 `go test . -run 'TestLauncher|TestHookConfiguration'`，再用 `SPS_RELEASE_PACKAGE` 指向 `dist/codex/` 中当前平台的 ZIP 运行 `go test ./tests -run TestReleasePackage`。真实宿主测试从 `plugins/codex/` 读取接入文件，从 `bin/` 读取本机程序。

运行保护限制：单个 hook JSON / 状态最多 16 MiB，待分析命令最多 256 KiB，跨语言嵌套最多 8 层，内部总时限 2.5 秒；超限拒绝。Python / JavaScript grammar 单次解析也有时限。完整参数比较在这些输入限制内进行。

## 一手依据

- [mvdan syntax API](https://pkg.go.dev/mvdan.cc/sh/v3/syntax)、[expand API](https://pkg.go.dev/mvdan.cc/sh/v3/expand)：语法树和静态参数解码，参数使用 Fields，不能拿 Literal 代替 Shell 参数的转义处理。
- [Tree-sitter 官方 Go binding](https://github.com/tree-sitter/go-tree-sitter)、[JavaScript grammar](https://github.com/tree-sitter/tree-sitter-javascript)：使用后关闭 parser / tree；解析树含错误时拒绝。
- [Tree-sitter 官方 Go binding](https://github.com/tree-sitter/go-tree-sitter)、[Python grammar](https://github.com/tree-sitter/tree-sitter-python)：使用后关闭 parser / tree；解析树含错误时拒绝。
- [gofrs/flock](https://pkg.go.dev/github.com/gofrs/flock)：复用跨平台非阻塞锁。
- [OpenAI Hooks](https://learn.chatgpt.com/docs/hooks)：同步 PreToolUse、退出码 2、deny、独立宿主权限检查、Windows 入口。真实接线结果另见验证记录。
