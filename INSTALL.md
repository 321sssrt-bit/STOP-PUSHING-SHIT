# 安装入口

首版面向 **Codex**，正式 GitHub 安装命令：

```sh
codex plugin marketplace add 321sssrt-bit/STOP-PUSHING-SHIT --ref main --sparse .agents/plugins --sparse plugins/codex
codex plugin add issue-pr-guard@stop-pushing-shit
```

装好后开启新聊天，在 `/hooks` 中审阅并信任插件。完整步骤、可选 ZIP 安装和卸载见[安装说明](plugins/codex/INSTALL.md)；英文步骤见 [English README](README.en.md#iii-installation)。无需另装 Go、Python、Node 或启动服务。

DSH 原生插件安装包见 [DSH 0.1.1 Release](https://github.com/321sssrt-bit/STOP-PUSHING-SHIT/releases/tag/dsh-v0.1.1)，本地构建输出位于 `dist/dsh/`；平台选择、安装、停用和卸载见
[DSH 安装说明](plugins/dsh/README.md)。DSH 包需要宿主已有 Node.js，与 Codex 的 ZIP 包分开使用。
