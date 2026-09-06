package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AppLogger 同时写文件、内存环形缓冲（供仪表盘读取）与终端。
type AppLogger struct {
	mu      sync.Mutex
	file    *os.File
	ring    []string
	ringCap int
	console bool
}

func NewLogger(path string, console bool) (*AppLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &AppLogger{file: f, ring: make([]string, 0, 4000), ringCap: 4000, console: console}, nil
}

func (a *AppLogger) Write(line string) {
	t := time.Now().Format("2006-01-02 15:04:05")
	full := "[" + t + "] " + line
	a.mu.Lock()
	a.ring = append(a.ring, full)
	if len(a.ring) > a.ringCap {
		a.ring = a.ring[a.ringCap/2:]
	}
	a.mu.Unlock()
	if a.console {
		fmt.Println(full)
	}
	if a.file != nil {
		a.file.WriteString(full + "\n")
		a.file.Sync()
	}
}

func (a *AppLogger) Printf(format string, args ...interface{}) { a.Write(fmt.Sprintf(format, args...)) }

// Recent 返回最近 n 条日志（n<=0 表示全部）
func (a *AppLogger) Recent(n int) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n <= 0 || n >= len(a.ring) {
		out := make([]string, len(a.ring))
		copy(out, a.ring)
		return out
	}
	out := make([]string, n)
	copy(out, a.ring[len(a.ring)-n:])
	return out
}

func (a *AppLogger) Close() {
	if a.file != nil {
		a.file.Close()
	}
}
