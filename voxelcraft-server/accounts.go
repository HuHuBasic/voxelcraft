package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var reUser = regexp.MustCompile(`^[a-zA-Z0-9_\x{4e00}-\x{9fa5}]{2,16}$`)

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	setCORS(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *GameServer) apiRegister(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Username == "" || body.Password == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "请填写账号和密码"})
		return
	}
	if !reUser.MatchString(body.Username) {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "账号需2-16位字母/数字/中文"})
		return
	}
	if len(body.Password) < 4 || len(body.Password) > 20 {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "密码需4-20位"})
		return
	}
	s.mu.Lock()
	if _, exists := s.users[body.Username]; exists {
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "账号已存在"})
		return
	}
	name := body.Name
	if name == "" {
		name = body.Username
	}
	name = name[:min(len(name), 12)]
	s.users[body.Username] = &User{
		Username:     body.Username,
		PasswordHash: sha256hex(body.Password),
		Name:         name,
		CreatedAt:    time.Now().UnixMilli(),
	}
	s.mu.Unlock()
	s.saveUsers()

	token := randToken()
	s.mu.Lock()
	s.tokens[token] = body.Username
	s.mu.Unlock()
	log.Printf("[注册] %s", body.Username)
	writeJSON(w, map[string]interface{}{"ok": true, "token": token, "user": s.users[body.Username]})
}

func (s *GameServer) apiLogin(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(200)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Username == "" || body.Password == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "请填写账号和密码"})
		return
	}
	s.mu.Lock()
	user, exists := s.users[body.Username]
	s.mu.Unlock()
	if !exists {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "账号不存在"})
		return
	}
	if user.PasswordHash != sha256hex(body.Password) {
		writeJSON(w, map[string]interface{}{"ok": false, "msg": "密码错误"})
		return
	}
	token := randToken()
	s.mu.Lock()
	s.tokens[token] = body.Username
	s.mu.Unlock()
	log.Printf("[登录] %s", body.Username)
	writeJSON(w, map[string]interface{}{"ok": true, "token": token, "user": user})
}

func (s *GameServer) apiCheck(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	username, ok := s.tokens[token]
	user := s.users[username]
	s.mu.Unlock()
	if !ok || user == nil {
		writeJSON(w, map[string]interface{}{"ok": false})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "user": user})
}

func (s *GameServer) apiPing(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	writeJSON(w, map[string]interface{}{"ok": true, "t": time.Now().UnixMilli()})
}

func (s *GameServer) apiVersion(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	writeJSON(w, map[string]interface{}{
		"ok":          true,
		"protocol":    protocolVersion,
		"server":      serverVersion,
		"minClient":   minClientProto,
		"gameCompat":  gameCompat,
		"compatible":  true,
		"crossNetwork": true,
	})
}

func (s *GameServer) apiRelayTxt(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	data, err := os.ReadFile(s.dataDir + "/relay_ws.txt")
	if err != nil {
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// serveLanding 提供简单的状态首页（浏览器直接访问端口时看到）
func (s *GameServer) serveLanding(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!doctype html><html lang="zh"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>VoxelCraft 服务器</title>
<style>body{font-family:system-ui,sans-serif;background:#0f1220;color:#e8eaf0;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
.card{background:#1a1f33;padding:32px 40px;border-radius:16px;box-shadow:0 10px 40px rgba(0,0,0,.4);text-align:center;max-width:480px}
h1{margin:0 0 8px;font-size:24px}.v{color:#2ecc71}.m{color:#9aa3b2;margin:6px 0 20px}
a{display:inline-block;margin-top:8px;background:#2ecc71;color:#06210f;padding:10px 18px;border-radius:10px;text-decoration:none;font-weight:700}
</style></head><body><div class="card">
<h1>VoxelCraft <span class="v">服务器</span></h1>
<p class="m">` + s.serverName + ` · ` + s.motd + `</p>
<p class="m">游戏端口 <b>` + itoa(s.port) + `</b> · 协议 v` + itoa(protocolVersion) + ` · 兼容所有客户端版本</p>
<a href="/dashboard">进入控制台</a>
</div></body></html>`
	w.Write([]byte(html))
}

func randToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func itoa(n int) string { return strconv.Itoa(n) }
