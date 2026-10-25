# RepoDock 项目交接

更新日期：2026-10-25。本轮包含首页与本地项目流程改版、平铺设置、目录保护、代理支持及 README 更新；已发布 v0.1.0 和配套小红书图文帖。

## 已发布版本与帖子

| 项目 | 地址与状态 |
| --- | --- |
| GitHub Releases | [RepoDock v0.1.0](https://github.com/javatoai/repodock/releases/tag/v0.1.0)，已公开发布并设为 Latest |
| 版本源码 | [a2332b6](https://github.com/javatoai/repodock/commit/a2332b6a028ca1c4b938ae4958cdd060af0cd077)，标签 v0.1.0 指向该提交 |
| 构建验证 | [Native build #2](https://github.com/javatoai/repodock/actions/runs/40154571002)，Windows x64、macOS arm64／x64 的测试、go vet、预览、打包与产物上传全部成功 |
| 小红书 | [VibeCoding:开源项目启动器](https://www.xiaohongshu.com/explore/6add6faf000000002c020d69)，2026-10-25 发布，已通过搜索和帖子详情回读核实 |

发布附件：

- [Windows x64 ZIP](https://github.com/javatoai/repodock/releases/download/v0.1.0/RepoDock-0.1.0-windows-amd64.zip)
- [macOS Apple silicon ZIP](https://github.com/javatoai/repodock/releases/download/v0.1.0/RepoDock-0.1.0-macos-arm64.zip)
- [macOS Intel ZIP](https://github.com/javatoai/repodock/releases/download/v0.1.0/RepoDock-0.1.0-macos-amd64.zip)
- [SHA256SUMS.txt](https://github.com/javatoai/repodock/releases/download/v0.1.0/SHA256SUMS.txt)

三个 ZIP 直接采用上述成功 CI 的产物，已核对 CI 校验值、平台架构、macOS 可执行权限，以及上传后 GitHub 返回的 SHA-256。macOS 包尚未经过 Apple 公证；公开发布不代表 macOS 实机运行或 Codex 实际聊天创建已经完成验收。

小红书使用确认过的文案及三张示例界面图，顺序为“已克隆列表（首图）→ 首页搜索 → 创建项目”。发布脚本曾返回成功但未捕获平台回执，随后查到新帖子并回读详情，核对了标题、正文和三张配图；不可因该类回执缺失而直接重复发布。笔记 ID 为 6add6faf000000002c020d69，交接中不保存访问令牌或登录信息。

当前机器的发布素材和校验记录保存在 artifacts/xiaohongshu/repodock-v0.1.0/ 与 artifacts/releases/v0.1.0/run-40154571002/；两者均为本地忽略目录，不随 Git 仓库分发。

## 产品与界面

RepoDock 使用 Go 1.27.1 + MyGO v0.5.5，为 Windows / macOS 提供原生 GitHub 项目准备与 Codex 打开流程。

- 启动进入首页。首页为远程搜索和创建入口；候选覆盖本人拥有的全部公开、私人仓库，按 updated_at 倒序。
- 已记录结果跳转到“已克隆”页并滚动高亮、清除筛选，不重复克隆或直接启动 Codex。
- “已克隆”显示全部 LocalProjects，包含克隆后未打开的项目；按准备完成时间倒序、旧记录回填最后打开时间，无时间时稳定排序，不截断、不扫描其他仓库。移除同步清理关联历史，保留文件和远端。无网络、缺少 gh 时仍可打开本地项目。
- 克隆检查实际“父目录／仓库名”，父目录中其他项目不影响。选择、更改位置和窗口聚焦时异步检查，取消并丢弃过期结果；检查中、目录存在（含空目录）、文件冲突、读取失败均禁止克隆。冲突可打开资源管理器，文件冲突打开父目录；保留位置更改。控制器和 Cloner 再次复查，最终不覆盖重命名处理竞争。克隆完成仍由用户点击打开 Codex。
- 首页支持新建项目及发布已有无远程目录，私人默认可改公开；发布成功并核对远程提交后自动打开 Codex。
- 左下角设置直接切换右侧主内容区，集中管理默认目录、主题、网络代理、Git/gh 工具和账户。表单滚动、底部固定保存；保存后停留，当前会话保留未保存输入（含代理），账户重新检测不重置草稿。减少标签、口号与常驻说明。
- 代理默认关闭，预填 HTTP／127.0.0.1／7897，支持 HTTP、HTTPS、SOCKS5。测试当前草稿的 GitHub API 连通性和响应；保存后刷新账户，业务任务中禁止修改。
- 默认窗口 960×680，最小 640×520；窄窗口改为图标导航。克隆与发布共用一个任务，跨页面保留进度。

## 架构入口

| 模块 | 职责 |
| --- | --- |
| internal/domain | 仓库身份、可见性、默认目录、准备时间、本地与打开历史、代理校验、发布检查点 |
| internal/application/controller.go | gh 刷新、克隆、本地打开、历史、配置及取消 |
| internal/application/publication.go | 发布与文件预览协调、保存检查点、仅重试 Codex 打开 |
| internal/application/network.go | 目录检查代次、窗口复查、代理草稿测试 |
| internal/platform/github.go | gh 账户、owner/all 仓库分页与账户切换校验 |
| internal/platform/publication.go | 发布前检查、文件预览、Git 提交、GitHub 创建、推送与远程核对 |
| internal/platform/clone.go | 临时目录克隆、Git 验证、不覆盖提交、取消清理 |
| internal/platform/clone_target.go | UI 与后端共用目标路径和冲突判定 |
| internal/platform/runner.go、proxy.go | 单任务代理快照、子进程环境／Git 单次配置、真实 API 连通性测试 |
| internal/desktop | 首页、已克隆及平铺设置页、原生系统操作与离线预览 |
| internal/storage | 版本 1／2→3 迁移、原子替换、Windows 共享冲突重试 |

UI 通过应用层接口调用，不直接执行命令或处理凭据。GitHub 接口现为 ListRepositories；Publisher 端口提供 Inspect / Publish。后台状态快照复制切片和发布检查点。

## 数据与兼容

设置 schema 为 3，兼容版本 1／2。旧记录全部保留，旧目录、发布检查点不丢失；代理默认关闭。PreparedAt 缺失时回填 LastOpenedAt。缺失默认目录时先用旧 LastParent，再用用户目录下的 Projects。未知未来版本禁止覆盖。

LocalProjects 包含已克隆、成功创建／发布或旧版迁入的本地路径；Recent 只记录 Codex 协议派发成功的打开事件。克隆成功立即记录 PreparedAt；发布完成的检查点在调用 Codex 前记录本地项目，打开失败也保留。重试打开不改变准备时间。已克隆菜单移除同时删除两个集合，避免重启迁移时复活，保存失败保留记录并提示。

Proxy 保存 Enabled／URL，不含认证。地址仅允许 HTTP／HTTPS／SOCKS5、有效主机及 1–65535 端口；拒绝内嵌凭据、路径、查询与片段。开启时 ExecRunner 清除继承代理及绕过变量，设置六个代理环境变量、空 NO_PROXY 和受控 GIT_CONFIG_COUNT；直接 Git 命令补充 -c 配置，gh 的 Git 子进程继承环境。关闭时沿用原有网络行为。不改系统代理或全局 Git，不失败回退直连。设置保存失败不切换运行配置。

参考已核对的 [Silverwing Go commandruntime](https://github.com/javatoai/silverwing/blob/fc325e92f8f904319813eefb7ae045582a379ff3/internal/commandruntime/command.go)。本轮 UI 不含代理认证、绕过规则或逐工具开关，Codex 仍由系统协议派发。

Publication 保存请求、账户、路径、分支、提交、创建请求状态、远程 ID/URL、发布完成状态，不保存令牌。保存检查点失败立即停止后续发布操作。

## 发布边界

首次发布前检查目录、Git 提交身份、当前 gh 账户以及 GitHub 同名仓库。已有 Git 目录必须是仓库根目录、处于本地分支且没有远程；普通目录可初始化。预览通过临时 Git 元数据读取 .gitignore，不初始化用户目录。

新建项目创建 main 和 README；已有仓库保留历史、分支与忽略规则。提交所有可跟踪的项目改动；已有提交且没有改动时不追加空提交。提交后保存 SHA，再次核对账户。

通过 gh api POST user/repos 创建仓库，校验仓库 ID、身份、URL 与可见性并保存回执，再添加 origin。推送只为本次 HTTPS Git 命令提供 gh credential helper，不修改全局凭据。核对远程分支 SHA 后标记 Published，随后派发 Codex 请求。

重试检查本地 SHA/分支、远程 ID/可见性/地址，复用已经提交或创建的阶段。创建结果未知而同名仓库已存在时停止，不能自动认领。推送、核对失败保留本地和远端，禁止强推或自动删除。只有 Codex 失败时不再次提交、建仓或推送。结束发布任务只清除检查点，保留目录与远端。

克隆仍使用 gh repo clone，手动 SSH 地址转换为 HTTPS；其临时目录、验证、不覆盖重命名及进程树清理约束保留。Windows 使用 Job Object，Unix 使用进程组。

## 开发与验证

本轮 Windows 本地完整测试通过，共 132 项（含子用例、无跳过）；go vet 通过。已检查目录冲突、本地未打开记录、代理浅深主题及 640×520 预览，设置滚动与固定保存通过交互测试。新版 Windows GUI 已构建并启动，确认 RepoDock 窗口存在、响应正常，当前启动仍使用 artifacts/dev-config，未启用用户的代理配置。启动状态见 artifacts/verification/native-start.json。本机 CGO 关闭且未发现 C 编译器，未运行竞态检查；真实 GitHub 创建/推送与实际 Codex 聊天创建仍待实机验收。

实测本机 http://127.0.0.1:7897，经应用 ProxyTester 请求 https://api.github.com/rate_limit 成功，见 artifacts/verification/proxy-live.json。普通 API 首页当时返回 403、未认证额度为 0；连接测试因此使用 /rate_limit 并校验 resources.core，额度用尽仍能识别网络连通。未保存或开启用户的代理设置。

```sh
go test ./...
go vet ./...
go run ./cmd/preview -out artifacts/redesign
go run ./cmd/repodock --config-dir ./artifacts/dev-config
```

自动测试覆盖真实临时 Git 克隆/推送、公开私人分页、版本 1／2 迁移、目录冲突及检查代次、目录竞争、移除不复活、离线打开、发布前拦截、失败重试、搜索跳转、草稿、菜单、主题与窄窗口。HTTP CONNECT、HTTPS 代理及 SOCKS5 使用本地服务检验实际流量；本机 gh、Git 通过本地拒绝代理检验真实 CONNECT 路由。测试使用合成令牌，不创建或访问真实 GitHub 仓库、不改系统／全局配置。

本轮运行在用户 Windows 本地 Q:\repodock。真实 GitHub 建仓与推送未由自动测试执行；系统协议派发成功不等于实际 Codex 聊天创建。目标系统验收见 docs/ACCEPTANCE.md。

离线预览共 33 张，包含 21 种页面状态和 12 种窄窗口状态，设置及代理包含浅深主题、目录冲突包含目录与文件，示例数据不可视为真实账户结果。

## 后续交付

v0.1.0 的三平台构建与 GitHub Releases 发布已完成。后续继续核对真实私人/公开仓库权限、Windows/macOS 系统对话框、通过 RepoDock 实际创建与推送 GitHub 仓库、实际 Codex 新聊天及工作目录；代码签名、公证与后续版本发布分别验证。
