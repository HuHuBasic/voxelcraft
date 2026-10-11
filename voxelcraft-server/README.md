# VoxelCraft 服务器（桌面版）

跨平台的《方块世界》联机服务器。**解压即用**，内置图形控制台、命令行与日志，
支持所有旧版 / 当前 / 未来客户端版本（协议永不拒绝客户端）。

- 单端口同时提供：游戏 WebSocket + HTTP API + 网页控制台
- 账号注册 / 登录（与游戏客户端共用同一套账号体系）
- 公共大厅 + 独立服务器（房主）房间模式，与手机/客户端完全兼容
- 局域网 / UPnP 自动端口映射 / 公网 IP 均可使用
- **内置公网穿透**：控制台一键开启 Cloudflare 快速隧道，零配置获得公网地址，朋友跨网络直接连
- **绑定你自己的域名**：粘贴 Cloudflare 隧道 Token 即可把 `你的域名` 指向本服务器
- **服务器列表**：客户端可浏览并连接其他服务器（官方大厅 + 自定义）
- **Windows**：双击运行即驻留系统托盘，无窗口，可设置开机自启动
- **Linux**：二进制直接运行，或安装为 systemd 服务

## 下载

| 平台 | 文件 |
|------|------|
| Windows | `VoxelCraft-Server-Windows.zip`（含 `VoxelCraftServer.exe` 托盘版 + `VoxelCraftServer-Console.exe` 命令行版） |
| Linux   | `VoxelCraft-Server-Linux.tar.gz`（含 `voxelcraft-server` 可执行文件） |

GitHub Releases：https://github.com/HuHuBasic/voxelcraft/releases

## Windows 使用

1. 解压 `VoxelCraft-Server-Windows.zip`。
2. **双击 `VoxelCraftServer.exe`** 即可启动：
   - 不弹出任何窗口，程序常驻**系统托盘**（右下角绿色 V 图标）。
   - 自动打开浏览器控制台 `http://localhost:8080/dashboard`。
   - 托盘右键可：打开控制台 / 启动·停止服务器 / 查看日志 / 开机自启动 / 退出。
3. **开机自启动**：托盘右键勾选「开机自启动」，写入注册表 `HKCU\...\Run`，
   下次开机自动在托盘运行（不弹窗）。
4. **命令行 / 日志**：使用 `VoxelCraftServer-Console.exe`，日志直接输出到终端，
   或加参数运行：`VoxelCraftServer-Console.exe --cli`。

## Linux 使用

```bash
tar -xzf VoxelCraft-Server-Linux.tar.gz
cd voxelcraft-server-linux
./voxelcraft-server            # 前台运行，日志输出到终端
# 或后台常驻（推荐）：
./start.sh
# 或安装为开机自启服务（无图形界面服务器首选）：
./install-service.sh
```

- 有图形界面时自动显示托盘并打开控制台；纯服务器环境自动转为无界面模式，
  通过浏览器访问 `http://<服务器IP>:8080/dashboard` 管理。
- 数据（账号、日志、配置）保存在程序目录下的 `data/`。

## 命令行参数

| 参数 | 说明 |
|------|------|
| `--port 8080` | 服务端口（WS + API + 控制台共用） |
| `--cli` | 命令行模式：无托盘，前台运行，日志输出到终端 |
| `--no-tray` | 不显示托盘，后台运行服务器（无图形界面时使用） |
| `--no-open` | 启动后不自动打开控制台网页（自启时建议加） |
| `--datadir <路径>` | 指定数据目录 |
| `--install-autostart` | 开启开机自启动并退出 |
| `--uninstall-autostart` | 关闭开机自启动并退出 |
| `--version` | 显示版本 |

示例：`voxelcraft-server --cli --port 9000`

## 让朋友连接（含公网穿透）

**最简单（推荐）**：打开控制台 `http://localhost:8080/dashboard` →「公网穿透」→
**开启快速隧道**。几秒后会出现一个 `wss://xxxx.trycloudflare.com` 公网地址，
复制发给好友，他们在游戏里填这个地址（或服务器列表里选）即可跨网络联机。无需端口映射。

其他方式：

1. **同一 WiFi / 局域网**：把控制台显示的 `ws://<你的局域网IP>:8080` 发给好友，
   他们在游戏里填这个地址即可。
2. **UPnP / 手动端口转发**：路由器支持 UPnP 时自动映射，对方用 `ws://<你的公网IP>:8080` 连接；
   不支持则手动做 **TCP 8080 端口转发**到本机。
3. **绑定你自己的域名**：在 Cloudflare 创建隧道，把 Token 与 hostname（如 `play.basicgame.cc.cd`）
   填到控制台「公网穿透」的命名隧道里，域名即指向本服务器。

> 账号系统：玩家在游戏客户端注册 / 登录后获得令牌，服务器据此鉴权；未登录也能以游客身份游玩。
> 所有客户端版本均兼容——协议版本仅用于声明，服务端对任何版本都不拒绝。

## 控制台（仪表盘）功能

打开 `http://localhost:8080/dashboard`：

- 实时运行状态、端口、在线人数、已运行时长
- 一键 **启动 / 停止 / 重启** 服务器
- **公网穿透**：开启/停止快速隧道、启动命名隧道（绑域名），显示公网地址并一键复制
- **服务器列表**：查看官方大厅与已登记的服务器，添加自定义服务器（供客户端连接其他服务器）
- 修改服务器名称、欢迎语、最大人数、端口（保存即重启）
- 在线玩家列表、实时日志
- 开机自启动开关

## 兼容性

- 服务端协议版本：`1`，软件版本：`2.2.0`
- 兼容客户端协议版本：向下到 `1`，并向前兼容未来版本
- 与既有 `server.js` / Android ChinaServer 使用相同的账号、握手与消息协议，
  因此所有已发布及后续客户端均可直接连接。

## 数据目录结构

```
data/
  users.json      # 账号（用户名 / 密码哈希 / 昵称）
  config.json     # 服务器配置
  logs/
    server.log    # 运行日志
```
