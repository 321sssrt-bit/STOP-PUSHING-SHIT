# 安装 STOP-PUSHING-SHIT · Codex

## 从 GitHub 安装

在已安装 Codex 的终端运行：

```sh
codex plugin marketplace add 321sssrt-bit/STOP-PUSHING-SHIT --ref main --sparse .agents/plugins --sparse plugins/codex
codex plugin add issue-pr-guard@stop-pushing-shit
```

正式入口固定到 `main` 分支，marketplace 指向 `plugins/codex`。该入口携带五个平台程序，启动时自动选择当前平台；两项 `--sparse` 减少无关源码和图片下载，不代表只下载一个平台。

装好后开一个新聊天，在 `/hooks` 中审阅并信任本插件。

审查窗口中，本插件有四项以“禁止倒屎”开头的 Hook：开始或恢复聊天、接收本次授权、中断时撤销授权、提交前检查。它们共用一个程序，分别在对应时机运行。

无需另装 Python、Go、Node 或启动服务；原生 Windows 需要支持 `commandWindows` 的 Codex。

## 从 ZIP 安装（可选）

如果 [GitHub Release](https://github.com/321sssrt-bit/STOP-PUSHING-SHIT/releases) 附带 ZIP，可以下载对应平台的 `STOP-PUSHING-SHIT-0.1.0-<平台>.zip`：

| 设备 | 平台 |
|---|---|
| Mac · Apple Silicon | `darwin-arm64` |
| Mac · Intel | `darwin-amd64` |
| Linux · ARM64 | `linux-arm64` |
| Linux · x86-64 | `linux-amd64` |
| Windows · x86-64 | `windows-amd64` |

解压到准备长期保留的目录，再把下面的命令发给你的 Agent，将 `<解压目录>` 换成包含 `.agents/plugins/marketplace.json` 和 `plugin/` 的目录：

```sh
codex plugin marketplace add '<解压目录>'
codex plugin add issue-pr-guard@stop-pushing-shit
```

安装后同样需要开启新聊天，在 `/hooks` 中审阅并信任插件。单平台 ZIP 只携带对应设备的程序；`universal` 包包含五个平台程序，保留作可选离线合集。

## 授权与卸载

创建 GitHub 仓库、Issue、PR，或执行已识别的 `git push` 时，先看完整请求，再另发一条消息：`授权提交`。只授权当前这一笔一次；内容变化、执行失败都要重新授权。

推送用单独一条 `git push <remote> <分支>`，也支持 `-u`、明确的分支 refspec、`--force`、`--force-with-lease`。插件展示目标地址、来源提交、目标分支和改动摘要。批量、标签、删除、动态或组合推送暂不放行；创建操作须使用结构化 GitHub 工具。

停用：在插件设置中禁用并开启新聊天。卸载：

```sh
codex plugin remove issue-pr-guard@stop-pushing-shit
```

本地状态可能包含待提交正文，卸载不自动删除这些数据。

## 许可

项目采用 [MIT 许可证](https://github.com/321sssrt-bit/STOP-PUSHING-SHIT/blob/main/LICENSE)。安装包同时附带 `LICENSE` 和第三方依赖许可 `THIRD_PARTY_NOTICES.txt`。
