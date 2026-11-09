package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Module is one classroom stage (jisuan / shuchu / shuru).
type Module interface {
	Name() string
	Routes(mux *http.ServeMux)
	Snapshot() ([]byte, error)
	Restore([]byte) error
	Reset()
	Check(content fs.FS) []error
}

// Config holds tunables from content/common/config.json (+ /data/config.override.json).
type Config struct {
	MaxConcurrency int            `json:"max_concurrency"`
	EffortStyle    string         `json:"effort_style"`
	Effort         map[string]string `json:"effort"`
	UseLLM         bool           `json:"use_llm"`
	MaxAttempts    int            `json:"max_attempts"`
	LowFX          bool           `json:"lowfx"`
	ShowDraft      bool           `json:"show_draft"`
	AnimScale      float64        `json:"anim_scale"`
	LanIP          string         `json:"lan_ip"`
}

type Seat struct {
	Seat  int `json:"seat"`
	Group int `json:"group"`
}

type App struct {
	Content  string // content dir
	Data     string
	Pin      string
	Mode     atomic.Value // string
	Hubs     map[string]*Hub
	Seats    []Seat
	seatMu   sync.Mutex
	tokens   map[string]int // token -> seat
	seatTok  map[int]string // seat -> token
	cfgMu    sync.RWMutex
	cfg      Config
	modules  []Module
	dirty    sync.Map // module name -> bool
	Started  time.Time
	Logger   *log.Logger
	OnConfig func(Config)
	lk       locks
}

