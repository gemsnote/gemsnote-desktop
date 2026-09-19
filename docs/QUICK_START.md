# Desktop 快速开始

Gemsnote Desktop 是离线优先的客户端。首次使用需要连接一个 Gemsnote 服务端完成登录；登录后笔记数据会保存到本地 SQLite，之后可以离线查看和编辑，联网后使用“立即同步”或“完全同步”。

## 下载 Release 包

从 GitHub Release 下载与系统匹配的包：

| 系统 | 文件 | 使用方式 |
| --- | --- | --- |
| Linux amd64/arm64 | `gemsnote-<version>-linux-<arch>.AppImage` | 增加可执行权限后直接运行 |
| Linux amd64/arm64 | `gemsnote-<version>-linux-<arch>.zip` | 解压后运行程序；其中包含 `.desktop` 文件和图标 |
| macOS Intel/Apple Silicon | `gemsnote-<version>-darwin-<arch>.dmg` | 打开 DMG，将 Gemsnote 拖入 Applications |
| Windows amd64 | `gemsnote-<version>-windows-amd64-installer.exe` | 运行 NSIS 安装程序 |

Linux AppImage 示例：

```bash
chmod +x gemsnote-1.0.0-linux-amd64.AppImage
./gemsnote-1.0.0-linux-amd64.AppImage
```

Linux ZIP 包可以将 `.desktop` 文件复制到 `~/.local/share/applications/`，以便从应用菜单启动。Linux 中文桌面会显示“珠玑笔记”，其它语言显示 “Gemsnote”。macOS 首次打开若出现安全提示，请在“系统设置 → 隐私与安全性”中允许；应用名称会随系统语言显示为“珠玑笔记”或 “Gemsnote”。Windows 安装程序会根据安装时的系统语言创建中文或英文的开始菜单、桌面快捷方式和卸载项。

## 首次连接服务端

启动客户端后，在登录页填写 Gemsnote 服务端地址、用户名和密码。服务端地址应填写 Web 服务的根地址，例如：

```text
http://127.0.0.1:9000
```

客户端会通过 `/api2/system/version` 识别 Gemsnote 服务端，并使用 API2 登录和同步。旧 Leanote 服务端不提供该接口，不能直接用于新客户端；请先升级或迁移服务端。

登录成功后，客户端会执行首次完全同步。同步过程中请保持客户端运行；数据较多时可能需要较长时间。

## 本地数据位置

客户端的默认数据目录如下：

| 系统 | 数据目录 |
| --- | --- |
| Linux | `${XDG_CONFIG_HOME:-~/.config}/gemsnote/` |
| macOS | `~/Library/Application Support/gemsnote/` |
| Windows | `%APPDATA%\\gemsnote\\` |

其中包括：

- `gemsnote.db`：SQLite 数据库，保存账号、笔记本、笔记、标签、同步游标和本地变更队列；
- `data/`：图片、附件、共享笔记缓存及其他本地文件；
- 旧版本地数据：首次启动时会从同级目录的 `leanote` 数据目录自动复制，不会删除原目录。

不要在客户端运行时直接修改 SQLite 数据库。迁移电脑或进行备份时，请先退出客户端，再完整复制对应的 `gemsnote` 数据目录。恢复备份前应关闭客户端，并保留原目录副本。

## 离线使用和同步

客户端可以离线阅读和编辑已经缓存的内容。编辑后的笔记会显示未同步状态；恢复网络后点击“立即同步”上传增量变更。需要重新从服务端合并全部笔记本、笔记、标签和头像时，使用“完全同步”。

退出账号前建议先执行一次同步并等待完成。注销只清除本地登录状态，不会删除笔记缓存；下次登录时客户端会重新验证服务端并执行完全同步。

## 常见问题

- 无法登录：确认服务端地址可访问、账号密码正确，并检查服务端是否为 Gemsnote/API2 版本。
- 同步失败：先确认网络和服务端运行状态；完全同步会保留本地缓存，必要时可退出后重新登录再同步。
- 找不到旧笔记：确认旧客户端数据目录位于当前系统默认目录的同级 `leanote` 目录，并在首次启动前关闭客户端。
- Linux 没有菜单图标：使用 ZIP 包时，将包内的 `.desktop` 文件和图标复制到用户应用目录；AppImage 可通过桌面环境的应用菜单工具注册。
