# Desktop 开发与构建指南

Gemsnote Desktop 使用 Go + Wails v2.12.0 构建，前端是主仓库 Vue Web UI 的构建产物，嵌入桌面应用二进制。客户端通过本地 bridge 提供离线 API，并将需要服务端处理的请求代理到 Gemsnote API2。

## 正文下载与时间语义

增量同步发现笔记 USN 变化后先获取正文，再将正文、元数据、服务端时间和 USN
一次写入 SQLite；下载失败不会提前确认笔记版本。下载期间本地出现修改则停止覆盖、
保留 dirty 数据并报错。完全同步会重新读取已存在的干净笔记正文，即使 USN 相等，
以修复旧版本“USN 已更新、正文仍旧”的缓存。清空正文也是有效更新。

`getSyncNotes` 和 `getNote` 的服务端元数据 DTO 也会序列化 `Content:""` 占位字段，
不能依据字段存在就认为已下载正文。API 客户端在这两个入口清除 `ContentPresent`，
只有 `getSyncNotesWithContent` 的正文快照可以保留该标志（包括有效的空正文）。
回归测试必须经过真实形状的 HTTP JSON 解码、同步和 SQLite 落库路径。

SQLite schema v3 新增 `local_edited_time`（bridge 字段 `LocalEditedTime`），记录本机
编辑时间，上传不发送此字段，也不发送 `UpdatedTime`；后者由服务端生成并回写本地。
已有笔记离线编辑不再覆盖上次确认的 `UpdatedTime`。全新未上传笔记的旧展示字段
暂保留创建时的本机时间，首次上传后被服务端时间替换；不要用它进行冲突裁决。
待上传笔记的数据库排序使用本机编辑时间。创建时间继续保留原始值，不因同步重置。
旧库升级只为 dirty 笔记保留其原有时间作为本机编辑时间，不能反推已经丢失的历史来源。

时间均按时间戳保存；不做客户端/服务端时差加减，不用时间决定版本胜负。同步和冲突
仍基于 USN。修改服务端时间不会触发旧 `/api` 合同变化：新客户端通过省略可选的
`UpdatedTime` 请求服务端赋值。升级后执行一次完全同步可修复已有正文及时间缓存。

## 目录与依赖

Desktop 是独立仓库，不包含在服务端源码或 Release 包内，也不是服务端的 Git
submodule。在服务端源码根目录执行以下命令取得客户端源码（目标目录不存在时）：

```bash
git clone https://github.com/gemsnote/gemsnote-desktop.git desktop-app
```

已有检出时不要覆盖目录；选择与服务端共享前端匹配的代码版本。服务端 `.gitignore`
忽略嵌套的 `desktop-app/`，两个仓库分别维护提交。

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

桌面界面及与 Web 的差异见 [桌面 UI 约定](UI.md)。本地笔记本移动 bridge 接收本地
笔记本 ID，标记 dirty，上传时转换为服务端 ID；不要把本地 ID 直接发送给服务端接口。

在 `desktop-app` 目录执行：

```bash
bash build-frontend.sh
go test ./...
wails dev
```

`build-frontend.sh` 会构建主仓库 Vue 前端，并复制到 `desktop-app/frontend/dist`。`wails dev` 和正式构建都会通过 `wails.json` 的前端钩子准备资源；如果直接运行 `go test ./...`，也必须确保 `frontend/dist` 已存在，因为 `main.go` 使用了 Go `embed`。

仅编译基础应用可在本目录执行 `wails build`，输出通常位于 `build/bin/`，应用图标来自
`build/appicon.png`。正式 AppImage、DMG、NSIS 安装包使用本仓库发布脚本，见
[Release 构建说明](RELEASE.md)，不要使用服务端的 `scripts/build-release.sh`。

## 架构约定