func NewApp(content, data, pin, mode string) (*App, error) {
	a := &App{Content: content, Data: data, Pin: pin, Hubs: map[string]*Hub{}, tokens: map[string]int{}, seatTok: map[int]string{}, Started: time.Now()}
	if mode == "" {
		mode = "live"
	}
	a.Mode.Store(mode)
	defer a.initLocks()
	for _, d := range []string{"state", "logs", "replay", "uploads"} {
		os.MkdirAll(filepath.Join(data, d), 0o755)
	}
	lf, err := os.OpenFile(filepath.Join(data, "logs", "yxz-"+time.Now().Format("2006-01-02")+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		a.Logger = log.New(lf, "", log.LstdFlags)
	} else {
		a.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	if err := a.loadConfig(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(content, "common", "seats.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &a.Seats); err != nil {
		return nil, err
	}
	a.restoreSeats()
	return a, nil
}

func (a *App) loadConfig() error {
	var c Config
	b, err := os.ReadFile(filepath.Join(a.Content, "common", "config.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	if ob, err := os.ReadFile(filepath.Join(a.Data, "config.override.json")); err == nil {
		json.Unmarshal(ob, &c)
	}
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = 10
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.AnimScale <= 0 {
		c.AnimScale = 1
	}
	a.cfgMu.Lock()
	a.cfg = c
	a.cfgMu.Unlock()
	return nil
}

func (a *App) Cfg() Config { a.cfgMu.RLock(); defer a.cfgMu.RUnlock(); return a.cfg }

func (a *App) SetCfg(fn func(c *Config)) {
	a.cfgMu.Lock()
	fn(&a.cfg)
	c := a.cfg
	a.cfgMu.Unlock()
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(filepath.Join(a.Data, "config.override.json"), b, 0o644)
}

func (a *App) ModeName() string { return a.Mode.Load().(string) }

func (a *App) Hub(name string) *Hub {
	if h, ok := a.Hubs[name]; ok {
		return h
	}
	h := NewHub()
	a.Hubs[name] = h
	return h
}

func (a *App) Register(m Module) {
	a.modules = append(a.modules, m)
	a.Hub(m.Name())
	if b, err := os.ReadFile(filepath.Join(a.Data, "state", m.Name()+".json")); err == nil {
		if err := m.Restore(b); err != nil {
			a.Logger.Printf("restore %s: %v", m.Name(), err)
		}
	}
}

func (a *App) Modules() []Module { return a.modules }

// MarkDirty asks the snapshot loop to persist a module soon (also call on key actions).
func (a *App) MarkDirty(name string) { a.dirty.Store(name, true) }

func (a *App) SaveNow(name string) {
	for _, m := range a.modules {
		if m.Name() == name {
			if b, err := m.Snapshot(); err == nil {
				tmp := filepath.Join(a.Data, "state", name+".json.tmp")
				if os.WriteFile(tmp, b, 0o644) == nil {
					os.Rename(tmp, filepath.Join(a.Data, "state", name+".json"))
				}
			}
		}
	}
}

func (a *App) SnapshotLoop() {
	t := time.NewTicker(5 * time.Second)
	for range t.C {
		a.dirty.Range(func(k, v any) bool {
			a.dirty.Delete(k)
			a.SaveNow(k.(string))
			return true
		})
		a.saveSeats()
	}
}

// ---------------- seats ----------------

type seatFile struct {
	Tokens map[string]int `json:"tokens"`
}

func (a *App) saveSeats() {
	a.seatMu.Lock()
	b, _ := json.Marshal(seatFile{Tokens: a.tokens})
	a.seatMu.Unlock()
	os.WriteFile(filepath.Join(a.Data, "state", "seats.json"), b, 0o644)
}

func (a *App) restoreSeats() {
	b, err := os.ReadFile(filepath.Join(a.Data, "state", "seats.json"))
	if err != nil {
		return
	}
	var f seatFile
	if json.Unmarshal(b, &f) == nil && f.Tokens != nil {
		a.tokens = f.Tokens
		for t, s := range f.Tokens {
			a.seatTok[s] = t
		}
	}
}

func (a *App) GroupOf(seat int) int {
	for _, s := range a.Seats {
		if s.Seat == seat {
			return s.Group
		}
	}
	return 0
}

// SeatOf returns the seat for the request's token (0 when none).
func (a *App) SeatOf(r *http.Request) int {
	tok := r.Header.Get("X-Seat-Token")
	if tok == "" {
		if c, err := r.Cookie("yxz_seat"); err == nil {
			tok = c.Value
		}
	}
	if tok == "" {
		return 0
	}
	a.seatMu.Lock()
	defer a.seatMu.Unlock()
	return a.tokens[tok]
}

func randTok() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *App) ClearSeats() {
	a.seatMu.Lock()
	a.tokens = map[string]int{}
	a.seatTok = map[int]string{}
	a.seatMu.Unlock()
	a.saveSeats()
	for _, h := range a.Hubs {
		h.Publish("student:all", "seats_cleared", map[string]any{})
	}
}

// ---------------- teacher pin ----------------

func (a *App) pinHash() string {
	h := sha256.Sum256([]byte("yxz|" + a.Pin))
	return hex.EncodeToString(h[:16])
}

func (a *App) IsTeacher(r *http.Request) bool {
	if a.Pin == "" {
		return true
	}
	c, err := r.Cookie("yxz_pin")
	return err == nil && c.Value == a.pinHash()
}

// Teacher wraps a handler with the teacher-pin check.
func (a *App) Teacher(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.IsTeacher(r) {
			Err(w, 401, "需要教师口令", "need_pin")
			return
		}
		h(w, r)
	}
}

// ---------------- helpers ----------------

func JSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func Err(w http.ResponseWriter, status int, msg, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

func Body(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 2<<20)).Decode(v)
}

func LanIPs() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, ad := range addrs {
			if ipn, ok := ad.(*net.IPNet); ok && ipn.IP.To4() != nil {
				ip := ipn.IP.String()
				if strings.HasPrefix(ip, "169.254.") {
					continue
				}
				out = append(out, ip)
			}
		}
	}
	return out
}

// ---------------- common routes ----------------

