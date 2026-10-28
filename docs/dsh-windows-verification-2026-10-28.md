# DSH Windows 原生验证

日期：2026-10-28（Asia/Hong_Kong）。插件版本：0.1.0。

本记录是目录绑定修复前的接入验收；同日后续源码修复、重建包与三平台结果见
[目录绑定修复与验收](directory-binding-fix-2026-10-28.md)。

## 结论

PAZEBRA 上的 Windows x64 安装包通过 **13/13 项原生宿主测试**。
官方安装、卸载、重新安装均成功；实际 `dsh web` 从隔离 profile 加载已安装 bundle 后，
三轮工具累计执行次数为 `[0, 1, 1]`，确认首次阻断、授权仅执行一次、再次调用仍阻断。

本轮环境：Node v24.18.0、DSH 0.2.0-rc.2、Windows PowerShell 5.1.26100.9711。
复用 PAZEBRA 已安装的工具，没有升级宿主或 PowerShell，也没有改动日常 DSH profile。

## 覆盖与执行证据

- 未授权阻断、完整参数绑定、单次消费、失败后不可复用。
- 自动消息、其他会话和子代理不能借用授权。
- 仓库可见性参数、轮次结束、用户取消、插件重载和状态损坏。
- 程序缺失、崩溃、无效 JSON 和超时均拒绝执行；其他监听器 allow 不能覆盖 guard。
- 官方 MCP 客户端连接真实 stdio 测试子进程，服务端执行记录确认只执行一次；
  被 DSH 改名的工具身份明确拒绝。
- 官方 `dsh-tool-pwsh` / `dsh-pwsh-local` 调用实际 PowerShell 5.1；
  未授权命令没有启动，普通命令确实写出执行标记。
- 真实 `git push` 仅指向临时本地裸仓库，授权后成功；审阅后的提交变化使推送重新阻断。
- 插件安装目录、测试工作目录和状态路径包含中文及空格。

先验证解包产物，再通过官方命令安装、卸载、重装最终包，并对**已安装副本**重新执行完整
13 项测试。CLI 探针也在重装后再次执行，输出 `SPS_DSH_CLI_PROBE_PASSED`。

Windows 的故障注入使用本轮交叉构建的原生
[`tests/dsh-fault-guard/main.go`](../tests/dsh-fault-guard/main.go) 程序；
它在普通生命周期事件中调用真实 guard，仅在工具检查时按场景崩溃、输出坏 JSON 或挂起。
没有把 Windows 无法执行 Unix shebang 脚本误记成崩溃或超时验证。

测试入口：[DSH 宿主测试](../tests/dsh-host.test.mjs)、
[CLI 加载探针](../tests/dsh-cli-probe.mjs)。
模型响应仍为确定性测试夹具；没有付费模型调用、真实 GitHub 写入或公网 Git 推送。

## 本地证据

- Windows 最终安装包测试日志（`dsh-host-windows.log`，仅本地）
- Windows CLI 三轮结果（`dsh-cli-windows.json`，仅本地）
- 环境版本与卸载、重装退出码（`dsh-environment-windows.json`，仅本地）
- 三平台包内容检查（`dsh-packages.json`，仅本地）

原始日志不随安装包分发。归档白名单、原始文件字节、执行权限和归档元数据检查通过；
Windows 产物约 2 MB，包含 `bin/win32-x64/issue-pr-guard.exe`，与 Node 的实际运行平台匹配。

## 交付与边界

安装包：`dist/dsh/dsh-stop-pushing-shit-0.1.0-windows-amd64.tgz`。
安装、停用、卸载见 [DSH README](../plugins/dsh/README.md)。

Go 规则和运行源码保持不变；本轮只补充 Windows 打包映射与原生测试支持。
涉及的两个跨平台测试在 macOS 定向复验通过；没有重跑无关的完整回归。

Windows x64 已完成原生验收。macOS Intel、Linux x86_64 和 Windows ARM64 尚未验收。
测试进程运行后退出；仅使用临时目录和隔离 DSH_HOME，没有启动常驻测试服务。
插件仍是有限的工具检查，不是操作系统沙箱；有状态终端和被宿主改名的 MCP 工具明确拒绝。
