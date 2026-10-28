# 目录绑定修复与验收

日期：2026-10-28。以下记录对应 0.1.0 候选的运行验收。

DSH 0.1.1 使用 tag `dsh-v0.1.1`；三个发布包只更新版本清单和安装说明，
运行源码与二进制均与此处已验收的包逐字节一致。原始日志保留本地，不随源码发布。

## 问题与修复

当 `A/link` 指向 `B/sub` 时，提前清理 `A/link/..` 会把检查目标改成 A，
而 POSIX 文件系统实际遍历后进入 B。旧版共用核心的回归中，10 个目录场景有
7 个失败；旧 DSH 适配层在真实官方 Bash 执行器中也复现了检查 A、执行 B。

本次只修改两处运行源码，继续共用同一个 Go 规则引擎：

- `push.go`：相对工作目录保留原始遍历语义；设置 Git 进程的工作目录，
  原样、按顺序传递每个 `-C`，由 Git 处理链接、`..`、空参数和中途失败。
- `plugins/dsh/index.js`：分别对齐实际官方工具。Bash 的相对目录直接拼接；
  PowerShell 的相对目录使用 `resolve`；绝对目录保持原值。

没有新增运行依赖、配置项或后台服务。已有的仓库、remote、提交和单次授权绑定继续使用。

## Codex 宿主信息缺失

本轮真宿主测试发现，当前 Codex 将 `exec_command` 转换为 `Bash` 钩子，
只发送 `tool_input.command` 和会话 `cwd`，省略工具指定的实际 `workdir`。
即使会话目录是有效仓库，也不能把它当成执行目录。

因此这条入口现在要求：

```sh
git -C "/absolute/repository" push origin main
```

第一个 `-C` 必须是绝对路径；普通 `git push` 或以相对 `-C` 开头的推送直接拒绝，
不生成可授权请求。DSH 适配层提供实际工作目录，仍支持普通 `git push`。
这项差异已同步到中英文首页、Codex 随包安装说明和行为文档。

实测 Codex 版本分别为 macOS `0.164.0-alpha.11.1`、Linux `0.162.1`、
Windows `0.165.0-alpha.1.2`。该结论来自这些实际宿主，不假定未来版本保持相同载荷。

## 本轮验证

| 检查 | macOS ARM64 | Spark1 Linux ARM64 | PAZEBRA Windows x64 |
| --- | --- | --- | --- |
| 共用 Go 推送定向测试 | 9 个顶层测试通过 | 9 个顶层测试通过 | 9 个顶层测试通过 |
| DSH 包的真实 AgentLoop、官方 Shell | 通过 | 通过 | 通过，原生 PowerShell |
| Codex ZIP 的隔离安装与真实 Shell | 通过 | 通过 | 通过，原生 Windows |
| Codex ZIP 的启动脚本与单次授权 | 通过 | 通过 | 通过 |

Go 目录矩阵包括普通目录、相对和绝对 `workdir`、相对和绝对 `-C`、
连续与空 `-C`、`workdir` 加 `-C`，以及前一个目录不存在但后面给了绝对 `-C`。
检查失败必须拒绝，不能跳过中途的无效目录。

真实宿主使用 7 个路径场景与本地授权闭环。DSH 直接加载重建 `.tgz` 内的
适配层和程序，以官方 Shell 实际执行的 `git rev-parse --show-toplevel` 为对照；
本轮没有重跑未变更的官方插件增删或 CLI profile 安装流程。
Codex 从重建 ZIP 创建隔离 marketplace，实际安装、启用并调用钩子。
Windows 使用真实 junction，不用 WSL 或交叉编译结果代替原生执行。

链接由 A 改向 B 后，旧授权不能推送；重新审阅授权后只允许 B 推送一次。
Codex 额外使用临时仓库的 `pre-push` hook 记录实际执行次数，避免把“第二次
推送没有改变分支”误当成“第二次没有执行”。计数证据单独保存在各平台日志中。

## 产物与证据

`dist/codex/` 已重建五个平台 ZIP 和 universal ZIP；`dist/dsh/` 已重建
macOS ARM64、Linux ARM64、Windows x64 的三个 `.tgz`。源码目录的 Codex
分发程序也已同步。本轮包内容使用原始字节比对，未计算或登记额外哈希。

Codex 打包器逐项核对归档内容、程序字节与执行权限。DSH 三包均核对 7 个正式文件，
适配源码和程序与本轮构建一致，执行权限正确，不包含测试、用户配置或凭据。

本地原始日志不进入安装包：

- 旧共用核心回归（`directory-go-red.log`，仅本地）、旧 DSH 回归（`directory-dsh-red.log`，仅本地）、
  旧核心对 Codex 缺目录的回归（`directory-codex-core-red.log`，仅本地）。
- Go macOS（`directory-go-macos.log`，仅本地）、Go Linux（`directory-go-linux.log`，仅本地）、
  Go Windows（`directory-go-windows.log`，仅本地）。
- DSH macOS（`directory-dsh-macos.log`，仅本地）、DSH Linux（`directory-dsh-linux.log`，仅本地）、
  DSH Windows（`directory-dsh-windows.log`，仅本地）。
- Codex macOS（`directory-codex-macos.log`，仅本地）、Codex Linux（`directory-codex-linux.log`，仅本地）、
  Codex Windows（`directory-codex-windows.log`，仅本地）。
- 实际推送计数 macOS（`directory-count-macos.log`，仅本地）、
  实际推送计数 Linux（`directory-count-linux.log`，仅本地）、
  实际推送计数 Windows（`directory-count-windows.log`，仅本地）。
- Codex 包内容（`directory-packages-codex.json`，仅本地）、DSH 包内容（`directory-packages-dsh.json`，仅本地）。

测试源：`push_test.go`、`tests/dsh-host.test.mjs`、
`tests/directory_binding_test.go`、`tests/host_smoke_test.go`。

## 边界

所有真实推送均指向临时本地裸仓库。模型响应为确定性夹具，没有付费模型、
真实 GitHub 写入、公开推送或日常配置变更。本轮仅运行相关定向测试。

macOS Intel、Linux x86_64 完成构建与打包，未做原生宿主验收。
本修复保证已测试的路径解析与重新授权行为；钩子检查和工具执行并非原子操作，
不承诺抵御同一操作系统身份在检查后恶意改写链接、程序或状态。