func (a *App) CommonRoutes(mux *http.ServeMux, llmStatus func() any) {
	a.lockRoutes(mux)
	mux.HandleFunc("GET /api/common/health", func(w http.ResponseWriter, r *http.Request) { JSON(w, map[string]any{"ok": true}) })
	mux.HandleFunc("GET /api/common/lan", func(w http.ResponseWriter, r *http.Request) {
		ip := a.Cfg().LanIP
		if ip == "" {
			if ips := LanIPs(); len(ips) > 0 {
				ip = ips[0]
			}
		}
		JSON(w, map[string]any{"ip": ip, "ips": LanIPs()})
	})
	mux.HandleFunc("GET /api/common/me", func(w http.ResponseWriter, r *http.Request) {
		seat := a.SeatOf(r)
		c := a.Cfg()
		JSON(w, map[string]any{"seat": seat, "group": a.GroupOf(seat), "teacher": a.IsTeacher(r), "lowfx": c.LowFX, "anim_scale": c.AnimScale})
	})
	mux.HandleFunc("GET /api/common/seats", func(w http.ResponseWriter, r *http.Request) {
		a.seatMu.Lock()
		taken := make([]int, 0, len(a.seatTok))
		for s := range a.seatTok {
			taken = append(taken, s)
		}
		a.seatMu.Unlock()
		JSON(w, map[string]any{"seats": a.Seats, "taken": taken})
	})
	mux.HandleFunc("POST /api/common/join", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Seat  int  `json:"seat"`
			Force bool `json:"force"`
		}
		if Body(r, &in) != nil || a.GroupOf(in.Seat) == 0 {
			Err(w, 400, "没有这个座位号", "bad_seat")
			return
		}
		cur := a.SeatOf(r)
		a.seatMu.Lock()
		old, taken := a.seatTok[in.Seat]
		mine := false
		if c, err := r.Cookie("yxz_seat"); err == nil && c.Value == old {
			mine = true
		}
		if t := r.Header.Get("X-Seat-Token"); t != "" && t == old {
			mine = true
		}
		if taken && !mine && !in.Force {
			a.seatMu.Unlock()
			Err(w, 409, "这个座位已经有平板了，要换到这台吗？", "seat_taken")
			return
		}
		if cur != 0 && cur != in.Seat { // leaving another seat
			if t := a.seatTok[cur]; t != "" {
				delete(a.tokens, t)
				delete(a.seatTok, cur)
			}
		}
		tok := old
		if !mine {
			if taken {
				delete(a.tokens, old)
			}
			tok = randTok()
		}
		a.tokens[tok] = in.Seat
		a.seatTok[in.Seat] = tok
		a.seatMu.Unlock()
		if taken && !mine {
			for _, h := range a.Hubs {
				h.Publish("student:"+strconv.Itoa(in.Seat), "seat_moved", map[string]string{"keep": tok})
			}
		}
		http.SetCookie(w, &http.Cookie{Name: "yxz_seat", Value: tok, Path: "/", MaxAge: 86400, SameSite: http.SameSiteLaxMode})
		go a.saveSeats()
		JSON(w, map[string]any{"seat": in.Seat, "group": a.GroupOf(in.Seat), "token": tok})
	})
	mux.HandleFunc("POST /api/common/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Pin string `json:"pin"`
		}
		Body(r, &in)
		if a.Pin != "" && in.Pin != a.Pin {
			Err(w, 403, "口令不对", "bad_pin")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "yxz_pin", Value: a.pinHash(), Path: "/", MaxAge: 12 * 3600, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		JSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/common/admin", a.Teacher(func(w http.ResponseWriter, r *http.Request) {
		hubs := map[string]any{}
		for n, h := range a.Hubs {
			sc, st := h.Count()
			hubs[n] = map[string]int{"screens": sc, "students": st}
		}
		a.seatMu.Lock()
		nseats := len(a.seatTok)
		a.seatMu.Unlock()
		JSON(w, map[string]any{"mode": a.ModeName(), "config": a.Cfg(), "llm": llmStatus(), "hubs": hubs, "seats": nseats, "ips": LanIPs(), "uptime": time.Since(a.Started).Round(time.Second).String()})
	}))
	mux.HandleFunc("POST /api/common/admin", a.Teacher(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action string          `json:"action"`
			Mode   string          `json:"mode"`
			Module string          `json:"module"`
			Config json.RawMessage `json:"config"`
		}
		if Body(r, &in) != nil {
			Err(w, 400, "格式不对", "bad_body")
			return
		}
		switch in.Action {
		case "mode":
			if in.Mode == "live" || in.Mode == "replay" || in.Mode == "mock" {
				a.Mode.Store(in.Mode)
				for _, h := range a.Hubs {
					h.Publish("screen", "mode", map[string]string{"mode": in.Mode})
				}
			}
		case "config":
			a.SetCfg(func(c *Config) { json.Unmarshal(in.Config, c) })
			c := a.Cfg()
			if a.OnConfig != nil {
				a.OnConfig(c)
			}
			for _, h := range a.Hubs {
				h.Publish("all", "config", map[string]any{"lowfx": c.LowFX, "anim_scale": c.AnimScale, "show_draft": c.ShowDraft})
			}
		case "clear_seats":
			a.ClearSeats()
		case "new_class": // 换下一个班：清空所有环节的课堂数据、座位和学生图片；保留回放录音和设置
			for _, m := range a.modules {
				m.Reset()
				a.SaveNow(m.Name())
			}
			a.ClearSeats()
			up := filepath.Join(a.Data, "uploads")
			os.RemoveAll(up)
			os.MkdirAll(filepath.Join(up, "shuchu"), 0o755)
			a.SetLock("", true)
			a.Logger.Printf("new_class: 已清空课堂数据，四个环节重新上锁")
		case "reset":
			for _, m := range a.modules {
				if in.Module == "" || in.Module == m.Name() {
					m.Reset()
					a.SaveNow(m.Name())
				}
			}
		}
		JSON(w, map[string]any{"ok": true, "mode": a.ModeName(), "config": a.Cfg()})
	}))
	mux.HandleFunc("GET /events/{mod}", func(w http.ResponseWriter, r *http.Request) {
		h, ok := a.Hubs[r.PathValue("mod")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		role := r.URL.Query().Get("role")
		if role == "screen" {
			if !a.IsTeacher(r) {
				Err(w, 401, "需要教师口令", "need_pin")
				return
			}
			h.Serve(w, r, "screen", 0, 0)
			return
		}
		seat := a.SeatOf(r)
		if seat == 0 {
			if t := r.URL.Query().Get("t"); t != "" {
				a.seatMu.Lock()
				seat = a.tokens[t]
				a.seatMu.Unlock()
			}
		}
		h.Serve(w, r, "student", seat, a.GroupOf(seat))
	})
}

