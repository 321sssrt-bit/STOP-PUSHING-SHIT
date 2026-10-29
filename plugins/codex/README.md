# Codex 首版

当前版本为 **0.1.1**，以 [.codex-plugin/plugin.json](.codex-plugin/plugin.json) 为准，npm 的 [package.json](package.json) 必须与它一致。安装步骤见 [INSTALL.md](INSTALL.md)，验收结果与边界见[验证说明](../../docs/verification.md)。

推荐 `npx --yes codex-stop-pushing-shit@0.1.1`，需要 Node.js 22+；安装器将包内 marketplace 保存到 Codex 配置目录，再调用 Codex 自带的安装命令。Hook 运行仍使用原生程序。GitHub `main` marketplace 与 Release ZIP 保留为不依赖 Node.js 的备用入口。三个入口携带相同的 Hook 和程序，由启动脚本选择平台。

| 源文件 | 用途 |
|---|---|
| [.codex-plugin/plugin.json](.codex-plugin/plugin.json) | 插件标识、版本和 hook 入口 |
| [hooks/hooks.json](hooks/hooks.json) | 四项同步 hook：SessionStart、UserPromptSubmit、Interrupt、PreToolUse |
| [hooks/run.sh](hooks/run.sh)、[hooks/run.ps1](hooks/run.ps1) | 启动已编译程序 |
| [marketplace.json](marketplace.json) | 打包时放入包根 `.agents/plugins/marketplace.json` |
| [INSTALL.md](INSTALL.md) | 随包分发的安装和卸载说明 |

本目录保存宿主接入文件。程序由仓库根目录的 Go 源码构建；GitHub marketplace 分发时，在本目录的 `bin/<平台>/` 中附带原生程序。打包器另将接入文件与程序组装到 `dist/codex/`，ZIP 内使用 `plugin/.codex-plugin/`、`plugin/hooks/`、`plugin/bin/`。

该路径布局遵循 [OpenAI Docs 的插件根目录与相对路径约定](https://developers.openai.com/plugins/build/plugins)。本目录的 `marketplace.json` 供 ZIP 打包使用，指向 `./plugin`；GitHub `main` 分支的 `.agents/plugins/marketplace.json` 则指向 `./plugins/codex`。

发布方式：当前含生命周期 Hook 的方案用于 Codex 手动安装。2026-10-27 核对的 [OpenAI Docs](https://developers.openai.com/plugins/build/plugins#bundled-mcp-servers-and-lifecycle-hooks) 明确说明，含生命周期 Hook 的插件不具备公共插件目录的上架资格；如以后决定进入官方公共目录，需要单独评估方案。
