//go:build !windows

package main

import "os/exec"

// hideWindow 非 Windows 平台为空操作。
func hideWindow(cmd *exec.Cmd) {}
