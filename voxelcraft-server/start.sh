#!/usr/bin/env bash
# 前台启动服务器（终端关闭即停止）。如需后台常驻请用 install-service.sh 安装 systemd 服务。
cd "$(dirname "$0")"
chmod +x voxelcraft-server 2>/dev/null
./voxelcraft-server
