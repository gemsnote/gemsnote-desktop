# Desktop Release 构建

Gemsnote Desktop 使用 Wails v2.12.0。GUI 构建依赖宿主系统的原生工具链，因此 Linux、macOS 和 Windows 发布包必须分别在对应操作系统及 CPU 架构上构建，不支持通过 `GOOS`/`GOARCH` 交叉编译。

Desktop 仓库还会复用 Gemsnote 服务端仓库中的 Vue 前端和 TinyMCE 资源。目录必须保持为：

```text
gemsnote/
├── frontend/
├── public/tinymce/
└── desktop-app/
```

版本号集中定义在 `api/version.go` 的 `ClientVersion`。Bash 和 PowerShell 脚本直接读取此定义，不再接受版本参数；版本缺失、重复或不符合 `MAJOR.MINOR.PATCH` 时立即失败。

## 支持的平台

| 平台 | 构建宿主 | 产物 |
| --- | --- | --- |
| Linux amd64 | Linux amd64 | `gemsnote-<version>-linux-amd64.zip`、`gemsnote-<version>-linux-amd64.AppImage` |
| Linux arm64 | Linux arm64 | `gemsnote-<version>-linux-arm64.zip`、`gemsnote-<version>-linux-arm64.AppImage` |
| macOS amd64 | Intel macOS | `gemsnote-<version>-darwin-amd64.dmg` |
| macOS arm64 | Apple Silicon macOS | `gemsnote-<version>-darwin-arm64.dmg` |
| Windows amd64 | Windows amd64 | `gemsnote-<version>-windows-amd64-installer.exe`（NSIS） |

每次成功构建会覆盖输出目录中的 `SHA256SUMS`，仅筛选当前版本，但范围依脚本不同：

- Linux Bash：目录内当前版本的 `.AppImage` 和 `.zip`；
- macOS Bash：目录内当前版本的 `.dmg`；
- Windows PowerShell：目录内当前版本的 `.exe`、`.dmg`、`.AppImage` 和 `.zip`。

因此在同一目录轮流运行不同平台脚本，最后的清单不保证涵盖所有平台。
各平台产物宜分别保存；集中分发时先收齐同一版本的产物，再统一生成并核对完整清单，
不要直接把某个本地构建的 `SHA256SUMS` 当作跨平台最终清单。

## 前置依赖

所有平台需要 Go 1.23 或更高版本、Node.js 22、npm，以及 Wails CLI v2.12.0：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
```

Linux/macOS 的 Bash 发布脚本固定执行 `$HOME/go/bin/wails`，不通过 PATH 查找。
如果设置了自定义 GOPATH/GOBIN，默认 `go install` 可能安装到其他目录，单纯修改
PATH 无法修复此脚本的缺文件错误。可显式安装到脚本所需目录：

```bash
GOBIN="$HOME/go/bin" go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
"$HOME/go/bin/wails" version
```

开发时直接执行 `wails dev/build` 则仍通过 PATH 查找。例如默认 Go 安装目录：

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Windows PowerShell 发布脚本通过 PATH 查找 Wails。默认安装目录可这样加入；若设置了
自定义 GOBIN，应改为该实际目录：

```powershell
$env:Path = "$(go env GOPATH)\bin;$env:Path"
```

将该目录加入系统或用户的永久 `PATH` 后，新打开的终端也可以直接运行 `wails version`。

Linux 还需要 `zip`、`sha256sum` 和 `appimagetool`：

```bash
sudo apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

