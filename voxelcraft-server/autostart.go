package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const autostartKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
const autostartValue = "VoxelCraftServer"

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return "voxelcraft-server"
	}
	ap, _ := filepath.Abs(p)
	return ap
}

// enableAutostart 开启开机自启动
func enableAutostart() {
	switch runtime.GOOS {
	case "windows":
		data := `"` + exePath() + `" --no-open`
		_ = exec.Command("reg", "add", autostartKey, "/v", autostartValue, "/t", "REG_SZ", "/d", data, "/f").Run()
		log.Printf("[自启动] 已写入注册表: %s", data)
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dir := filepath.Join(home, ".config", "autostart")
		_ = os.MkdirAll(dir, 0o755)
		exe := exePath()
		exec := exe
		if strings.Contains(exe, " ") {
			exec = `"` + exe + `"`
		}
		content := "[Desktop Entry]\n" +
			"Type=Application\n" +
			"Name=VoxelCraft Server\n" +
			"Comment=VoxelCraft 方块世界服务器\n" +
			"Exec=" + exec + " --no-open\n" +
			"Hidden=false\n" +
			"X-GNOME-Autostart-enabled=true\n" +
			"Terminal=false\n"
		_ = os.WriteFile(filepath.Join(dir, "voxelcraft-server.desktop"), []byte(content), 0o644)
		// 同时尝试 systemd 用户服务（服务器/无桌面环境更可靠）
		enableSystemdUser(exe)
		log.Printf("[自启动] 已写入 ~/.config/autostart/voxelcraft-server.desktop")
	}
}

func disableAutostart() {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("reg", "delete", autostartKey, "/v", autostartValue, "/f").Run()
		log.Printf("[自启动] 已移除注册表项")
	case "linux":
		home, _ := os.UserHomeDir()
		if home != "" {
			_ = os.Remove(filepath.Join(home, ".config", "autostart", "voxelcraft-server.desktop"))
		}
		disableSystemdUser()
		log.Printf("[自启动] 已关闭")
	}
}

func isAutostartEnabled() bool {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("reg", "query", autostartKey, "/v", autostartValue).Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), autostartValue)
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		_, err = os.Stat(filepath.Join(home, ".config", "autostart", "voxelcraft-server.desktop"))
		return err == nil
	}
	return false
}

// ---- Linux systemd 用户服务（可选，提升在无桌面环境下的自启可靠性）----
func enableSystemdUser(exe string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	_ = os.MkdirAll(dir, 0o755)
	content := "[Unit]\n" +
		"Description=VoxelCraft Server\n" +
		"After=network.target\n\n" +
		"[Service]\n" +
		"ExecStart=" + exe + " --no-open\n" +
		"Restart=on-failure\n\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
	_ = os.WriteFile(filepath.Join(dir, "voxelcraft-server.service"), []byte(content), 0o644)
	_ = exec.Command("systemctl", "--user", "enable", "voxelcraft-server.service").Run()
}

func disableSystemdUser() {
	_ = exec.Command("systemctl", "--user", "disable", "voxelcraft-server.service").Run()
	home, _ := os.UserHomeDir()
	if home != "" {
		_ = os.Remove(filepath.Join(home, ".config", "systemd", "user", "voxelcraft-server.service"))
	}
}