// ---------------- static ----------------

// Static serves the built frontend. Pages: no-cache; /assets/*: immutable.
func (a *App) Static(dist fs.FS) http.Handler {
	files := http.FileServer(http.FS(dist))
	pages := map[string]string{
		"/": "index.html", "/admin": "admin.html",
		"/jisuan/": "jisuan/index.html", "/jisuan/screen": "jisuan/screen.html",
		"/shuchu/": "shuchu/index.html", "/shuchu/screen": "shuchu/screen.html",
		"/shuru/": "shuru/index.html", "/shuru/screen": "shuru/screen.html",
		"/changxiang/": "changxiang/index.html", "/changxiang/screen": "changxiang/screen.html",
	}
	teacherPages := map[string]bool{"/admin": true, "/jisuan/screen": true, "/shuchu/screen": true, "/shuru/screen": true, "/changxiang/screen": true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/jisuan" || p == "/shuchu" || p == "/shuru" || p == "/changxiang" {
			http.Redirect(w, r, p+"/", http.StatusFound)
			return
		}
		if f, ok := pages[p]; ok {
			if teacherPages[p] && !a.IsTeacher(r) {
				f = "login.html"
			}
			b, err := fs.ReadFile(dist, f)
			if err != nil {
				http.Error(w, "前端还没有构建", 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(b)
			return
		}
		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		files.ServeHTTP(w, r)
	})
}

// Gzip-free: assets are small and LAN-local; the classroom gateway is the bottleneck, not bytes.
