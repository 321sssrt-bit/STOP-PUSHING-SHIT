# npm 与 GitHub Release

Codex 使用 `codex-stop-pushing-shit`，DSH 使用 `dsh-stop-pushing-shit`，保持独立版本号。
当前版本分别是 0.1.3 和 0.1.3。npm 负责分发包内全部程序，不使用安装时下载脚本。

## 打包

更新对应宿主版本与安装文档；Codex 的 `package.json` 必须与 `.codex-plugin/plugin.json`
版本相同，打包器会检查。Go 核心若有变化，先按 [Go 开发说明](go-development.md)
构建五个平台程序，并更新 `plugins/codex/bin/` 中的分发程序。

在 macOS 或 Linux 上使用 Node.js、npm 和已有 Go 工具链：

```sh
go run ./tools/package /absolute/path/to/release-binaries
node tools/package-dsh.mjs darwin-arm64 /absolute/path/to/release-binaries/guard-darwin-arm64
node tools/package-dsh.mjs linux-arm64 /absolute/path/to/release-binaries/guard-linux-arm64
node tools/package-dsh.mjs windows-amd64 /absolute/path/to/release-binaries/guard-windows-amd64.exe
node tools/package-npm.mjs
node --test tests/npm-package.test.mjs
```

`dist/codex/` 保存 ZIP，`dist/dsh/` 保存单平台包，`dist/npm/` 保存两个 npm 通用包。
npm 打包从 `plugins/codex/bin/` 读取同一套程序；Codex ZIP 打包器会核对构建输入与
这些分发程序一致。发布前还需逐文件比较 DSH 单平台包与 npm 包，并在目标宿主实测安装。
仅更新分发方式、未改运行程序时，复用已验收程序并逐字节核对，不另造一套二进制。

## 发布顺序

1. 记录源码提交，创建对应的 `codex-v<版本>` / `dsh-v<版本>` tag；不移动旧 tag。
2. 创建 GitHub 草稿 Release，上传离线包及 `dist/npm/` 内对应的 `.tgz`。
3. 下载草稿附件，逐字节比较原包；用同一个 `.tgz` 执行 `npm publish <包路径> --access public`。
4. 从 npm 下载精确版本，再与 Release 附件逐字节比较，并用该下载包实测安装。
5. 确认后公开 Release，回读版本、tag、附件和 npm 的 `latest`。

无图形浏览器的发布机使用 `npm publish <包路径> --access public --browser=false`，
按 npm 提供的链接完成网页认证。不要让远端尝试启动浏览器。发布可能返回 HTTP 202，
且注册表短暂显示 `0.0.0-stage`；以精确版本下载成功和 `latest` 回读为准，不重复上传。

不要在发布机上重新打包，也不要把单平台 DSH 包发布成 npm 通用包。
不覆盖历史版本；每次变更发布新版本。发布凭据只留在已登录的机器上，不复制到仓库或安装包。

## 验证边界

`tests/npm-package.test.mjs` 检查白名单文件、版本、程序字节与可执行权限，并在隔离的
Codex 配置目录中安装、重复安装、移除 npm 临时来源后重装，运行拒绝→授权→单次执行→再次拒绝闭环。
它不修改日常 Codex 配置，也不向 GitHub 创建仓库。
DSH 沿用 `tests/dsh-host.test.mjs`，通过 `SPS_DSH_PLUGIN` 指向解包后的 `index.js` 验证实际包。
