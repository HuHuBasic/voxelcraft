package main

import (
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed dashboard.html
var dashboardHTML []byte

type config struct {
	ServerName string `json:"serverName"`
	MOTD       string `json:"motd"`
	MaxPlayers int    `json:"maxPlayers"`
	Port       int    `json:"port"`
}

func (s *GameServer) configPath() string { return s.dataDir + "/config.json" }

func (s *GameServer) loadConfig() {
	data, err := os.ReadFile(s.configPath())
	if err != nil {
		return
	}
	var c config
	if json.Unmarshal(data, &c) == nil {
		if c.ServerName != "" {
			s.serverName = c.ServerName
		}
		if c.MOTD != "" {
			s.motd = c.MOTD
		}
		if c.MaxPlayers > 0 {
			s.maxPlayers = c.MaxPlayers
		}
		if c.Port > 0 {
			s.port = c.Port
		}
	}
}

func (s *GameServer) saveConfig() {
	c := config{ServerName: s.serverName, MOTD: s.motd, MaxPlayers: s.maxPlayers, Port: s.port}
	data, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(s.configPath(), data, 0o644)
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func getPublicIP() string {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	return strings.TrimSpace(string(buf[:n]))
}

func (s *GameServer) serveDashboard(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}

func (s *GameServer) apiStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	running := s.running
	port := s.port
	count := 0
	for c := range s.clients {
		if !c.player.IsHost && !c.player.RoomClient {
			count++
		}
	}
	uptime := int64(0)
	if running {
		uptime = time.Since(s.startedAt).Milliseconds()
	}
	serverName := s.serverName
	motd := s.motd
	maxP := s.maxPlayers
	s.mu.Unlock()

	// 注意：publicPlayers() 内部会自行加锁，必须在本函数释放锁之后再调用
	names := s.publicPlayers()

	writeJSON(w, map[string]interface{}{
		"ok":         true,
		"running":    running,
		"port":       port,
		"players":    count,
		"playerList": names,
		"uptime":     uptime,
		"version":    serverVersion,
		"protocol":   protocolVersion,
		"serverName": serverName,
		"motd":       motd,
		"maxPlayers": maxP,
		"lanIP":      getLocalIP(),
		"publicIP":   getPublicIP(),
		"autostart":  isAutostartEnabled(),
		"wsURL":      "ws://" + getLocalIP() + ":" + itoa(port),
		"compatible": true,
	})
}

func (s *GameServer) apiControl(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	switch body.Action {
	case "stop":
		s.Stop()
		writeJSON(w, map[string]interface{}{"ok": true, "running": false})
	case "start":
		if err := s.Start(); err != nil {
			writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true, "running": true})
	case "restart":
		if err := s.Restart(); err != nil {
			writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true, "running": true})
	default:
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "未知操作"})
	}
}

func (s *GameServer) apiLogs(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	n := 300
	if v := r.URL.Query().Get("tail"); v != "" {
		if m, err := strconv.Atoi(v); err == nil && m > 0 {
			n = m
		}
	}
	writeJSON(w, map[string]interface{}{"ok": true, "logs": log.Recent(n)})
}

func (s *GameServer) apiPlayers(w http.ResponseWriter, r *http.Request) {
	// publicPlayers() 内部自行加锁，不要在此处再持锁
	names := s.publicPlayers()
	writeJSON(w, map[string]interface{}{"ok": true, "players": names})
}

func (s *GameServer) apiSettings(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var body struct {
		ServerName string `json:"serverName"`
		MOTD       string `json:"motd"`
		MaxPlayers int    `json:"maxPlayers"`
		Port       int    `json:"port"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	s.mu.Lock()
	if body.ServerName != "" {
		s.serverName = body.ServerName
	}
	if body.MOTD != "" {
		s.motd = body.MOTD
	}
	if body.MaxPlayers > 0 {
		s.maxPlayers = body.MaxPlayers
	}
	portChanged := false
	if body.Port > 0 && body.Port != s.port {
		s.port = body.Port
		portChanged = true
	}
	s.mu.Unlock()
	s.saveConfig()

	if portChanged {
		if err := s.Restart(); err != nil {
			writeJSON(w, map[string]interface{}{"ok": false, "msg": "端口更改失败: " + err.Error()})
			return
		}
	}
	writeJSON(w, map[string]interface{}{"ok": true, "port": s.port, "restarted": portChanged})
}

func (s *GameServer) apiAutostart(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Enabled {
		enableAutostart()
	} else {
		disableAutostart()
	}
	writeJSON(w, map[string]interface{}{"ok": true, "enabled": isAutostartEnabled()})
}
