# Gemsnote Desktop（珠玑笔记）

Gemsnote Desktop 是基于 [Leanote Desktop](https://github.com/leanote/desktop-app) 完全重构的桌面客户端。原版实现基于 Electron；本项目改用 Go + Wails，运行性能更好、发布包体积更小，并针对 Gemsnote 重新设计了整个 UI，与 Web 端保持一致。

客户端使用全新的 API2 与 Gemsnote 服务端通信，本地数据使用 SQLite 保存，支持离线查看和编辑，并在联网后进行同步。笔记、笔记本、标签和已下载的图片、附件保存在本机；账号及共享笔记按服务端权限工作。系统管理仅在 Web 提供，桌面端没有管理入口。

历史版本查询优先读取服务端，远程读取失败时回退到本地编辑历史。查询到的服务端历史
不会在该流程中写入本地数据库，因此不能保证离线访问完整服务端历史；本地编辑历史
也不是完整的服务端历史备份。详见[离线使用说明](docs/QUICK_START.md#离线使用和同步)。

## 下载、安装和使用

请先安装与操作系统和 CPU 架构匹配的 Release 包。Linux、macOS 和 Windows 的安装方式、首次登录、服务端配置、本地数据目录和备份说明见 [快速开始](docs/QUICK_START.md)。

## 开发与构建

源码开发、项目结构、API2、本地 SQLite、前端资源、测试、平台依赖和 Release 构建说明见 [开发指南](docs/DEVELOPMENT.md)。完整的多平台发布参数也可参考 [Release 构建说明](docs/RELEASE.md)。

## 服务端

客户端需要连接 Gemsnote 服务端。服务端源码、数据库配置、迁移和部署文档位于 [Gemsnote 主仓库](https://github.com/gemsnote/gemsnote)。

## 许可证

本项目沿用 Leanote Desktop 的开源基础，并在本项目许可证及原项目许可证允许的范围内进行修改和分发。
