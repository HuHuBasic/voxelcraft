package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ---- 版本与兼容性 ----
// 服务端永不因协议版本不符而拒绝客户端；对缺失/多余字段做防御式解析；
// 未知消息类型直接忽略或透传。这样无论更老、当前还是未来的客户端都能联机且不崩溃。
const (
	protocolVersion = 1      // 当前服务端网络协议版本
	serverVersion   = "2.1.0" // 服务端软件版本
	minClientProto  = 1      // 兼容的最低客户端协议版本
	gameCompat      = "1.0.0+"
	tickRate        = 50 * time.Millisecond
)

var colors = []string{
	"#e74c3c", "#3498db", "#2ecc71", "#f39c12", "#9b59b6",
	"#1abc9c", "#e67e22", "#2980b9", "#27ae60", "#d35400",
	"#8e44ad", "#16a085", "#c0392b", "#f1c40f", "#7f8c8d",
}

type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	Name         string `json:"name"`
	CreatedAt    int64  `json:"createdAt"`
	Op           bool   `json:"op"`
}

type Player struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Username  string  `json:"username"`
	Color     string  `json:"color"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Pitch     float64 `json:"pitch"`
	Yaw       float64 `json:"yaw"`
	LastMove  int64   `json:"-"`
	Pv        int     `json:"-"`
	Gv        string  `json:"-"`
	IsHost    bool    `json:"-"`
	RoomClient bool   `json:"-"`
	Room      string  `json:"-"`
	Cid       int     `json:"-"`
	Handled   bool    `json:"-"`
}

type client struct {
	conn   *websocket.Conn
	player *Player
	hello  *time.Timer
}

type GameServer struct {
	port    int
	dataDir string

	mu          sync.Mutex
	httpServer  *http.Server
	listener    net.Listener
	upgrader    websocket.Upgrader
	clients     map[*client]bool
	worldBlocks map[string]int
	hosts       map[string]*client
	users       map[string]*User
	tokens      map[string]string // token -> username

	running    bool
	httpUp     bool
	serverName string
	motd       string
	maxPlayers int
	nextID     int
	nextCid    int
	startedAt  time.Time
}

func NewGameServer(port int, dataDir string) *GameServer {
	s := &GameServer{
		port:        port,
		dataDir:     dataDir,
		clients:     make(map[*client]bool),
		worldBlocks: make(map[string]int),
		hosts:       make(map[string]*client),
		users:       make(map[string]*User),
		tokens:      make(map[string]string),
		serverName:  "VoxelCraft 服务器",
		motd:        "欢迎来到方块世界！",
		maxPlayers:  50,
		nextID:      1,
		nextCid:     1,
	}
	s.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	s.loadUsers()
	return s
}

// ---------- 用户/令牌持久化 ----------
func (s *GameServer) usersPath() string { return s.dataDir + "/users.json" }

func (s *GameServer) loadUsers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.usersPath())
	if err == nil {
		_ = json.Unmarshal(data, &s.users)
	}
	if s.users == nil {
		s.users = make(map[string]*User)
	}
}

func (s *GameServer) saveUsers() {
	data, err := json.MarshalIndent(s.users, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.usersPath(), data, 0o644)
}

// ---------- 路由 ----------
func (s *GameServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.rootHandler)
	mux.HandleFunc("/api/register", s.apiRegister)
	mux.HandleFunc("/api/login", s.apiLogin)
	mux.HandleFunc("/api/check", s.apiCheck)
	mux.HandleFunc("/api/ping", s.apiPing)
	mux.HandleFunc("/api/version", s.apiVersion)
	mux.HandleFunc("/relay.txt", s.apiRelayTxt)
	mux.HandleFunc("/dashboard", s.serveDashboard)
	mux.HandleFunc("/api/status", s.apiStatus)
	mux.HandleFunc("/api/control", s.apiControl)
	mux.HandleFunc("/api/logs", s.apiLogs)
	mux.HandleFunc("/api/players", s.apiPlayers)
	mux.HandleFunc("/api/settings", s.apiSettings)
	mux.HandleFunc("/api/autostart", s.apiAutostart)
	return mux
}

func (s *GameServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		s.handleWS(w, r)
		return
	}
	s.serveLanding(w, r)
}

// ---------- 启动/停止 ----------
// Start 绑定 HTTP 监听（仅首次）并开启游戏服务。
// 停止时只关闭游戏连接，HTTP 控制台/API 始终存活，以便随时重新启动。
func (s *GameServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	if !s.httpUp {
		ln, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(s.port))
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.listener = ln
		s.httpServer = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
		s.httpUp = true
		s.mu.Unlock()
		log.Printf("监听已启动 → http://0.0.0.0:%d (WS+API+控制台)", s.port)
		go s.httpServer.Serve(ln)
		go s.tickLoop()
		s.mu.Lock()
	}
	s.running = true
	s.startedAt = time.Now()
	s.mu.Unlock()
	log.Printf("游戏服务器已启动（端口 %d）", s.port)
	return nil
}

// Stop 仅停止游戏服务，保留 HTTP 控制台/API，便于仪表盘再次启动。
func (s *GameServer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	for c := range s.clients {
		_ = c.conn.Close()
	}
	s.clients = make(map[*client]bool)
	s.hosts = make(map[string]*client)
	s.mu.Unlock()
	log.Printf("游戏服务器已停止（控制台仍可访问）")
}

// Restart 仅在更改端口等需重绑定时使用：关闭旧监听并重新绑定。
func (s *GameServer) Restart() error {
	s.Stop()
	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	if s.httpServer != nil {
		_ = s.httpServer.Close()
		s.httpServer = nil
	}
	s.httpUp = false
	s.mu.Unlock()
	time.Sleep(400 * time.Millisecond)
	return s.Start()
}

func (s *GameServer) tickLoop() {
	t := time.NewTicker(tickRate)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		if !s.running {
			s.mu.Unlock()
			continue
		}
		now := time.Now().UnixNano() / int64(time.Millisecond)
		var positions []map[string]interface{}
		for c := range s.clients {
			p := c.player
			if p.IsHost || p.RoomClient {
				continue
			}
			if now-p.LastMove < 2000 {
				positions = append(positions, map[string]interface{}{
					"id": p.ID, "x": p.X, "y": p.Y, "z": p.Z, "pitch": p.Pitch, "yaw": p.Yaw,
				})
			}
		}
		s.mu.Unlock()
		if len(positions) > 0 {
			s.broadcastAll(map[string]interface{}{"type": "players_move", "players": positions})
		}
	}
}

// ---------- 广播 ----------
func (s *GameServer) broadcast(data map[string]interface{}, exclude *client) {
	msg, err := json.Marshal(data)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if c == exclude {
			continue
		}
		if c.conn.WriteMessage(websocket.TextMessage, msg) != nil {
			_ = c.conn.Close()
			delete(s.clients, c)
		}
	}
}

func (s *GameServer) broadcastAll(data map[string]interface{}) {
	s.broadcast(data, nil)
}

func (s *GameServer) publicPlayers() []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []map[string]interface{}
	for c := range s.clients {
		p := c.player
		if p.IsHost || p.RoomClient {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": p.ID, "name": p.Name, "color": p.Color, "username": p.Username,
			"x": p.X, "y": p.Y, "z": p.Z, "pitch": p.Pitch, "yaw": p.Yaw,
		})
	}
	return list
}

func (s *GameServer) roomClients(room string) []*client {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*client
	for c := range s.clients {
		if c.player.RoomClient && c.player.Room == room {
			list = append(list, c)
		}
	}
	return list
}

func (s *GameServer) routeHostToRoom(room string, data map[string]interface{}) {
	clients := s.roomClients(room)
	var targets []*client
	if to, ok := data["to"]; ok {
		switch v := to.(type) {
		case []interface{}:
			cids := map[int]bool{}
			for _, e := range v {
				if f, ok := e.(float64); ok {
					cids[int(f)] = true
				}
			}
			for _, c := range clients {
				if cids[c.player.Cid] {
					targets = append(targets, c)
				}
			}
		case float64:
			for _, c := range clients {
				if c.player.Cid == int(v) {
					targets = append(targets, c)
				}
			}
		}
	} else {
		targets = clients
	}
	out := map[string]interface{}{}
	for k, val := range data {
		if k == "to" {
			continue
		}
		out[k] = val
	}
	msg, err := json.Marshal(out)
	if err != nil {
		return
	}
	for _, c := range targets {
		if c.conn.WriteMessage(websocket.TextMessage, msg) != nil {
			_ = c.conn.Close()
		}
	}
}

func (s *GameServer) sendPublicInit(c *client) {
	s.mu.Lock()
	p := c.player
	if p == nil {
		s.mu.Unlock()
		return
	}
	blocks := make([][2]interface{}, 0, len(s.worldBlocks))
	for k, id := range s.worldBlocks {
		blocks = append(blocks, [2]interface{}{k, id})
	}
	players := s.publicPlayers()
	s.mu.Unlock()
	c.conn.WriteJSON(map[string]interface{}{
		"type":          "init",
		"playerId":      p.ID,
		"color":         p.Color,
		"username":      p.Username,
		"name":          p.Name,
		"protocol":      protocolVersion,
		"serverVersion": serverVersion,
		"blocks":        blocks,
		"players":       players,
	})
}

// ---------- WebSocket ----------
func (s *GameServer) handleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	s.mu.Lock()
	username, ok := s.tokens[token]
	running := s.running
	s.mu.Unlock()
	if !ok || username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("401 Unauthorized"))
		return
	}
	if !running {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("403 Server stopped"))
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	user := s.users[username]
	if user == nil {
		user = &User{Username: username, Name: username}
	}
	id := s.nextID
	s.nextID++
	color := colors[(id-1)%len(colors)]
	name := user.Name
	if name == "" {
		name = username
	}
	p := &Player{
		ID: id, Name: name, Username: username, Color: color,
		X: 16, Y: 12, Z: 16, LastMove: time.Now().UnixNano() / int64(time.Millisecond),
	}
	c := &client{conn: conn, player: p}
	s.clients[c] = true
	s.mu.Unlock()

	log.Printf("[+] %s (%s) ID:%d", p.Name, username, id)

	c.hello = time.AfterFunc(1500*time.Millisecond, func() {
		s.mu.Lock()
		if c.player.Handled {
			s.mu.Unlock()
			return
		}
		c.player.Handled = true
		s.mu.Unlock()
		s.sendPublicInit(c)
		s.broadcast(map[string]interface{}{
			"type": "player_join", "player": map[string]interface{}{
				"id": p.ID, "name": p.Name, "color": color, "username": username,
				"x": 16, "y": 12, "z": 16, "pitch": 0, "yaw": 0,
			},
		}, c)
	})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			s.onDisconnect(c)
			return
		}
		s.handleMessage(c, raw)
	}
}

func (s *GameServer) onDisconnect(c *client) {
	s.mu.Lock()
	if _, exists := s.clients[c]; !exists {
		s.mu.Unlock()
		return
	}
	p := c.player
	delete(s.clients, c)
	if p.IsHost {
		delete(s.hosts, p.Room)
		room := p.Room
		s.mu.Unlock()
		for _, rc := range s.roomClients(room) {
			rc.conn.WriteJSON(map[string]interface{}{"type": "host_gone"})
		}
		log.Printf("[-] %s 离开 (房主, 房间:%s)", p.Name, room)
		return
	}
	if p.RoomClient {
		room := p.Room
		cid := p.Cid
		s.mu.Unlock()
		if h, ok := s.hosts[room]; ok {
			h.conn.WriteJSON(map[string]interface{}{"type": "room_client_leave", "cid": cid})
		}
		log.Printf("[-] %s 离开 (房间:%s)", p.Name, room)
		return
	}
	id := p.ID
	s.mu.Unlock()
	s.broadcastAll(map[string]interface{}{"type": "player_leave", "playerId": id})
	log.Printf("[-] %s 离开", p.Name)
}

func (s *GameServer) handleMessage(c *client, raw []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	if data == nil {
		return
	}
	typ, _ := data["type"].(string)
	s.mu.Lock()
	p := c.player
	s.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[消息异常] %s: %v", p.Name, r)
		}
	}()

	switch typ {
	case "hello":
		s.mu.Lock()
		p.Pv = toInt(data["pv"])
		if gv, ok := data["gv"].(string); ok {
			p.Gv = gv
		}
		if p.Handled {
			s.mu.Unlock()
			return
		}
		p.Handled = true
		if c.hello != nil {
			c.hello.Stop()
		}
		room := ""
		if r, ok := data["room"].(string); ok {
			room = strings.TrimSpace(r)[:min(len(strings.TrimSpace(r)), 24)]
		}
		s.mu.Unlock()
		if room != "" {
			s.mu.Lock()
			h, ok := s.hosts[room]
			s.mu.Unlock()
			if ok && h.conn != nil {
				s.mu.Lock()
				p.RoomClient = true
				p.Room = room
				p.Cid = s.nextCid
				s.nextCid++
				s.mu.Unlock()
				h.conn.WriteJSON(map[string]interface{}{
					"type": "room_client_join", "cid": p.Cid, "name": p.Name, "username": p.Username,
				})
				return
			}
			c.conn.WriteJSON(map[string]interface{}{"type": "room_missing", "room": room})
		}
		s.sendPublicInit(c)
		s.broadcast(map[string]interface{}{
			"type": "player_join", "player": map[string]interface{}{
				"id": p.ID, "name": p.Name, "color": p.Color, "username": p.Username,
				"x": 16, "y": 12, "z": 16, "pitch": 0, "yaw": 0,
			},
		}, c)

	case "host":
		s.mu.Lock()
		room := ""
		if r, ok := data["room"].(string); ok {
			room = strings.TrimSpace(r)[:min(len(strings.TrimSpace(r)), 24)]
		}
		if room == "" {
			s.mu.Unlock()
			c.conn.WriteJSON(map[string]interface{}{"type": "host_err", "msg": "房间名无效"})
			return
		}
		p.Handled = true
		if c.hello != nil {
			c.hello.Stop()
		}
		p.IsHost = true
		p.Room = room
		s.hosts[room] = c
		s.mu.Unlock()
		c.conn.WriteJSON(map[string]interface{}{
			"type": "host_ok", "room": room, "serverVersion": serverVersion, "protocol": protocolVersion,
		})
		log.Printf("[房主] %s 开房: %s", p.Name, room)

	case "chat":
		msg, _ := data["msg"].(string)
		if msg == "" {
			return
		}
		msg = msg[:min(len(msg), 200)]
		if p.IsHost {
			s.routeHostToRoom(p.Room, map[string]interface{}{"type": "chat", "name": strOr(data["name"], "房主"), "msg": msg})
		} else if p.RoomClient {
			if h, ok := s.hosts[p.Room]; ok {
				h.conn.WriteJSON(mergeWith(data, map[string]interface{}{"_from": p.Cid}))
			}
		} else {
			s.broadcastAll(map[string]interface{}{"type": "chat", "playerId": p.ID, "name": p.Name, "msg": msg})
		}

	case "move":
		s.mu.Lock()
		if x, ok := data["x"].(float64); ok {
			p.X = x
		}
		if y, ok := data["y"].(float64); ok {
			p.Y = y
		}
		if z, ok := data["z"].(float64); ok {
			p.Z = z
		}
		if pi, ok := data["pitch"].(float64); ok {
			p.Pitch = pi
		}
		if ya, ok := data["yaw"].(float64); ok {
			p.Yaw = ya
		}
		p.LastMove = time.Now().UnixNano() / int64(time.Millisecond)
		s.mu.Unlock()
		if p.IsHost {
			s.routeHostToRoom(p.Room, data)
		} else if p.RoomClient {
			if h, ok := s.hosts[p.Room]; ok {
				h.conn.WriteJSON(mergeWith(data, map[string]interface{}{"_from": p.Cid}))
			}
		}

	case "set_name":
		if nm, ok := data["name"].(string); ok && nm != "" {
			nm = nm[:min(len(nm), 12)]
			s.mu.Lock()
			p.Name = nm
			if u, ok := s.users[p.Username]; ok {
				u.Name = nm
			}
			s.mu.Unlock()
			s.saveUsers()
			if p.IsHost {
				s.routeHostToRoom(p.Room, map[string]interface{}{"type": "player_name", "name": nm})
			} else if p.RoomClient {
				if h, ok := s.hosts[p.Room]; ok {
					h.conn.WriteJSON(mergeWith(data, map[string]interface{}{"_from": p.Cid}))
				}
			} else {
				s.broadcastAll(map[string]interface{}{"type": "player_name", "playerId": p.ID, "name": nm})
			}
		}

	case "block_break":
		k := blockKey(data)
		s.mu.Lock()
		delete(s.worldBlocks, k)
		s.mu.Unlock()
		if p.IsHost {
			s.routeHostToRoom(p.Room, data)
		} else if p.RoomClient {
			if h, ok := s.hosts[p.Room]; ok {
				h.conn.WriteJSON(mergeWith(data, map[string]interface{}{"_from": p.Cid}))
			}
		} else {
			s.broadcast(map[string]interface{}{"type": "block_break", "x": data["x"], "y": data["y"], "z": data["z"], "playerId": p.ID}, c)
		}

	case "block_place":
		k := blockKey(data)
		bid := toInt(data["blockId"])
		s.mu.Lock()
		s.worldBlocks[k] = bid
		s.mu.Unlock()
		if p.IsHost {
			s.routeHostToRoom(p.Room, data)
		} else if p.RoomClient {
			if h, ok := s.hosts[p.Room]; ok {
				h.conn.WriteJSON(mergeWith(data, map[string]interface{}{"_from": p.Cid}))
			}
		} else {
			s.broadcast(map[string]interface{}{"type": "block_place", "x": data["x"], "y": data["y"], "z": data["z"], "blockId": bid, "playerId": p.ID}, c)
		}

	case "init", "players_move", "player_join", "player_leave", "block_update":
		if p.IsHost {
			s.routeHostToRoom(p.Room, data)
		}

	default:
		// 未知消息类型（未来版本新增）：房主发来的也透传给房间（向前兼容），其余忽略、不崩溃
		if p.IsHost {
			s.routeHostToRoom(p.Room, data)
		}
	}
}

// ---------- 工具 ----------
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func strOr(v interface{}, def string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func blockKey(data map[string]interface{}) string {
	return strconv.Itoa(toInt(data["x"])) + "," + strconv.Itoa(toInt(data["y"])) + "," + strconv.Itoa(toInt(data["z"]))
}

func mergeWith(data map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range data {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
