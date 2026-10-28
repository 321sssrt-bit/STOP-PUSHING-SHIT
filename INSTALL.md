# 安装入口

首版面向 **Codex**，正式 GitHub 安装命令：

```sh
codex plugin marketplace add 321sssrt-bit/STOP-PUSHING-SHIT --ref main --sparse .agents/plugins --sparse plugins/codex
codex plugin add issue-pr-guard@stop-pushing-shit
```

装好后开启新聊天，在 `/hooks` 中审阅并信任插件。完整步骤、可选 ZIP 安装和卸载见[安装说明](plugins/codex/INSTALL.md)；英文步骤见 [English README](README.en.md#installation)。无需另装 Go、Python、Node 或启动服务。
