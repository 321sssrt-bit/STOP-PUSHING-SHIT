# DSH 原生插件验证记录

日期：2026-10-28（Asia/Hong_Kong）。插件版本：0.1.0。本地交付，未发布远端。

本记录是目录绑定修复前的接入验收；同日后续源码修复、重建包与三平台结果见
[目录绑定修复与验收](directory-binding-fix-2026-10-28.md)。

## 结论

macOS ARM64 与 Spark1 Linux ARM64 的最终安装包均通过 13 项直接相关宿主测试。
两边均通过 DSH 官方安装、卸载、重装流程，以及从实际 `dsh web` profile 加载已安装
bundle 的三轮探针：工具累计执行次数为 `0 → 1 → 1`。

Go 授权规则和命令识别源码没有改动。新增 JS 原生接入层，使用同步 `tools.guard()`、
真实会话事件、完整工具参数和独立状态目录。没有第二套规则实现或常驻辅助服务。

## 本轮证据

| 层级 | macOS ARM64 | Spark1 Linux ARM64 |
| --- | --- | --- |
| 运行环境 | 本轮 npm 安装的 DSH；精确版本见环境记录 | DSH 0.2.0-rc.2，Node v24.22.1 |
| 安装包宿主测试 | 13/13 通过 | 13/13 通过 |
| 实际 CLI profile 加载 | `SPS_DSH_CLI_PROBE_PASSED` | `SPS_DSH_CLI_PROBE_PASSED` |
| 未授权／授权／再次调用 | `[0, 1, 1]` | `[0, 1, 1]` |
| 官方插件命令安装／卸载／重装 | 通过 | 通过 |
| 包内容、字节与可执行权限 | 通过 | 通过 |

本地原始证据（不随安装包发布）：

- macOS 测试日志（`dsh-host-macos.log`，仅本地）
- Linux 测试日志（`dsh-host-linux.log`，仅本地）
- macOS CLI 结果（`dsh-cli-macos.json`，仅本地）
- Linux CLI 结果（`dsh-cli-linux.json`，仅本地）
- macOS 精确运行版本（`dsh-environment-macos.json`，仅本地）
- 安装包检查结果（`dsh-packages.json`，仅本地）

测试实现：[宿主测试](../tests/dsh-host.test.mjs)、
[真实 MCP 夹具](../tests/dsh-mcp-fixture.cjs)、
[CLI 加载探针](../tests/dsh-cli-probe.mjs)。
测试直接使用已安装的 Cordis、AgentLoop、Session 和 ToolRuntime；模型响应是确定性夹具。
Shell 测试使用官方 Bash 插件，MCP 测试使用官方 MCP 客户端和实际 stdio 子进程。
执行证据来自工具体内的文件记录以及临时裸仓库的真实分支变化，而非仅检查返回字符串。

## 覆盖的行为

1. 未授权请求不执行；完整参数授权后仅执行一次，同一轮再次调用被拒绝。
2. 修改请求参数后重新阻断；完整参数仍传给 MCP 服务端。
3. 工具执行失败也消耗授权；“继续”等普通消息不构成授权。
4. 自动消息、其他会话和子代理不能借用授权。
5. 创建仓库必须显式填写 `private`；更改可见性需重新授权。
6. 本轮结束、用户取消、插件重载后旧的有效授权失效。
7. 命令行创建和交互终端调用被拒绝；普通读取工具正常运行。
8. 有状态的 persistent shell 明确拒绝；默认标准 Bash 正常运行。
9. 程序缺失、崩溃、无效输出、超时、无效配置和损坏状态均拒绝执行。
10. 其他 `tools/pre-execute` 监听器返回 allow，不能覆盖本插件的 guard 拒绝。
11. 真实 MCP 创建请求只执行一次；被 DSH 截断或改名的工具身份明确拒绝。
12. 真实 Bash 在未授权时不启动待阻断命令，普通命令确实执行。
13. 授权后真实 `git push` 到临时本地裸仓库成功；审阅后提交变化时阻断推送。

另外，当前 Go 源码的 16 项授权状态、绑定、并发消费、状态故障、仓库创建、推送及
Codex launcher/hooks 定向测试通过。未扩大为全量命令识别回归。

## 产物

- `dist/dsh/dsh-stop-pushing-shit-0.1.0-darwin-arm64.tgz`
- `dist/dsh/dsh-stop-pushing-shit-0.1.0-linux-arm64.tgz`

最终包只含原生接入文件、单平台 Go 程序、安装说明、MIT 许可证及第三方许可。
逐项比对归档中文件与当前源码/本轮构建程序的原始字节；程序执行权限正确。
归档属主统一为 root，移除了 macOS 扩展属性；不包含测试、配置、凭据或本机路径。

安装与停用见 [DSH README](../plugins/dsh/README.md)。

## 验证边界

- 使用隔离 DSH_HOME；没有改动本机或 Spark1 的日常 DSH profile。
  CLI 探针运行后退出，没有留下测试 Web 服务。
- 未使用付费模型、真实 GitHub 写入或公网 Git 推送。真实模型自主行为不等于本轮的
  确定性宿主验证，不能据此宣称所有模型都不会尝试绕过。
- 本记录最初仅覆盖 macOS ARM64 和 Linux ARM64；Windows x64 后续已在 PAZEBRA
  完成原生验证，见 [Windows 补充记录](dsh-windows-verification-2026-10-28.md)。
  macOS Intel 和 Linux x86_64 尚未验证。
- 本插件依赖 DSH 成功加载。被禁用、未加载或由其他可信插件移除时没有保护；
  它不是操作系统沙箱，也不承诺识别任意未知工具内隐藏的外部写入。
- 被宿主改名的 MCP 工具以及有状态终端明确不支持，没有静默放行。
