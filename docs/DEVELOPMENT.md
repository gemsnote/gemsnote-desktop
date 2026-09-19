# Desktop 开发与构建指南

Gemsnote Desktop 使用 Go + Wails v2.12.0 构建，前端是主仓库 Vue Web UI 的构建产物，嵌入桌面应用二进制。客户端通过本地 bridge 提供离线 API，并将需要服务端处理的请求代理到 Gemsnote API2。

## 目录与依赖

开发目录建议保持如下布局：

```text
gemsnote/
├── frontend/
├── public/tinymce/
└── desktop-app/
```

Desktop 依赖主仓库的 `frontend` 和 `public/tinymce`。需要 Go 1.23+、Node.js 22、npm、Wails CLI v2.12.0；Linux 还需要 GTK/WebKit 开发库。

安装 Wails CLI：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
```

Linux 常用依赖：

```bash
sudo apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

## 本地开发

在 `desktop-app` 目录执行：

```bash
bash build-frontend.sh
go test ./...
wails dev
```

`build-frontend.sh` 会构建主仓库 Vue 前端，并复制到 `desktop-app/frontend/dist`。`wails dev` 和正式构建都会通过 `wails.json` 的前端钩子准备资源；如果直接运行 `go test ./...`，也必须确保 `frontend/dist` 已存在，因为 `main.go` 使用了 Go `embed`。

## 架构约定

- 新的服务端请求统一使用 `/api2` 和 JSON 契约，不得重新引入旧 `/api`。
- 本地 bridge 负责 SQLite 中的笔记、笔记本、标签、附件和同步队列，保证离线使用。
- 服务端专属功能通过 API2 代理；登录必须在线验证，注销清除本地登录状态但保留缓存。
- 同步使用服务端 ID，不能把本地 SQLite 自增键当作服务端资源 ID。
- 本地 SQLite 数据库位于平台默认 Gemsnote 数据目录，结构迁移由 `db/migrations.sql` 管理。

## 测试

```bash
go test ./...
```

前端测试和构建在主仓库 `frontend` 目录执行：

```bash
npm ci
npm test -- --run
npm run build
```

涉及同步、API2 或本地 bridge 的改动，应同时运行对应的 `sync`、`webapi`、`sharedsync` 测试，并检查离线启动、登录、完全同步、增量同步、头像和附件。

## 本地构建 Release

Wails GUI 构建依赖宿主系统原生工具链，不能用 `GOOS`/`GOARCH` 在其他系统交叉构建。Linux/macOS 使用：

```bash
scripts/build-release.sh 1.0.0
```

Windows PowerShell 使用：

```powershell
.\scripts\build-release.ps1 -Version 1.0.0
```

脚本会检查版本、依赖、前端资源和测试，然后生成对应平台安装包。完整的平台、图标、AppImage runtime、NSIS 和 GitHub Actions 说明见 [Release 构建说明](RELEASE.md)。

安装包的显示名称按系统语言本地化：中文系统显示“珠玑笔记”，其它语言显示 “Gemsnote”；可执行文件名、数据目录名和内部包标识始终保持 `gemsnote`，避免升级和数据迁移受到影响。

## 提交前检查

提交前至少确认：

1. `go test ./...` 通过；
2. 前端测试和生产构建通过；
3. 新请求没有引用旧 `/api`；
4. 没有把 SQLite 数据库、用户缓存、构建产物或密钥加入 Git；
5. API、同步协议和本地数据库迁移与服务端兼容。
