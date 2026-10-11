package main

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
)

// ---------- 公网穿透 ----------
func (s *GameServer) apiTunnel(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if tunnelMgr == nil {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "隧道未初始化"})
		return
	}
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method == "GET" {
		writeJSON(w, tunnelMgr.Status())
		return
	}
	var body struct {
		Action   string `json:"action"`
		Token    string `json:"token"`
		Hostname string `json:"hostname"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	switch body.Action {
	case "start_quick":
		if err := tunnelMgr.StartQuick(); err != nil {
			writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
			return
		}
		writeJSON(w, tunnelMgr.Status())
	case "start_named":
		if err := tunnelMgr.StartNamed(body.Token, body.Hostname); err != nil {
			writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
			return
		}
		writeJSON(w, tunnelMgr.Status())
	case "stop":
		tunnelMgr.Stop()
		writeJSON(w, tunnelMgr.Status())
	default:
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "未知操作"})
	}
}

// ---------- 服务器注册表（供游戏客户端"连接其他服务器"） ----------
type serverEntry struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Note    string `json:"note"`
	Official bool  `json:"official"`
}

var serversMu sync.Mutex

func (s *GameServer) serversPath() string { return s.dataDir + "/servers.json" }

func (s *GameServer) loadServers() []serverEntry {
	serversMu.Lock()
	defer serversMu.Unlock()
	data, err := os.ReadFile(s.serversPath())
	if err != nil {
		return []serverEntry{}
	}
	var list []serverEntry
	if json.Unmarshal(data, &list) == nil {
		return list
	}
	return []serverEntry{}
}

func (s *GameServer) saveServers(list []serverEntry) {
	serversMu.Lock()
	defer serversMu.Unlock()
	data, _ := json.MarshalIndent(list, "", "  ")
	_ = os.WriteFile(s.serversPath(), data, 0o644)
}

func (s *GameServer) apiServers(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	if r.Method == "POST" {
		var e serverEntry
		_ = json.NewDecoder(r.Body).Decode(&e)
		if e.Name == "" || e.URL == "" {
			writeJSON(w, map[string]interface{}{"ok": false, "msg": "请填写服务器名称和地址"})
			return
		}
		list := s.loadServers()
		list = append(list, serverEntry{Name: e.Name, URL: e.URL, Note: e.Note})
		s.saveServers(list)
		writeJSON(w, map[string]interface{}{"ok": true, "servers": list})
		return
	}
	// GET：返回已登记服务器 + 本服务器自身对外地址（若隧道已开启）
	list := s.loadServers()
	if len(list) == 0 {
		// 内置默认公共服务器，确保"连接其他服务器"开箱即用
		list = []serverEntry{
			{Name: "VoxelCraft 官方大厅", URL: "wss://basicgame.cc.cd", Note: "官方公共服务器", Official: true},
			{Name: "VoxelCraft 备用(HTTP)", URL: "ws://basicgame.cc.cd:8080", Note: "官方备用地址", Official: true},
		}
	}
	if tunnelMgr != nil {
		st := tunnelMgr.Status()
		if u, ok := st["publicURL"].(string); ok && u != "" {
			list = append([]serverEntry{{Name: s.serverName + "（本服务器·公网）", URL: u, Note: "一键穿透地址", Official: false}}, list...)
		}
	}
	writeJSON(w, map[string]interface{}{"ok": true, "servers": list})
}

// ---------- 域名绑定（Cloudflare DNS） ----------
func (s *GameServer) apiDNS(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var body struct {
		Token  string `json:"token"`
		Zone   string `json:"zone"`
		Sub    string `json:"sub"`
		Target string `json:"target"`
		Type   string `json:"type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	full, err := cloudflareUpsertDNS(body.Token, body.Zone, body.Sub, body.Target, body.Type)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "record": full, "msg": "DNS 已更新，稍候全球生效"})
}
