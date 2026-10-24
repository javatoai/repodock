# RepoDock 项目交接

更新日期：2026-10-24。当前版本：0.1.0。

本文件面向接手开发、测试和发布的工程师。首次阅读建议依次查看本文件、[README](README.md)、[开发与构建](docs/DEVELOPMENT.md)及[实机验收清单](docs/ACCEPTANCE.md)。

## 1. 产品定位与已确定的约束

RepoDock 是独立的 GitHub 项目启动器，主流程为“选择仓库 → 选择本机父目录 → 克隆 → 使用 Codex 打开”。目标平台为 Windows x64、macOS arm64、macOS amd64，使用 Go + MyGO 原生 UI。

主界面保持单页、简洁：可搜索仓库输入框、主要操作卡片、最近项目和右上角账户／设置。使用按钮、布局和状态表达功能，详细日志与错误按需展开。它不是多工具工作台，也不承载定时睡眠功能。

维护时需保持以下行为：

- 复用用户本机 `gh` 有效登录，默认选择 GitHub.com 下本人拥有、凭据有权访问的私人仓库。
- 手动输入支持 GitHub HTTPS / SSH 地址，统一解析仓库身份后通过 HTTPS 克隆。
- 一个实例同时只执行一次克隆；选定父目录后保存到仓库同名子目录。
- 克隆完成后由用户点击打开 Codex 桌面应用，不自动打开，也不切换为 Codex CLI。
- 最近项目最多 20 条；移除记录保留代码，失效路径禁用打开入口。
- 设置与项目记录保存在本机，GitHub 凭据由 `gh` 管理。

## 2. 已实现功能与当前交付状态

已实现地址校验、私人仓库分页／搜索／键盘选择、原生目录选择、阶段进度、取消／重试、目录保护、Codex 协议打开、最近项目和主题设置。

| 验证项目 | 当前结果 |
| --- | --- |
| Go 单元测试、真实临时 Git remote 克隆测试 | Linux 云端通过 |
| MyGO 内存界面测试 | 通过，覆盖搜索、键盘选择、按钮和主题 |
| `go test -race ./internal/...`、`go vet ./...` | 通过 |
| Windows x64 / macOS arm64 / macOS amd64 构建 | 通过，ZIP、目标架构、资源与 SHA-256 已核对 |
| 用户真实私人仓库、系统对话框与 Codex 桌面联动 | 尚未在 Windows / macOS 实机验收 |
| 正式代码签名与 Apple 公证 | 尚未配置 |

当前开发工作区是 Linux 云端 `/workspace/repodock`，不是用户电脑的本地目录。云端测试与离线截图不代表真实账户已完成克隆或用户本机已启动 Codex。

## 3. 快速接手

工具链为 Go 1.27.1、MyGO v0.5.5，依赖由 `go.mod` / `go.sum` 固定。运行真实主流程还需要在本机安装 Git、GitHub CLI、Codex 桌面应用。

```sh
go test ./...
go test -race ./internal/...
go vet ./...
go run ./cmd/repodock --config-dir ./artifacts/dev-config
```

仅在需要访问真实 GitHub 时，由用户在自己的终端完成登录：

```sh
gh auth login --hostname github.com --web
```

使用 `go run ./cmd/preview -out artifacts` 生成八种离线示例界面，使用 `go run ./cmd/icon` 生成图标。自动测试和离线预览不使用用户真实凭据，不启动 Codex。

当前云端的 Go 工具链额外位于 `/workspace/.repodock-tools/go/bin/go`，这是环境专用路径，不应硬编码为用户机器的路径。

## 4. 架构与阅读入口

| 入口 | 职责 |
| --- | --- |
| `cmd/repodock/main.go` | 参数、版本、配置路径与桌面启动 |
| `internal/application/ports.go` | GitHub、克隆、存储、系统动作与工具解析接口 |
| `internal/application/controller.go` | 异步状态协调、请求身份、取消、最近项目与退出 |
| `internal/domain/repository.go` | 仓库地址、配置、最近项目与进度模型 |
| `internal/platform/github.go` | 本机 gh 账户确认与私人仓库分页 |
| `internal/platform/clone.go` | 克隆生命周期、临时目录、Git 验证与最终提交 |
| `internal/platform/runner.go`、`process_*.go` | 子进程、输出上限、凭据脱敏和进程树终止 |
| `internal/platform/rename_*.go` | 各系统“不覆盖已有目录”的原子提交 |
| `internal/platform/codex.go` | Codex 桌面深链构造 |
| `internal/desktop/` | MyGO 视图、原生目录选择、主题、装配与系统动作 |
| `internal/storage/preferences.go` | 原子设置保存、损坏备份和版本保护 |

