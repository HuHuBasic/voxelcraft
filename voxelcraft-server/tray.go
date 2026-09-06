package main

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/getlantern/systray"
)

//go:embed icon.png
var iconData []byte

// trayNoOpen 由 runTray 设置，onTrayReady 在启动后据此决定是否自动打开控制台。
var trayNoOpen bool

// systrayAvailable 判断当前环境是否可显示系统托盘。
func systrayAvailable() bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return true
	}
	// Linux 需要图形会话
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return false
}

func runTray(noOpen bool) {
	trayNoOpen = noOpen
	systray.Run(onTrayReady, nil)
}

func dashboardURL() string {
	return "http://localhost:" + itoa(gs.port) + "/dashboard"
}

func onTrayReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("VoxelCraft 服务器")
	systray.SetTooltip("VoxelCraft 服务器 · 右键菜单操作")

	mOpen := systray.AddMenuItem("打开控制台", "在浏览器中打开服务器控制台")
	mStart := systray.AddMenuItem("启动服务器", "启动游戏服务器")
	mStop := systray.AddMenuItem("停止服务器", "停止游戏服务器")
	mLogs := systray.AddMenuItem("查看日志", "打开日志文件")
	mAuto := systray.AddMenuItemCheckbox("开机自启动", "切换开机自启动", isAutostartEnabled())
	mQuit := systray.AddMenuItem("退出", "退出服务器")

	if !trayNoOpen {
		openBrowser(dashboardURL())
	}

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser(dashboardURL())
			case <-mStart.ClickedCh:
				gs.Start()
			case <-mStop.ClickedCh:
				gs.Stop()
			case <-mLogs.ClickedCh:
				openFile(filepath.Join(appDataDir, "logs", "server.log"))
			case <-mAuto.ClickedCh:
				if mAuto.Checked() {
					disableAutostart()
					mAuto.Uncheck()
				} else {
					enableAutostart()
					mAuto.Check()
				}
			case <-mQuit.ClickedCh:
				gs.Stop()
				log.Close()
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}

// openBrowser 用系统默认浏览器打开 URL（无需第三方依赖）
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}

// openFile 用系统默认程序打开文件/文件夹
func openFile(path string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", path}
	case "darwin":
		cmd = "open"
		args = []string{path}
	default:
		cmd = "xdg-open"
		args = []string{path}
	}
	_ = exec.Command(cmd, args...).Start()
}
