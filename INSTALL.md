# 安装入口

**Codex** 推荐从 npm 安装（已有 Codex、Node.js 22+ 和 npm）：

```sh
npx --yes codex-stop-pushing-shit@0.1.1
```

装好后开启新聊天，在 `/hooks` 中审阅并信任插件。旧 GitHub / ZIP 用户先按[安装说明](plugins/codex/INSTALL.md)切换来源；英文步骤见 [English README](README.en.md#iii-installation)。GitHub marketplace 和 [Codex Release ZIP](https://github.com/321sssrt-bit/STOP-PUSHING-SHIT/releases/tag/codex-v0.1.1) 继续保留，使用这两种方式无需 Node.js 或 npm。

**DSH** 推荐从 npm 安装到实际使用的 profile：

```sh
dsh plugin --profile web add dsh-stop-pushing-shit@0.1.2
```

需要已有 DSH、Node.js 24.9+、pnpm 和 Git；备用包见 [DSH 0.1.2 Release](https://github.com/321sssrt-bit/STOP-PUSHING-SHIT/releases/tag/dsh-v0.1.2)。安装、停用和卸载见 [DSH 安装说明](plugins/dsh/README.md)。

两个宿主独立版本，不能混装。每个 Release 都附带与对应 npm 版本完全相同的 `.tgz`，并保留原有分平台离线包。
