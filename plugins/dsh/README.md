# STOP-PUSHING-SHIT for DSH

DSH 原生插件，版本 0.1.2。创建 GitHub 仓库（含 fork、模板）、Issue、PR 和
`git push` 前，先返回完整请求供审阅；用户单独发送 `授权提交` 后，只执行该请求一次。
参数、工作目录或推送提交改变，执行失败、取消或本轮结束后，都不能复用旧授权。

## 安装

需要 Node.js 24.9+、DSH、其插件管理命令使用的 pnpm，以及用于核对推送内容的 Git。
推荐从 npm 安装，包内已包含三个支持平台的程序，不需要手动选择平台：

```sh
dsh plugin --profile web add dsh-stop-pushing-shit@0.1.2
```

Windows PowerShell 将 `dsh` 换成 `dsh.cmd`。升级时使用目标版本运行同一条命令。
安装时不会从 GitHub 或开发者服务器额外下载程序。

### GitHub Release 备用安装

从 [DSH 0.1.2 Release](https://github.com/321sssrt-bit/STOP-PUSHING-SHIT/releases/tag/dsh-v0.1.2)
下载 `dsh-stop-pushing-shit-0.1.2.tgz`，它与 npm 发布使用同一个文件；也可按平台下载更小的包：

- macOS Apple Silicon：`dsh-stop-pushing-shit-0.1.2-darwin-arm64.tgz`
- Linux ARM64：`dsh-stop-pushing-shit-0.1.2-linux-arm64.tgz`
- Windows x64：`dsh-stop-pushing-shit-0.1.2-windows-amd64.tgz`

在终端运行，路径替换为下载或本地构建的包：

```sh
dsh plugin --profile web add /absolute/path/dsh-stop-pushing-shit-0.1.2.tgz
```

Windows PowerShell 使用：

```powershell
dsh.cmd plugin --profile web add C:\path\to\dsh-stop-pushing-shit-0.1.2.tgz
```

若没有全局 pnpm，也可以临时提供，不必修改全局安装：

```sh
npm exec --yes --package=pnpm -- dsh plugin --profile web add dsh-stop-pushing-shit@0.1.2
```

安装到实际使用的 profile；使用 `headless` 时把 `web` 改为 `headless`。安装后重新启动
该 DSH profile。插件管理页应显示 `stop-pushing-shit` 已启用且无加载错误。
源码目录没有内置二进制，不能直接当安装包使用。

插件通过 DSH bundle 自动加载，无须改 Codex hooks，也不需要模型 API key 或独立服务。
运行状态放在 `$DSH_HOME/data/stop-pushing-shit/`；未设置 `DSH_HOME` 时使用
`~/.dsh/data/stop-pushing-shit/`。不读取或复用 Codex 的授权状态。

## 使用

1. 让助手准备创建或推送操作。首次工具调用会被拒绝，并返回完整参数。
2. 阅读助手展示的参数、可见性或推送的目标及提交。
3. 另发一条完整消息 `授权提交`。只对原请求授权本轮一次；重试需要重新授权。

只认主会话中 DSH 标记为 `source.kind: user` 的真实用户消息。自动续跑、工具反馈、
子代理任务和历史回放不提供授权。授权文本必须是单个文本块且完整匹配，不能附带其他内容。

## 停用与卸载

在 DSH 插件管理页关闭本 bundle，重启对应 profile 后生效。关闭后保护也随之停止。
重新启用会撤销旧的有效授权。卸载：

```sh
dsh plugin --profile web remove dsh-stop-pushing-shit
```

卸载不会自动删除状态目录。该目录可能包含用户审阅过的请求参数；确认不再需要后可自行删除。

## 支持范围

- 使用原生 `tools.guard()`，在普通 `tools/pre-execute` 放行后仍能拒绝调用。
  Go 程序缺失、无法运行、超时、输出无效或状态损坏时拒绝执行。
- 完整保留结构化工具参数；Shell 接入支持标准 `dsh-tool-bash` 和 `dsh-tool-pwsh`。
  有状态的 persistent shell 和交互终端输入不能按独立完整命令核对，明确拒绝；
  请使用 DSH 默认 Web profile 的标准 Shell 工具。
- DSH 对过长或含特殊字符的 MCP 工具名进行改名后，不再向执行入口提供原始名字。
  这类调用明确拒绝；请缩短 MCP `serverName`，并使用标准工具名（例如 `create_issue`）。
- 规则和命令识别全部来自仓库已有 Go 程序；JS 只负责宿主事件、工具适配和运行程序。
  这仍是有限命令识别，不是操作系统沙箱，也不能防住任意自定义工具内隐藏的外部写入。
- DSH 将插件和配置视为可信代码。插件根本未加载、被禁用或被其他插件删除时，不能提供保护。
  安装状态、模型提示文字或包存在本身不等于保护已运行。
- 提供并验证了 macOS ARM64、Linux ARM64、Windows x64 包。
  Windows 使用宿主的标准 PowerShell 工具，已在 PowerShell 5.1 下验证；
  其他架构未做 DSH 宿主验收。

## 开发与验证

开发时可给插件配置绝对路径 `binary`、`dataDir`；通常安装不需要配置。
在源码根目录构建原有 Go 程序，再打包：

```sh
go build -trimpath -o /tmp/issue-pr-guard .
node tools/package-dsh.mjs darwin-arm64 /tmp/issue-pr-guard
```

Linux 包必须使用 Linux ARM64 程序；可用现有 `tools/build-release.sh` 交叉构建。
Windows 包使用 `windows-amd64` 参数及新构建的 `.exe` 程序。
输出目录为 `dist/dsh/`。包只含接入代码、当前平台二进制、说明和许可证。

```sh
SPS_DSH_ROOT=/absolute/path/to/node_modules/@deepseek-ai/dsh \
SPS_DSH_BINARY=/absolute/path/to/freshly-built-guard \
node --test tests/dsh-host.test.mjs
```

测试使用已安装的真实 Cordis、DSH AgentLoop、会话及工具分发器，配合确定性模型响应；
官方 Bash / PowerShell 执行器实际执行临时目录内的命令和本地裸仓库推送。
不调用付费模型，不向 GitHub 写入，不修改日常 DSH 配置。

Windows 的崩溃、无效输出和超时测试使用原生测试程序，避免把不能执行 Unix 脚本误当成
故障处理通过。开发者需先构建 `./tests/dsh-fault-guard`，再把
`SPS_DSH_FAULT_BINARY` 指向该 `.exe`；它不进入安装包。