`appimagetool` 必须下载与 Linux CPU 架构匹配的版本：amd64 下载 `appimagetool-x86_64.AppImage`，arm64 下载 `appimagetool-aarch64.AppImage`。从 [GitHub Releases](https://github.com/AppImage/appimagetool/releases) 下载后，必须重命名为 `appimagetool`、增加可执行权限并放入 `PATH`，例如：

```bash
mv appimagetool-x86_64.AppImage appimagetool
chmod +x appimagetool
sudo mv appimagetool /usr/local/bin/
```

`APPIMAGE_RUNTIME_FILE` 是可选环境变量，仅在 Linux 构建 AppImage 时使用。通常无需设置；如果 `appimagetool` 下载 Type 2 runtime 失败（例如 GitHub 返回 504），可从 [type2-runtime Releases](https://github.com/AppImage/type2-runtime/releases) 手工下载**与当前构建架构一致**的 runtime：Linux amd64 用 `runtime-x86_64`，Linux arm64 用 `runtime-aarch64`。下载后将该变量设为本地文件的绝对路径，再运行构建脚本：

```bash
APPIMAGE_RUNTIME_FILE=/absolute/path/to/runtime-x86_64 scripts/build-release.sh
# Linux arm64 则使用 /absolute/path/to/runtime-aarch64
```

也可以和显式的平台、架构、输出目录参数一起使用。路径必须指向已下载的普通文件；脚本会检查文件是否存在，并将其作为 `appimagetool --runtime-file` 参数传入，从而跳过在线下载 runtime。该变量不会影响 ZIP 包或 macOS、Windows 构建。

Linux ZIP 内含 `gemsnote.desktop` 和图标。菜单安装需同时处理可执行文件路径与图标
主题目录，不能只复制 `.desktop`；完整步骤见[快速开始](QUICK_START.md#linux-zip-手动安装菜单)。

Windows 脚本依赖 PowerShell 和 Git for Windows 提供的 `bash`，因为 Wails 的前端构建钩子会调用 `build-frontend.sh`。

Windows 安装包还需要 NSIS，`makensis.exe` 必须可从构建进程的 PATH 找到。
当前 GitHub 工作流安装 NSIS 3.12.0，本地可使用相同版本。已安装 Chocolatey 时，
在具备安装权限的 PowerShell 中执行：

```powershell
choco install nsis --version=3.12.0 --yes --no-progress
```

也可以自行安装 NSIS。默认安装目录可这样加入当前构建终端的 PATH 并检查；若安装在
其他位置，将路径改为实际目录：

```powershell
$NsisDir = Join-Path ${env:ProgramFiles(x86)} 'NSIS'
if (-not (Test-Path (Join-Path $NsisDir 'makensis.exe'))) { throw 'NSIS not found' }
$env:Path = "$NsisDir;$env:Path"
Get-Command makensis
makensis -VERSION
if ($LASTEXITCODE -ne 0) { throw 'NSIS check failed' }
```

发布脚本最终调用 `wails build --nsis`，不会替你安装 NSIS。缺少该依赖时，即使
Go 和前端编译通过，安装包生成仍会失败。

## 首次构建与前端资源

两种发布脚本均在 Go 测试前调用 `build-frontend.sh`，构建共享前端并复制到
desktop 的 `frontend/dist`（含 TinyMCE），确保 `go:embed` 在干净检出时也有资源可用。
直接运行发布脚本即可，无需预先手动准备；准备失败会停止，不继续测试或打包。
若绕过发布脚本单独执行 `go test ./...`，仍须先运行 `bash build-frontend.sh`。

## Linux 和 macOS

在 `desktop-app` 目录运行：

```bash
scripts/build-release.sh
```

完整参数形式为：

```text
scripts/build-release.sh [linux|darwin] [amd64|arm64] [absolute-output-dir]
```

平台、架构和输出目录可以省略，默认使用当前宿主平台、宿主架构和 `desktop-app/release/`。指定的平台和架构必须与宿主一致，输出目录如果指定则必须是绝对路径。

Linux AppImage 构建遇到 runtime 下载失败时，按上文的 `APPIMAGE_RUNTIME_FILE` 说明指定已下载的架构匹配文件即可重试。

## Windows

在 PowerShell 中运行：

```powershell
.\scripts\build-release.ps1
```

也可以指定绝对输出目录：

```powershell
.\scripts\build-release.ps1 -Platform windows -Arch amd64 -OutputDir C:\release
```

## 构建流程

两个脚本执行以下主要检查和构建步骤：

1. 校验版本格式以及 `api.ClientVersion`；
2. 校验 Wails v2.12.0、Go、Node/npm 和共享前端目录；
3. 执行前端 `npm ci`、测试，再通过 `build-frontend.sh` 构建并准备嵌入资源；
4. 执行 Desktop 全量 Go 测试；
5. 使用 Wails 在宿主平台原生构建；
6. 打包产物并更新 SHA-256 校验文件。

Linux 会同时生成带 `.desktop` 和图标的 ZIP 以及 AppImage；macOS 会将 `.app` 制作为 DMG；Windows 会使用 Wails 的 NSIS 模板生成安装器 EXE。Windows 不生成 MSI。

安装后的显示名称按系统语言选择：中文系统显示“珠玑笔记”，其它语言显示 “Gemsnote”。Linux `.desktop`、macOS `InfoPlist.strings` 和 Windows NSIS 安装器均包含相应的本地化名称；可执行文件名仍为 `gemsnote`。

macOS 发布给其他用户前还应完成应用签名和 Apple notarization；Windows 已生成 NSIS 安装包，正式分发可进一步增加 Authenticode 签名。这些签名材料不应写入仓库。

## GitHub 自动发布

Desktop 与服务端均使用不带 `v` 的 `MAJOR.MINOR.PATCH` 标签，例如 `1.0.0`。
发布工作流会按同名 tag 分别检出服务端共享前端与 Desktop 源码，因此推送 Desktop
标签前，确认服务端仓库已有该标签，且两者是匹配的版本。Desktop 标签必须与
`api/version.go` 的 `ClientVersion` 一致；工作流保留此校验。

```bash
git tag -a 1.0.0 -m "Gemsnote Desktop 1.0.0"
git push origin 1.0.0
```

也可在 Actions 中手动指定已有的无 `v` 标签发布。新标签应指向包含新版工作流的提交；
旧 `v` 标签不再触发此工作流，不必删除或移动。已有同名标签时不要重复创建或强行覆盖。
