# 宿主目录

发布顺序是 Codex 首版、DSH 第二版。按宿主保存接入文件，不复制一套运行源码来表示一个新版本。

| 目录 | 当前状态 | 内容 |
|---|---|---|
| [codex](codex/README.md) | 已实现，0.1.1 | npm 安装器、四项同步 hooks、marketplace 与 ZIP 备用包 |
| [dsh](dsh/README.md) | 已实现，0.1.3 | 原生执行 guard、npm 通用包与独立平台备用包 |

Go 运行源码保留在仓库根目录；其中仍包含当前 Codex 事件协议。DSH 原生接入层将真实事件和完整工具参数转换给同一个 Go 程序，不复制授权规则和命令识别。

Codex 打包器输出到 `dist/codex/`；`tools/package-dsh.mjs` 输出 DSH 包到 `dist/dsh/`。版本号以各宿主自己的清单为准，打包不自动改变现有版本号。

`tools/package-npm.mjs` 使用 `plugins/codex/bin/` 内相同的分发程序，输出两个宿主各自的 npm 包到 `dist/npm/`。npm 包内已附带程序，无安装时下载脚本。发布时将同一个 `.tgz` 同时用于 npm 和对应 GitHub Release；流程见 [npm 与 Release 发布](../docs/npm-release.md)。

发布 tag 使用宿主前缀，例如 `dsh-v0.1.1`，指向该安装包对应的源码提交；已发布的 tag 不移动。
