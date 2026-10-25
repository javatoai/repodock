# 开发与构建

RepoDock 使用 Go 1.27.1 / MyGO v0.5.5。保留原生 Windows、macOS 架构，不使用 WebView。

## 启动

```sh
go run ./cmd/repodock --config-dir ./artifacts/dev-config
```

独立配置目录用于开发；省略参数时读取系统用户配置。Git、gh、Codex 为外部本机工具。GitHub 登录由用户通过 gh auth login 完成。

Windows 可构建为无控制台窗口的图形程序：

```powershell
go build -ldflags=-H=windowsgui -o artifacts/RepoDock.exe ./cmd/repodock
```

## 自动检查

```sh
go test ./...
go vet ./...
```

竞态检查需支持的 C 编译器及启用 CGO，再执行 go test -race ./internal/...。不要将环境不支持的竞态检查报告为通过。

## 离线界面预览

```sh
go run ./cmd/preview -out artifacts/redesign
```

生成 21 种状态和 12 张 640×520 窄窗口预览，覆盖未打开的克隆记录、目录／文件冲突、检查中、代理成功及失败。预览与 MyGO 内存界面测试共用真实视图，账户、路径与仓库均为示例，不查询真实 GitHub、不启动 Codex。

默认窗口 960×680。首页使用搜索组合框，已克隆页滚动显示完整本地记录；创建表单在首页原位展开，设置直接平铺在右侧主区域。设置草稿（含代理）只在当前会话首次进入时初始化，切换页面、账户刷新和重复点击入口均不重置；表单滚动、保存按钮固定底部，保存后不跳转。系统文件夹选择器仍为原生对话框，结果回到窗口 UI 线程后更新视图。

## 接口与持久化

领域层区分 LocalProjects 与 Recent；已克隆页读取全部 LocalProjects，按 PreparedAt 排序，旧记录回填 LastOpenedAt、同时间稳定排序。Recent 仅在 Codex 请求成功派发后更新；移除本地记录同时移除对应 Recent，避免迁移复活。配置版本 3 兼容版本 1／2，保留全部旧记录、目录和发布检查点，新增 Proxy 字段。

TargetInspector 与 CloneTargetPath 共用实际目标路径、冲突规则。控制器以取消句柄和代次忽略过期检查，窗口重新聚焦时复查；BeginClone 在创建父目录前复查，Cloner 在准备暂存目录前复查，最终 renameNoReplace 防止检查后的竞争。目标为文件时 Explorer 入口使用父目录。

GitHub.ListRepositories 读取本人名下 visibility=all、affiliation=owner 的分页结果；控制器整体排序、去重，保留部分失败结果并校验账户代次。

Publisher.Inspect 只读检查项目和待提交文件；Publisher.Publish 检查本地状态、提交、创建仓库、推送、验证并逐阶段保存。重试复用回执；创建结果未知时不自动采用同名仓库。所有命令通过现有 Runner 和进程树管理执行，凭据不进入配置。

服务工厂每次捕获 Preferences.Proxy 快照，ExecRunner 仅在开启时清除继承的代理／NO_PROXY 和 Git 注入变量，设置六种大小写代理变量、清空绕过规则，并传递 GIT_CONFIG_COUNT／Git -c 配置；gh 启动的 Git 子进程也会继承。关闭时保留原有网络行为，不修改系统和全局 Git 设置。ProxyTester 使用独立 http.Transport，实际请求 GitHub API /rate_limit 并校验 resources.core，避免普通 API 配额耗尽误报代理错误；支持 HTTP CONNECT、HTTPS 代理和 SOCKS5，不会静默直连。

参考 [Silverwing Go commandruntime](https://github.com/javatoai/silverwing/blob/fc325e92f8f904319813eefb7ae045582a379ff3/internal/commandruntime/command.go) 的地址校验和子进程环境做法。测试通过本地 HTTP／HTTPS／SOCKS5 服务检验流量，实际安装的 gh、Git 使用拒绝连接的本地代理验证 CONNECT 路由；使用合成令牌，不请求真实 GitHub API、不修改全局配置。

## 打包

```sh
python scripts/package.py --targets windows/amd64
python scripts/package.py
```

脚本生成原生资源、三方许可、ZIP 和 SHA-256；支持 Windows amd64、macOS arm64、macOS amd64。推送会触发 Native build 工作流，执行测试、检查并上传构建产物；签名、公证及正式版本发布仍需单独验收。

设计约定见 [DESIGN.md](../DESIGN.md)，产品约束见 [PRODUCT.md](../PRODUCT.md)，实机步骤见 [验收清单](ACCEPTANCE.md)。
