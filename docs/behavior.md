# 识别范围与授权规则

本文保留首版的详细行为，首页见 [README](../README.md)。0.1.0 面向 Codex，保护 GitHub 仓库、Issue / PR 创建和常见的分支 `git push`；不接管 Release 发布或所有外部写入。

## 一次授权绑定什么

1. 结构化创建工具先被拒绝，插件返回完整待提交参数；draft PR 同样需要授权。每个聊天只有当前一笔待审阅请求，内部标识不要求用户填写。
2. 用户审阅全部参数：新仓库的名称、归属、可见性、描述与初始化选项，或 Issue / PR 的目标仓库、标题、完整正文及 head、base、draft。
3. 用户在后续独立消息中整条准确发送 `授权提交`。额外空格、换行、引号和其他文字均不接受。
4. 原样重试仅放行一次。授权绑定同一聊天、当前轮次、工具、工作目录和全部 JSON 参数值。

“最后提交”“继续”“修好”“同意提交”和工具参数中夹带的授权文字均不能放行。参数改变后重新授权；不完整的变更请求也会撤销旧授权。

授权在实际工具执行前持锁写盘消费；调用失败也不能重用。新用户消息、恢复和中断撤销未使用的授权，同轮压缩保留。失败或结果不明时先只读核实，不能自动重试提交。

同一个程序注册四个事件：`SessionStart` 提示规则并在开始或恢复时撤销旧授权；`UserPromptSubmit` 接收用户的单次授权；`Interrupt` 在中断时撤销授权；`PreToolUse` 核对并消费授权。子代理启动时不再另行重复提示，提交前检查仍由 `PreToolUse` 执行。

## 哪些调用会检查

| 路径 | 行为 |
|---|---|
| `create_repository` / `create_repo` 及其 `_from_template` 形式 | 明确 `name` 与 `private` 布尔值，完整参数绑定，单次授权 |
| `fork_repository` / `fork_repo` | 明确源 `owner/repo`，绑定目标组织及其他全部参数，单次授权 |
| `repository_write` / `repo_write` 的 `method: create` 或 `action: create` | 同新建仓库 |
| `create_issue`、`create_pull_request`、`create_pr` | 完整参数绑定，单次授权 |
| `issue_write` / `pull_request_write` 的 `method: create` 或 `action: create` | 同上 |
| 可识别的 gh、REST、GraphQL 创建 | 拒绝，要求改走结构化工具；不按命令文本授权 |
| Python subprocess、os.spawn* / exec* / posix_spawn*，导入别名、常量拼接、f-string、Python → SSH → gh | 按实际可执行文件和参数位置识别；subprocess 包含 executable / shell 选项 |
| Windows gh.exe、常见 PowerShell / CMD 包装与 REST 别名 | 有限识别；已验证的 GET 查询放行 |
| 查询、本地编辑、打印或准备草稿 | 不追加创建授权 |

新建/模板仓库未明确 `private` 布尔值时直接拒绝，不生成可授权请求；不会自动补默认值。目标归属未传时由创建工具使用当前认证账户，审阅时应明确这一点。fork 的 `owner/repo` 是源仓库，目标组织等参数一起绑定；可见性继承源仓库，需要先只读核实，插件不联网查询。

命令识别覆盖 `gh repo create/new/fork`、REST `/user/repos`、`/orgs/{org}/repos`、模板 `/generate` 和 `/forks`，以及 GraphQL `createRepository`、`cloneTemplateRepository`。查询、`git init`、`git clone`、`gh repo clone` 不属于远端建仓。

Shell 语法树处理注释、heredoc 及命令替换；quoted heredoc 的文本不当作执行内容，实际命令替换仍会检查。Python 使用独立语法树，PowerShell / CMD 只覆盖已验证的常见形式。

## git push

识别常见 shell、Python、SSH、PowerShell 包装中的 `git push`；只允许单独、静态、明确的 `git push <remote> <分支>` 进入授权流程。支持 `-C`、工具的 `workdir`、`-u`、`--force`、`--force-with-lease`；显式 refspec 的目标须写全 `refs/heads/...`。

插件只读核对仓库、remote 地址、来源分支及提交、目标引用、本地远端跟踪提交，并显示提交列表及文件改动摘要。原样重试前重新核对；上述内容变化即重新授权。远端跟踪值是本地记录，不冒充实时远端状态。

隐含目标、批量、删除、标签、多推送地址、组合命令、动态参数，以及会扩展范围的 mirror / followTags / recurseSubmodules 配置，均不进入授权流程。简写目标受 remote.push 重映射时要求写出完整 refspec。Git 配置别名、未知执行入口和同身份篡改仍属于识别边界，不保证拦截。

## 识别与运行边界

- **Node 启动 gh 仍不受识别**，已有明确边界测试。
- 浏览器、磁盘脚本内部、运行时动态/编码程序、已启动终端的后续输入、未知工具及宿主跳过 hook 的路径不保证受约束。
- 无法解析的命令、过深嵌套、已识别执行入口中的未知命令名和未知写请求保守拒绝，可能误拦动态只读操作。
- 管道及无法静态确定的 stdin 不计算实际文本；已识别的 stdin 解释器会拒绝未知输入。Python 多次赋值、增量赋值和不支持的命名 Unicode 转义在执行路径中保守拒绝。
- PowerShell / CMD 命令文本后带附加参数时拒绝；CMD `/k` 与 `/c` 同样检查，Windows 命令位置的变量展开也保守拒绝。
- Agent 与插件拥有同一操作系统身份，不能阻止该身份篡改插件或状态。本项目没有凭证隔离、网络网关或完整安全沙箱。
- 状态、锁、输入及可捕获运行错误返回阻断退出码 2；程序有早于 hook 超时的内部时限。强制终止、hook 未加载或宿主失效仍受宿主行为影响。

状态仅保存在宿主提供的 `PLUGIN_DATA`，每个聊天保存当前一笔待提交参数。插件不读取凭证、不联网；状态、锁文件可能保留，不自动删除用户工作数据。

## 验证说明

当前验证结果与边界见[验证说明](verification.md)，工具链、构建和测试命令见 [Go 开发记录](go-development.md)。历史报告与原始证据在本地原位保留，不随源码提交。