UI 通过应用层接口调用，不直接执行 Git、解析 gh JSON 或操作任务目录。快照复制切片与可用性映射；后台结果更新后通过 MyGO 窗口刷新展示。

## 5. GitHub、克隆与取消边界

GitHub 查询通过本机 `gh api`，每页 100 条，使用 `affiliation=owner` 与 `visibility=private`，按更新时间排序。适配器核对返回仓库的所有者，并在分页结束再次确认账户。控制器使用 generation 防止旧刷新覆盖新账户；普通分页失败可保留已读结果，但 `Complete` 必须为 false。账户切换时清理旧账户结果。

克隆流程为：

1. 校验仓库身份、父目录与目标同名目录，Windows 额外检查保留文件名。
2. 在所选父目录创建 `.repodock-<repo>-*` 临时目录。
3. 执行 `gh repo clone <HTTPS URL> <stage> --no-upstream -- --progress`，不使用 Shell 拼接。
4. 成功后通过 `git -C <stage> rev-parse --show-toplevel` 验证仓库。
5. 使用系统“不覆盖”重命名提交到最终路径，成功后记录最近项目。

取消须先结束进程树，等命令返回后再删除本次临时目录。Windows 使用 Job Object 并在归入 Job 前挂起子进程；Unix 使用独立进程组。清理失败要回显，不能将失败标记为成功。任何情况下都不得自动覆盖用户已有的同名目录。

命令环境移除继承的本地仓库选择变量，保留用户代理与正常凭据配置；关闭诊断 trace，错误和日志进行凭据脱敏。不要在交接、日志或测试中输出账户令牌。

## 6. Codex、配置与 UI

Codex 通过 `codex://threads/new?path=…` 打开绝对目录，路径经过 URL 编码。系统协议派发成功只代表操作已交给系统，真实聊天创建与目录正确性需要桌面实机验证。安装应用检测属于辅助提示，不能替代实际协议验证。

Windows 配置路径为 `%APPDATA%\RepoDock\state.json`，macOS 为 `~/Library/Application Support/RepoDock/state.json`。schema 版本为 1，保存主题、工具路径、上次父目录和最近项目，不保存 GitHub 令牌。设置文件损坏时保留备份，不覆盖未知未来版本。

工具自动解析 PATH 和固定安装目录，也允许设置绝对路径；解析后检查 `--version` 输出。缺少 Git / gh 不应阻止用户打开已有最近项目。最近项目可用性刷新通过记录身份避免旧结果覆盖刚克隆或重新打开的项目。

参考设计来自 Go / MyGO 版 Silverwing；本工作区的 `/workspace/silverwing` 旧副本仍是 Kotlin，不应拿它作为当前 UI 技术栈的依据。

## 7. 构建与发布

```sh
python3 scripts/package.py
python3 scripts/package.py --targets windows/amd64
python3 scripts/package.py --go /absolute/path/to/go
```

脚本使用 Python 标准库与固定 MyGO CLI，生成 ZIP、第三方许可证和 `dist/SHA256SUMS.txt`。CI 工作流在 Windows、macOS runner 上执行测试、静态检查、预览与打包，上传 Artifacts。正式签名、公证与远程构建状态需在发布时单独确认。

Git 纳入源代码、测试、文档、图片、许可证和 CI。`dist/`、`artifacts/`、开发配置、缓存与本机凭据不纳入 Git。构建压缩包与源代码提交是两种交付物，推送代码不等同于发布安装包。

## 8. 下一位维护者优先处理

1. 按实机清单完成 Windows、macOS 上的 gh 私人仓库、原生对话框、取消与 Codex 联动验收。
2. 在 Windows 验证 Job Object 子进程清理，在 macOS 验证 Finder 启动时的 PATH 探测、目录选择和系统协议。
3. 将 README 新增图片、开发文档与本交接文档纳入分发检查。目前打包脚本仅复制根 README 和许可证等原生资源，离线 ZIP 中的 README 相对文档／截图链接尚未全部配套。
4. 项目扩展应继续围绕仓库准备与打开流程，保持单页、少文字、错误与日志按需展开。
