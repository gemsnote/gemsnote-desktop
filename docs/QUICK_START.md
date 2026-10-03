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

Linux ZIP 的菜单安装见下方步骤。Linux 中文桌面会显示“珠玑笔记”，其它语言显示 “Gemsnote”。macOS 首次打开若出现安全提示，请在“系统设置 → 隐私与安全性”中允许；应用名称会随系统语言显示为“珠玑笔记”或 “Gemsnote”。Windows 安装程序会根据安装时的系统语言创建中文或英文的开始菜单、桌面快捷方式和卸载项。

### Linux ZIP 手动安装菜单

先解压并将程序目录放到固定位置。在解压目录执行以下命令安装菜单和图标：

```bash
chmod +x usr/bin/gemsnote
mkdir -p "$HOME/.local/share/applications" "$HOME/.local/share/icons/hicolor/256x256/apps"
cp gemsnote.desktop "$HOME/.local/share/applications/gemsnote.desktop"
cp usr/share/icons/hicolor/256x256/apps/gemsnote.png "$HOME/.local/share/icons/hicolor/256x256/apps/gemsnote.png"
```

编辑刚复制的 `gemsnote.desktop`，把 `Exec=gemsnote` 改为程序的实际绝对路径，
例如 `Exec="/home/yourname/Applications/gemsnote/usr/bin/gemsnote"`，保留 `Icon=gemsnote`。
不要在 `Exec` 中使用 `~` 或 `$HOME`，桌面启动器不会像 shell 一样展开它们。
也可以把程序安装到桌面会话的 PATH，但仅在终端临时修改 PATH 不一定影响应用菜单。
必要时重新登录桌面以刷新菜单；以后移动程序目录时也必须更新 `Exec`。

## 首次连接服务端

启动客户端后，在登录页填写 Gemsnote 服务端地址、用户名和密码。服务端地址应填写 Web 服务的根地址，例如：

```text
http://127.0.0.1:9000
```

当前登录流程通过 `/api2/auth/login` 返回的服务端版本和用户资料识别服务端，使用 API2 同步。
`/api2/system/version` 也可用于独立检查服务端版本。旧 Leanote 服务端不提供新客户端
所需的 API2 登录和同步合同，不能直接使用；请先升级或迁移服务端。

登录成功后，如果该账号没有本地缓存，执行首次重新同步；已有缓存时弹出选择框，
由用户选择“暂不同步”或“重新同步”。暂不同步会保留缓存并暂停自动同步，可进入后手工同步。
重新同步会清理当前账号本地缓存并重新下载，未上传的数据会丢失，必须先确认可以丢弃。
同步过程中请保持客户端运行；笔记同步完成后关闭进度窗，图片和附件可继续在后台下载。

## 本地数据位置

客户端的默认数据目录如下：

| 系统 | 数据目录 |
| --- | --- |
| Linux | `${XDG_CONFIG_HOME:-~/.config}/gemsnote/` |
| macOS | `~/Library/Application Support/gemsnote/` |
| Windows | `%APPDATA%\gemsnote\` |

其中包括：

- `gemsnote.db`：SQLite 数据库，保存账号、笔记本、笔记、标签、同步游标和本地变更队列；
- `data/`：图片、附件、共享笔记缓存及其他本地文件。

客户端不再自动复制或接管旧 `leanote` 目录，也不转换原版 Leanote Electron 缓存。
已有 Gemsnote 数据仍从原 `gemsnote` 目录读取，不受此变更影响。旧目录不会被删除。
有旧本地数据时先完整备份，优先在旧客户端上传至服务端后再由新客户端重新下载。
有无法上传的旧数据时保留原环境并先导出，不要仅改文件名来迁移。

不要在客户端运行时直接修改 SQLite 数据库。迁移电脑或进行备份时，请先退出客户端，再完整复制对应的 `gemsnote` 数据目录。恢复备份前应关闭客户端，并保留原目录副本。

## 离线使用和同步

离线只能访问已缓存的笔记和已下载的文件。历史查询在线时优先读取服务端，失败时
回退到本地编辑历史；服务端历史查询结果不会自动落库，不能依赖一次在线查看来获得
完整历史的离线副本。当前桌面端不支持共享笔记的历史查询。

客户端可以离线阅读和编辑已经缓存的内容。编辑后的笔记会显示未同步状态；恢复网络后点击“立即同步”上传增量变更。需要重新从服务端合并全部笔记本、笔记、标签和头像时，使用“完全同步”。

“重新同步”与“完全同步”不同：前者经确认清理当前账号本地缓存后重新下载，后者保留
本地数据并合并同步。不要用重新同步排查仍含未上传数据的缓存，除非已经备份且同意丢弃。

正常退出账号前客户端会尝试同步；成功直接退出，失败才询问是否不同步退出。
确认则清除登录状态并保留缓存，取消则返回。下次登录必须联网验证；已有缓存时仍由
用户选择，不会自动执行完全同步。离线使用依赖已保存的登录状态及已下载内容。

## 常见问题

- 无法登录：确认服务端地址可访问、账号密码正确，并检查服务端是否为 Gemsnote/API2 版本。
- 同步失败：先确认网络和服务端运行状态；完全同步会保留本地缓存，必要时可退出后重新登录再同步。
- 找不到旧笔记：先检查服务端是否有对应内容，再检查所选账号及缓存；本客户端不自动导入旧目录，不要删除原数据。
- Linux 没有菜单图标：按上文分别安装 `.desktop` 和图标并检查 `Exec`；AppImage 可通过桌面环境的应用菜单工具注册。
