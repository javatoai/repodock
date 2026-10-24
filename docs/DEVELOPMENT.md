# 开发与构建

RepoDock 使用 Go 1.27.1 和 MyGO v0.5.5，桌面界面直接调用应用层。依赖版本由 `go.mod` / `go.sum` 固定，MyGO CLI 通过 Go tool 指令使用同一固定版本。

## 本地启动

```sh
go test ./...
go run ./cmd/repodock
```

Git、GitHub CLI 和 Codex 是本机独立安装的工具。读取真实私人仓库需要先运行 `gh auth login --hostname github.com --web` 完成登录；自动测试和离线预览不需要登录。

使用独立的 RepoDock 设置目录进行开发：

```sh
go run ./cmd/repodock --config-dir ./artifacts/dev-config
```

MyGO 原生热重载：

```sh
go tool mygo dev
```

## 界面预览与图标

生成八种使用示例数据的离线预览，无需图形显示服务器或 GitHub 登录：

```sh
go run ./cmd/preview -out artifacts
```

预览包含主界面、深色、空列表、仓库下拉、克隆中、完成、失败和设置状态。README 中的四幅界面图片分别来自 `dropdown.png`、`progress.png`、`success.png` 和 `dark.png`，发布到 `docs/assets/` 后可直接在 GitHub 中展示。

重新生成应用图标：

```sh
go run ./cmd/icon
```

## 测试

```sh
go test ./...
go test -race ./internal/...
go vet ./...
```

核心测试使用真实临时 Git remote 检验克隆，通过可注入命令适配器检验 GitHub 分页、权限错误、网络失败和账户切换，通过 MyGO 内存渲染检验键盘选择、按钮状态、主题及最近项目操作。自动测试不使用用户真实的 gh 凭据或启动 Codex。

真实私人仓库、系统对话框与 Codex 联动需要按[发布验收清单](ACCEPTANCE.md)在目标系统验证。

## 打包

Python 打包脚本仅使用标准库；Windows 终端可将下列 `python3` 替换为 `python`：

```sh
python3 scripts/package.py
# 仅构建 Windows x64：
python3 scripts/package.py --targets windows/amd64
# 指定 Go 工具链：
python3 scripts/package.py --go /path/to/go
```

脚本通过固定版本的 MyGO CLI 生成应用图标、Windows 清单与版本资源、macOS `.app` 元数据，并收集第三方许可证、创建 ZIP 和 SHA-256 校验文件。构建结果位于 `dist/`，支持 Windows amd64、macOS arm64 和 macOS amd64。

macOS 本机打包时，MyGO 默认使用 ad-hoc 签名；从其他系统交叉构建时不执行完整应用签名。Developer ID 签名与 Apple 公证需要在 macOS 上另行配置，相关选项见 [MyGO 配置文档](https://github.com/egoist/mygo/blob/v0.5.5/docs/configuration.md)。

[GitHub Actions 工作流](../.github/workflows/build.yml)在 Windows 与 macOS runner 上执行测试、静态检查、预览生成及对应平台打包，并上传构建产物。

## 架构

依赖方向为“界面 → 应用用例 → 领域模型与接口”；系统、GitHub、进程与存储通过可注入适配器连接。

| 包 | 职责 |
| --- | --- |
| `internal/domain` | 仓库地址、设置、最近项目和阶段进度 |
| `internal/application` | 工作流协调、请求取消、过期结果保护与端口接口 |
| `internal/platform` | gh / Git 命令、进程树管理、目录提交及 Codex URI |
| `internal/storage` | 原子 JSON 保存、损坏设置备份与未来版本保护 |
| `internal/desktop` | MyGO 界面、原生目录选择、主题与系统打开动作 |

克隆先写入所选父文件夹中的 `.repodock-*` 临时目录，验证成功后以“不覆盖已有目录”的方式保存到最终路径。Windows 使用 Job Object，Unix 使用进程组，确保取消时终止子进程后再清理本次临时目录。

Codex 通过 `codex://threads/new?path=…` 系统协议打开，项目绝对路径经过 URL 编码。项目目录中的中文、空格和特殊字符按路径处理；启动失败时本地仓库保留，可再次尝试打开。
