package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

var (
	log        *AppLogger
	gs         *GameServer
	appDataDir string
)

func main() {
	port := flag.Int("port", 8080, "游戏/服务端口（WS + HTTP API + 控制台共用）")
	dataDir := flag.String("datadir", "", "数据目录（默认：程序目录下 data）")
	cliMode := flag.Bool("cli", false, "命令行模式：无托盘，前台运行，日志输出到终端")
	noTray := flag.Bool("no-tray", false, "不显示托盘，后台运行服务器（无图形界面时使用）")
	noOpen := flag.Bool("no-open", false, "启动后不自动打开控制台网页")
	installAS := flag.Bool("install-autostart", false, "启用开机自启动并退出")
	uninstallAS := flag.Bool("uninstall-autostart", false, "取消开机自启动并退出")
	showVer := flag.Bool("version", false, "显示版本并退出")
	flag.Parse()

	if *showVer {
		fmt.Printf("VoxelCraft Server %s (protocol %d)\n", serverVersion, protocolVersion)
		return
	}

	// 数据目录
	if *dataDir == "" {
		exe, _ := os.Executable()
		appDataDir = filepath.Join(filepath.Dir(exe), "data")
	} else {
		appDataDir = *dataDir
	}
	_ = os.MkdirAll(appDataDir, 0o755)

	// 日志
	var err error
	log, err = NewLogger(filepath.Join(appDataDir, "logs", "server.log"), *cliMode)
	if err != nil {
		fmt.Println("日志初始化失败:", err)
	}

	// 游戏服务器
	gs = NewGameServer(*port, appDataDir)
	gs.loadConfig()

	// 自启动管理（独立动作，执行后退出）
	if *installAS {
		enableAutostart()
		return
	}
	if *uninstallAS {
		disableAutostart()
		return
	}

	log.Printf("VoxelCraft 服务器 v%s 启动中…", serverVersion)

	// 启动游戏服务器
	if err := gs.Start(); err != nil {
		log.Printf("启动失败: %v", err)
	}

	// 尝试 UPnP 自动端口映射（最佳努力）
	go tryUPnP(gs.port)

	// 运行模式
	wantTray := !*cliMode && !*noTray && systrayAvailable()
	if wantTray {
		runTray(*noOpen)
		return
	}

	// 无托盘（headless / CLI）：以服务方式运行，控制台网页仍可访问
	if !*cliMode && !*noOpen {
		openBrowser(dashboardURL())
	}
	if *cliMode {
		log.Printf("命令行模式：服务器运行中，Ctrl+C 退出。控制台: http://localhost:%d/dashboard", gs.port)
	} else {
		log.Printf("无界面模式：服务器运行中，控制台: http://localhost:%d/dashboard", gs.port)
	}
	waitForever()
}

func waitForever() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("收到退出信号，正在停止服务器…")
	if gs != nil {
		gs.Stop()
	}
	if log != nil {
		log.Close()
	}
	os.Exit(0)
}
