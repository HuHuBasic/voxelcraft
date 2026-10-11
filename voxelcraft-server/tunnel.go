package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TunnelManager 内置公网穿透（Cloudflare Tunnel）。
// - 快速隧道：零配置，自动获得一个 *.trycloudflare.com 公网地址；
// - 命名隧道：粘贴 Cloudflare 隧道 Token，将你的域名绑定到本服务器。
// cloudflared 二进制在首次使用时自动下载并缓存到 data 目录。
type TunnelManager struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	publicURL string // 供客户端连接用的 wss:// 地址
	mode      string // "" | "quick" | "named"
	hostname  string
	token     string
	status    string // "idle" | "starting" | "running" | "error"
	lastErr   string
	logBuf    []string
	port      int
	dataDir   string
}

var tunnelMgr *TunnelManager

func NewTunnelManager(port int, dataDir string) *TunnelManager {
	return &TunnelManager{port: port, dataDir: dataDir, status: "idle"}
}

func (t *TunnelManager) cloudflaredPath() string {
	if p := os.Getenv("CLOUDFLARED_BIN"); p != "" {
		return p
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return filepath.Join(t.dataDir, "cloudflared"+ext)
}

// ensureBinary 确保 cloudflared 存在，不存在则下载。
func (t *TunnelManager) ensureBinary() error {
	p := t.cloudflaredPath()
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	var url string
	switch runtime.GOOS {
	case "windows":
		url = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			url = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-arm64"
		} else {
			url = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-amd64"
		}
	default:
		if runtime.GOARCH == "arm64" {
			url = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64"
		} else {
			url = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
		}
	}
	log.Printf("[隧道] 正在下载 cloudflared → %s", url)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("下载 cloudflared 失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("下载 cloudflared 失败: HTTP %d", resp.StatusCode)
	}
	tmp := p + ".download"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		return err
	}
	log.Printf("[隧道] cloudflared 已就绪: %s", p)
	return nil
}

var reTunnelURL = regexp.MustCompile(`https://[a-zA-Z0-9._-]+\.trycloudflare\.com`)

func (t *TunnelManager) scanOutput(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 256*1024)
	for sc.Scan() {
		line := sc.Text()
		t.mu.Lock()
		t.logBuf = append(t.logBuf, line)
		if len(t.logBuf) > 200 {
			t.logBuf = t.logBuf[len(t.logBuf)-200:]
		}
		if t.publicURL == "" {
			if m := reTunnelURL.FindString(line); m != "" {
				t.publicURL = "wss://" + strings.TrimPrefix(m, "https://")
				t.status = "running"
				log.Printf("[隧道] 公网地址已就绪: %s", t.publicURL)
			}
		}
		if t.status == "starting" && strings.Contains(line, "Connection registered") {
			t.status = "running"
		}
		t.mu.Unlock()
	}
}

// StartQuick 启动快速隧道（零配置公网地址）。
func (t *TunnelManager) StartQuick() error {
	t.mu.Lock()
	if t.cmd != nil {
		t.mu.Unlock()
		return fmt.Errorf("隧道已在运行")
	}
	t.mode = "quick"
	t.status = "starting"
	t.publicURL = ""
	t.lastErr = ""
	t.logBuf = nil
	t.mu.Unlock()

	if err := t.ensureBinary(); err != nil {
		t.mu.Lock()
		t.status = "error"
		t.lastErr = err.Error()
		t.mu.Unlock()
		return err
	}
	bin := t.cloudflaredPath()
	cmd := exec.Command(bin, "tunnel", "--url", "http://localhost:"+itoa(t.port), "--no-autoupdate", "--loglevel", "info")
	hideWindow(cmd)
	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil {
		t.mu.Lock()
		t.status = "error"
		t.lastErr = "无法获取隧道输出"
		t.mu.Unlock()
		return fmt.Errorf("无法获取隧道输出")
	}
	if err := cmd.Start(); err != nil {
		t.mu.Lock()
		t.status = "error"
		t.lastErr = err.Error()
		t.mu.Unlock()
		return err
	}
	t.mu.Lock()
	t.cmd = cmd
	t.mu.Unlock()
	go t.scanOutput(io.MultiReader(stdout, stderr))
	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		if t.cmd == cmd {
			t.cmd = nil
			if t.status != "error" {
				t.status = "idle"
			}
			t.publicURL = ""
		}
		t.mu.Unlock()
	}()

	// 等待解析出公网地址（最多 ~25 秒）
	for i := 0; i < 50; i++ {
		t.mu.Lock()
		u := t.publicURL
		st := t.status
		t.mu.Unlock()
		if u != "" || st == "error" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.publicURL == "" && t.status == "error" {
		return fmt.Errorf("%s", t.lastErr)
	}
	if t.publicURL == "" {
		return fmt.Errorf("隧道启动中，请稍候在控制台查看公网地址")
	}
	return nil
}

// StartNamed 启动命名隧道（绑定你自己的域名，需在 Cloudflare 中为隧道配置 hostname）。
func (t *TunnelManager) StartNamed(token, hostname string) error {
	token = strings.TrimSpace(token)
	hostname = strings.TrimSpace(hostname)
	if token == "" {
		return fmt.Errorf("请填写 Cloudflare 隧道 Token")
	}
	if hostname == "" {
		return fmt.Errorf("请填写你的域名（在 Cloudflare 隧道中配置的 hostname）")
	}
	t.mu.Lock()
	if t.cmd != nil {
		t.mu.Unlock()
		return fmt.Errorf("隧道已在运行")
	}
	t.mode = "named"
	t.status = "starting"
	t.publicURL = "wss://" + hostname
	t.hostname = hostname
	t.token = token
	t.lastErr = ""
	t.logBuf = nil
	t.mu.Unlock()

	if err := t.ensureBinary(); err != nil {
		t.mu.Lock()
		t.status = "error"
		t.lastErr = err.Error()
		t.mu.Unlock()
		return err
	}
	bin := t.cloudflaredPath()
	cmd := exec.Command(bin, "tunnel", "run", "--token", token)
	hideWindow(cmd)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.mu.Lock()
		t.status = "error"
		t.lastErr = err.Error()
		t.mu.Unlock()
		return err
	}
	t.mu.Lock()
	t.cmd = cmd
	t.status = "running"
	t.mu.Unlock()
	go t.scanOutput(io.MultiReader(stdout, stderr))
	go func() {
		_ = cmd.Wait()
		t.mu.Lock()
		if t.cmd == cmd {
			t.cmd = nil
			if t.status != "error" {
				t.status = "idle"
			}
			t.publicURL = ""
		}
		t.mu.Unlock()
	}()
	return nil
}

// Stop 停止隧道。
func (t *TunnelManager) Stop() {
	t.mu.Lock()
	cmd := t.cmd
	t.cmd = nil
	t.status = "idle"
	t.publicURL = ""
	t.mode = ""
	t.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
}

// Status 返回当前隧道状态快照。
func (t *TunnelManager) Status() map[string]interface{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]interface{}{
		"ok":        true,
		"status":    t.status,
		"mode":      t.mode,
		"publicURL": t.publicURL,
		"hostname":  t.hostname,
		"lastErr":   t.lastErr,
		"logs":      t.logBuf,
	}
}
