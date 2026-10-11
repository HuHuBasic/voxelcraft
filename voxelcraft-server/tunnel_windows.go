//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindow 在 Windows 上让 cloudflared 子进程不弹出控制台窗口。
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