- 历史查询由 `webapi/local.go` 的 `listHistories` 优先调用远程历史接口，失败时读取 SQLite 中的本地编辑历史；远程查询结果不在此流程落库，不应把本地历史当作服务端完整镜像。共享笔记历史查询当前返回不支持。
- 新的服务端请求统一使用 `/api2`，编码遵循各端点合同，不得重新引入旧 `/api`。JSON、表单和 multipart 不能互换；当前同步笔记写入仍使用表单，详见[服务端 API2](https://github.com/gemsnote/gemsnote/blob/main/docs/development/API2.md)。
- 本地 bridge 负责 SQLite 中的笔记、笔记本、标签、附件和同步队列，保证离线使用。
- 服务端专属功能通过 API2 代理；登录必须在线验证，注销清除本地登录状态但保留缓存。
- 同步使用服务端 ID，不能把本地 SQLite 自增键当作服务端资源 ID。
- 本地 SQLite 数据库位于平台默认 Gemsnote 数据目录；`db/migrations.sql` 提供基础结构，`db/db.go` 按 `PRAGMA user_version` 执行 v2/v3 等增量迁移。修改结构时需同时检查两者及升级测试。

“关于”通过本地 `GET /api2/desktop/about` 获取当前客户端的版本、平台、架构和 Go 运行时，不依赖远程服务端或 `window.go` 绑定。该端点只属于 desktop bridge。

登录后按用户 ID 检查实际 SQLite 缓存（含部分下载和共享内容），不能把读取失败、用户名或保存的服务器地址变化当作“没有缓存”。已有缓存时保持自动同步暂停，由用户选择“暂不同步”或确认“重新同步”；无缓存时可调用本地 `POST /api2/web/resetSync` 并传入 `initial=true`，后端仍须在同步锁内重新检查缓存为空，否则返回 `confirmationRequired`。只有用户明确确认的重置请求才传 `confirm=true`。

同步进度同时通过 Wails `sync-progress` 事件和本地 `GET /api2/web/syncProgress` 提供。前端在进度窗打开时每 500ms 查询一次本地快照，不请求远程服务端；请求完成后停止。字段 `Running`、`Mode`、`Stage` 表示状态，`Current`/`Total` 表示当前阶段完成数量（`Total=0` 表示未知总量），`Percent` 是粗粒度流程进度。未知总量时显示等待动画与已完成数量，不伪造精确百分比。确认窗、进度窗和最终成功/失败以发起同步的 HTTP 请求结果为准。

重新同步在笔记本、正文和标签落库后返回成功并关闭进度窗；图片、附件使用去重后的后台队列，最多 4 路并行下载，SQLite 落库串行进行，头像也在后台刷新。后台任务持有启动时的账号和服务器凭据，不使用前台可变的 API 客户端。注销、切换账号、再次重置和退出应用时取消并等待后台写入停止，避免旧任务污染新缓存。下载状态由本地 `GET /api2/web/downloadStatus` 返回，不能计入笔记的 dirty/未同步状态；失败仅记录后台日志，不能阻塞已完成的笔记同步。当前后台队列在进程内运行，关闭应用会取消未完成下载。

排查慢同步时查看 `API GET ... elapsed/bytes`、`Note sync page ... download/local` 和 `Background media downloads ...` 日志，以区分正文网络耗时、SQLite 写入耗时及图片附件下载；日志不输出认证 token 或笔记正文。

共享前端的文件选择按钮、确认/输入对话框和表单校验提示使用应用内语言。请复用 `FilePicker.vue` 和 `dialogs.ts`，不要直接使用原生 `alert/confirm/prompt`。系统文件选择窗口中的目录名称、系统按钮等由操作系统负责本地化；Wails v2 没有跨平台的运行时语言切换选项，应用语言不能覆盖这些系统界面。

## 测试

```bash
go test ./...
```

前端测试和构建在主仓库 `frontend` 目录执行：

```bash
npm ci
npm test -- --run
npm run build
npm run test:auth-sync
npm run test:desktop-ui
```

涉及同步、API2 或本地 bridge 的改动，应同时运行对应的 `sync`、`webapi`、`sharedsync` 测试，并检查离线启动、登录、完全同步、增量同步、头像和附件。

## 本地构建 Release

Wails GUI 构建依赖宿主系统原生工具链，不能用 `GOOS`/`GOARCH` 在其他系统交叉构建。Linux/macOS 使用：

```bash
scripts/build-release.sh
```

Windows PowerShell 使用：

```powershell
.\scripts\build-release.ps1
```

脚本直接读取 `api/version.go` 中的 `ClientVersion`，无需传入版本号，并检查版本格式、依赖、前端资源和测试，然后生成对应平台安装包。Bash 和 PowerShell 发布脚本均在 Go 测试前调用 `build-frontend.sh`，自动准备最新的嵌入资源，干净检出无需手动预构建。完整的平台、图标、AppImage runtime、NSIS 和无 `v` 标签发布说明见 [Release 构建说明](RELEASE.md)。

安装包的显示名称按系统语言本地化：中文系统显示“珠玑笔记”，其它语言显示 “Gemsnote”；可执行文件名、数据目录名和内部包标识始终保持 `gemsnote`，避免升级和数据迁移受到影响。

## 提交前检查

提交前至少确认：

1. `go test ./...` 通过；
2. 前端测试和生产构建通过；
3. 新请求没有引用旧 `/api`；
4. 没有把 SQLite 数据库、用户缓存、构建产物或密钥加入 Git；
5. API、同步协议和本地数据库迁移与服务端兼容。

## macOS 原生菜单语言

Wails 2.12 的 App/Edit/Window 角色菜单使用硬编码英文，不会随 SPA 的语言设置自动翻译。
`internal/nativemenu` 在 `OnDomReady` 后将翻译任务投递到 AppKit 主线程，读取
`NSLocale.preferredLanguages` 的第一项：中文（`zh`，含简繁体及地区变体）使用简体中文菜单，
其它语言或缺省使用英文。遵循 macOS 为当前进程提供的语言偏好（包括系统的应用专属语言设置），
修改系统语言后需退出并重新启动；网页界面的 `SetLanguage` 不覆盖此原生菜单策略。

翻译通过原生 selector 匹配“隐藏/退出/撤销/拷贝”等项目，并递归处理编辑、窗口和语音菜单，
只替换标题，保留 Wails/AppKit 的 target、action、启用状态和快捷键。
不重建菜单、不执行 JavaScript 编辑命令，不改动依赖中的 Wails 源码。
Linux/Windows 使用空实现。macOS 的 Go 测试会编译原生菜单 fixture，验证语言优先级、
重复调用及动作/快捷键保持；可用 `go test ./internal/nativemenu` 单独运行。
