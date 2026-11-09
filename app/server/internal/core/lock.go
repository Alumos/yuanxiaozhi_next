package core

// 环节锁：每个环节默认锁住，教师在该环节的大屏上解锁后，学生平板才能操作。
// 锁住时学生的 GET 照常（页面能打开、能选座位），写操作返回 423。

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var Lockable = []string{"shuru", "jisuan", "shuchu", "changxiang"}

type locks struct {
	mu sync.RWMutex
	on map[string]bool // true = 锁住
}

func (a *App) lockFile() string { return filepath.Join(a.Data, "state", "locks.json") }

func (a *App) initLocks() {
	a.lk.on = map[string]bool{}
	for _, m := range Lockable {
		a.lk.on[m] = true
	}
	if b, err := os.ReadFile(a.lockFile()); err == nil {
		var v map[string]bool
		if json.Unmarshal(b, &v) == nil {
			for _, m := range Lockable {
				if x, ok := v[m]; ok {
					a.lk.on[m] = x
				}
			}
		}
	}
}

func (a *App) Locks() map[string]bool {
	a.lk.mu.RLock()
	defer a.lk.mu.RUnlock()
	out := make(map[string]bool, len(a.lk.on))
	for k, v := range a.lk.on {
		out[k] = v
	}
	return out
}

func (a *App) Locked(mod string) bool {
	a.lk.mu.RLock()
	defer a.lk.mu.RUnlock()
	return a.lk.on[mod]
}

// SetLock changes one stage's lock (mod "" = all stages) and tells every tablet and screen.
func (a *App) SetLock(mod string, on bool) {
	a.lk.mu.Lock()
	for _, m := range Lockable {
		if mod == "" || mod == m {
			a.lk.on[m] = on
		}
	}
	b, _ := json.Marshal(a.lk.on)
	a.lk.mu.Unlock()
	os.WriteFile(a.lockFile(), b, 0o644)
	for _, m := range Lockable {
		if mod == "" || mod == m {
			a.Hub(m).Publish("all", "lock", map[string]any{"mod": m, "locked": on})
		}
	}
}

// lockExempt: requests that only finish or re-attach to something already running, or are teacher-driven.
func lockExempt(p string) bool {
	return strings.Contains(p, "/screen/") || p == "/api/shuchu/vote" || p == "/api/shuchu/group/image" || p == "/api/jisuan/analyze"
}

// LockGuard blocks students' write requests to a locked stage.
func (a *App) LockGuard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/") {
			mod, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
			if mod != "common" && a.Locked(mod) && !lockExempt(r.URL.Path) && !a.IsTeacher(r) {
				Err(w, 423, "老师还没开放这个环节，先听老师讲", "locked")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

func (a *App) lockRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/common/locks", func(w http.ResponseWriter, r *http.Request) { JSON(w, a.Locks()) })
	mux.HandleFunc("POST /api/common/lock", a.Teacher(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Mod    string `json:"mod"`
			Locked bool   `json:"locked"`
		}
		if Body(r, &in) != nil {
			Err(w, 400, "格式不对", "bad_body")
			return
		}
		a.SetLock(in.Mod, in.Locked)
		a.Logger.Printf("lock %q -> %v", in.Mod, in.Locked)
		JSON(w, a.Locks())
	}))
}
