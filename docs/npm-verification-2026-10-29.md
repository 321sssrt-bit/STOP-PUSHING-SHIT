# npm 分发验证 · 2026-10-29

本轮仅增加 npm 分发、Codex 安装器及文档。Go 核心、Codex Hook 启动脚本和 DSH
接入代码未修改；程序复用已有分发文件，不宣称本轮重新编译或完成全量回归。

| 项目 | 本轮结果 |
| --- | --- |
| Codex npm 包 | `codex-stop-pushing-shit@0.1.1`，包含五个平台程序 |
| DSH npm 包 | `dsh-stop-pushing-shit@0.1.2`，包含 macOS ARM64、Linux ARM64、Windows x64 程序 |
| macOS ARM64 | npm 包定向测试 3/3；实际 `npm exec` 从本地 tarball 安装成功 |
| Linux ARM64 | 同一份 npm 包定向测试 3/3；DSH 插件管理器安装通用 tarball 成功 |
| DSH 实际包 | macOS / Linux 各通过 1 项真实 AgentLoop 定向测试：拒绝、完整参数绑定、单次授权 |
| 备用包一致性 | 六个 Codex ZIP、三个 DSH 单平台包与 npm 包的运行文件逐字节一致 |
| Windows 原生验证 | 本轮未完成：SSH 主机信任校验未通过；不以打包检查代替原生运行 |
| Mac Intel / Linux x64 | 仅检查包内文件及程序一致性，本轮没有原生运行 |

Codex 的 3 项测试覆盖包文件白名单、清单版本、五平台程序字节、可执行权限、
真实 CLI 安装与重复安装、删除 npm 临时来源后重新安装，以及已安装 Hook 的
拒绝→授权→执行一次→再次拒绝。测试目录包含中文、空格及 `&`。
所有安装和状态文件使用临时隔离目录，不更改日常 Codex / DSH 配置，不调用付费模型，
也不向 GitHub 创建测试仓库、Issue 或 PR。

原 GitHub / ZIP 来源与 npm 来源使用同一个 marketplace 名称。已实测 Codex 拒绝
同名不同来源的登记，因此安装说明明确要求切换时先移除旧来源，再立即安装新来源；
npm 安装器不自动删除用户已有来源。

npm 包不含安装时下载脚本、依赖或凭据。GitHub Release 同时保留 npm 原包和原有离线格式；
发布与回读顺序见 [npm 与 GitHub Release](npm-release.md)。

## 公开 npm 包回读

- 从未登录 npm 的 macOS 环境下载 `codex-stop-pushing-shit@0.1.1` 与
  `dsh-stop-pushing-shit@0.1.2`，分别为 9,591,190 和 5,684,953 字节；与本地原包、
  GitHub Release 附件逐字节一致。11 个 Release 附件全部回读通过。
- 使用全新 npm 缓存，从公共注册表执行 Codex npm 安装命令，隔离安装成功。
- 使用全新 DSH profile，从公共注册表按 `dsh-stop-pushing-shit@0.1.2` 包名安装成功。
- npm `latest` 已分别回读为 `0.1.1` 和 `0.1.2`。发布初期的临时占位版本已消失，
  未通过重复上传或改版本号规避传播等待。
