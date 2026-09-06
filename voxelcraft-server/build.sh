#!/usr/bin/env bash
# 构建脚本：生成 Linux 二进制 + 压缩包，以及 Windows exe（GUI 托盘版 + 控制台版）与 zip。
set -e
cd "$(dirname "$0")"

APP=voxelcraft-server
rm -rf dist pkg
mkdir -p dist

echo "==> go mod tidy"
go mod tidy

echo "==> 构建 Linux (amd64, CGO + GTK 托盘)"
CGO_ENABLED=1 go build -o "dist/$APP" .

echo "==> 构建 Windows GUI 版 (无控制台窗口 / 托盘常驻)"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui -s -w" -o dist/VoxelCraftServer.exe .

echo "==> 构建 Windows 控制台版 (命令行 / 日志)"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-H=console -s -w" -o dist/VoxelCraftServer-Console.exe .

# ---------- Linux 包 ----------
mkdir -p "pkg/$APP-linux"
cp "dist/$APP" "pkg/$APP-linux/"
cp README.md "pkg/$APP-linux/"
cp start.sh "pkg/$APP-linux/"
cp install-service.sh "pkg/$APP-linux/"
cp voxelcraft-server.service "pkg/$APP-linux/"
tar -czf "dist/VoxelCraft-Server-Linux.tar.gz" -C pkg "$APP-linux"

# ---------- Windows 包 ----------
mkdir -p "pkg/$APP-win"
cp dist/VoxelCraftServer.exe "pkg/$APP-win/"
cp dist/VoxelCraftServer-Console.exe "pkg/$APP-win/"
cp README.md "pkg/$APP-win/README.txt"
cp start-windows.bat "pkg/$APP-win/"
cd pkg && zip -rq "../dist/VoxelCraft-Server-Windows.zip" "$APP-win" && cd ..

echo "==> 校验"
( cd dist && sha256sum * )

echo "完成。产物在 dist/："
ls -lh dist/
