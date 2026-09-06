#!/usr/bin/env bash
# 安装为 systemd 用户服务（开机自启、崩溃自动重启、无图形界面也可用）。
set -e
EXE="$(realpath "$(dirname "$0")/voxelcraft-server")"
mkdir -p ~/.config/systemd/user
cat > ~/.config/systemd/user/voxelcraft-server.service <<EOF
[Unit]
Description=VoxelCraft Server
After=network.target

[Service]
ExecStart=$EXE --no-open
Restart=on-failure

[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now voxelcraft-server.service
echo "已安装并启动 systemd 用户服务。"
echo "查看状态: systemctl --user status voxelcraft-server"
echo "停止服务: systemctl --user stop voxelcraft-server"
echo "关闭自启: systemctl --user disable voxelcraft-server"
