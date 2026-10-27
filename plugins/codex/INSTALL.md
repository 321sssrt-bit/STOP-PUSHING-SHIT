# 安装 STOP-PUSHING-SHIT · Codex

把下面的命令发给你的 Agent：

```sh
codex plugin marketplace add 321sssrt-bit/STOP-PUSHING-SHIT
codex plugin add issue-pr-guard@stop-pushing-shit
```

装好后开一个新聊天，在 `/hooks` 中审阅并信任本插件。当前仓库为私有，安装时需使用有访问权限的 GitHub 账户。

同一份插件自动选择 macOS / Linux 的 ARM64、x86-64 或 Windows x86-64 程序。无需另装 Python、Go、Node 或启动服务；原生 Windows 需要支持 `commandWindows` 的 Codex。

创建 GitHub 仓库、Issue、PR，或执行已识别的 `git push` 时，先看完整请求，再另发一条消息：`授权提交`。只授权当前这一笔一次；内容变化、执行失败都要重新授权。

推送用单独一条 `git push <remote> <分支>`，也支持 `-u`、明确的分支 refspec、`--force`、`--force-with-lease`。插件展示目标地址、来源提交、目标分支和改动摘要。批量、标签、删除、动态或组合推送暂不放行；创建操作须使用结构化 GitHub 工具。

停用：在插件设置中禁用并开启新聊天。卸载：

```sh
codex plugin remove issue-pr-guard@stop-pushing-shit
```

本地状态可能包含待提交正文，卸载不自动删除这些数据。
