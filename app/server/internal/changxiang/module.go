// Package changxiang is the 畅想 stage: students design a new 元小智 feature
// (name + 输入 / 计算 / 输出) and the classroom screen shows them as rolling scrolls.
package changxiang

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"yxz/internal/core"
)

const maxPerSeat = 5

type Idea struct {
	ID      string    `json:"id"`
	Seat    int       `json:"seat"`
	Group   int       `json:"group"`
	Name    string    `json:"name"`
	Input   string    `json:"input"`
	Compute string    `json:"compute"`
	Output  string    `json:"output"`
	At      time.Time `json:"at"`
	Hidden  bool      `json:"hidden,omitempty"` // 教师隐藏（不上大屏）
}

type Module struct {
	app       *core.App
	hub       *core.Hub
	sensitive []string

	mu    sync.RWMutex
	ideas []*Idea
}

func New(app *core.App) *Module {
	m := &Module{app: app, hub: app.Hub("changxiang")}
	b, _ := os.ReadFile(filepath.Join(app.Content, "common", "sensitive_words.txt"))
	for _, w := range strings.Split(string(b), "\n") {
		if w = strings.TrimSpace(w); w != "" && !strings.HasPrefix(w, "#") {
			m.sensitive = append(m.sensitive, w)
		}
	}
	return m
}

func (m *Module) Name() string              { return "changxiang" }
func (m *Module) Check(fs.FS) []error       { return nil }
func (m *Module) Snapshot() ([]byte, error) { m.mu.RLock(); defer m.mu.RUnlock(); return json.Marshal(m.ideas) }
func (m *Module) Restore(b []byte) error    { return json.Unmarshal(b, &m.ideas) }
func (m *Module) Reset() {
	m.mu.Lock()
	m.ideas = nil
	m.mu.Unlock()
	m.hub.Publish("all", "reset", map[string]any{})
}

var rePhone = regexp.MustCompile(`1[3-9]\d{9}|\d{7,}|\d{3,4}-\d{7,8}`)
var reAddr = regexp.MustCompile(`(路|街|巷|弄|小区|号楼|单元|栋|幢)\s*\d|\d+\s*(号|室|栋|幢|单元)|身份证|住在|我家在|电话`)

func (m *Module) checkText(s string) (string, string) {
	if rePhone.MatchString(s) || reAddr.MatchString(s) {
		return "不用告诉元小智电话、住址这些哦", "personal_info"
	}
	for _, w := range m.sensitive {
		if strings.Contains(s, w) {
			return "这句话换个说法吧", "sensitive"
		}
	}
	return "", ""
}

func clean(s string, n int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

func newID() string { b := make([]byte, 5); rand.Read(b); return hex.EncodeToString(b) }

func (m *Module) mine(seat int) []*Idea {
	var out []*Idea
	for _, i := range m.ideas {
		if i.Seat == seat {
			out = append(out, i)
		}
	}
	return out
}

func (m *Module) visible() []*Idea {
	out := make([]*Idea, 0, len(m.ideas))
	for _, i := range m.ideas {
		if !i.Hidden {
			out = append(out, i)
		}
	}
	return out
}

func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/changxiang/mine", func(w http.ResponseWriter, r *http.Request) {
		seat := m.app.SeatOf(r)
		if seat == 0 {
			core.Err(w, 401, "先选一下座位号吧", "no_seat")
			return
		}
		m.mu.RLock()
		defer m.mu.RUnlock()
		core.JSON(w, map[string]any{"ideas": m.mine(seat), "max": maxPerSeat})
	})
	mux.HandleFunc("POST /api/changxiang/submit", func(w http.ResponseWriter, r *http.Request) {
		seat := m.app.SeatOf(r)
		if seat == 0 {
			core.Err(w, 401, "先选一下座位号吧", "no_seat")
			return
		}
		var in struct{ Name, Input, Compute, Output string }
		if core.Body(r, &in) != nil {
			core.Err(w, 400, "格式不对", "bad_body")
			return
		}
		it := &Idea{ID: newID(), Seat: seat, Group: m.app.GroupOf(seat), Name: clean(in.Name, 16), Input: clean(in.Input, 120), Compute: clean(in.Compute, 120), Output: clean(in.Output, 120), At: time.Now()}
		switch {
		case utf8.RuneCountInString(it.Name) < 2:
			core.Err(w, 400, "给新功能起个名字吧（至少 2 个字）", "name")
			return
		case utf8.RuneCountInString(it.Input) < 2:
			core.Err(w, 400, "“输入”还没写：元小智要收到什么？", "input")
			return
		case utf8.RuneCountInString(it.Compute) < 2:
			core.Err(w, 400, "“计算”还没写：元小智要怎么想、怎么算？", "compute")
			return
		case utf8.RuneCountInString(it.Output) < 2:
			core.Err(w, 400, "“输出”还没写：元小智最后给出什么？", "output")
			return
		}
		if msg, code := m.checkText(it.Name + " " + it.Input + " " + it.Compute + " " + it.Output); msg != "" {
			core.Err(w, 400, msg, code)
			return
		}
		m.mu.Lock()
		if len(m.mine(seat)) >= maxPerSeat {
			m.mu.Unlock()
			core.Err(w, 409, "已经交了 5 个畅想啦，去看看大屏吧", "max")
			return
		}
		m.ideas = append(m.ideas, it)
		n := len(m.mine(seat))
		m.mu.Unlock()
		m.hub.Publish("screen", "idea", it)
		m.app.SaveNow("changxiang")
		core.JSON(w, map[string]any{"idea": it, "count": n, "max": maxPerSeat})
	})
	mux.HandleFunc("GET /api/changxiang/screen/state", m.app.Teacher(func(w http.ResponseWriter, r *http.Request) {
		m.mu.RLock()
		all := append([]*Idea(nil), m.ideas...)
		m.mu.RUnlock()
		sort.SliceStable(all, func(i, j int) bool { return all[i].At.Before(all[j].At) })
		seats := map[int]bool{}
		for _, i := range all {
			seats[i.Seat] = true
		}
		core.JSON(w, map[string]any{"ideas": all, "seats": len(seats), "mode": m.app.ModeName()})
	}))
	mux.HandleFunc("POST /api/changxiang/screen/control", m.app.Teacher(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action string `json:"action"`
			ID     string `json:"id"`
		}
		if core.Body(r, &in) != nil {
			core.Err(w, 400, "格式不对", "bad_body")
			return
		}
		switch in.Action {
		case "hide", "show", "delete":
			m.mu.Lock()
			for k, i := range m.ideas {
				if i.ID == in.ID {
					if in.Action == "delete" {
						m.ideas = append(m.ideas[:k], m.ideas[k+1:]...)
					} else {
						i.Hidden = in.Action == "hide"
					}
					break
				}
			}
			m.mu.Unlock()
			m.hub.Publish("screen", "ideas_changed", map[string]any{})
		case "look":
			m.hub.Publish("student:all", "look", map[string]any{"on": true})
		case "unlook":
			m.hub.Publish("student:all", "look", map[string]any{"on": false})
		case "reset":
			m.Reset()
		}
		m.app.SaveNow("changxiang")
		core.JSON(w, map[string]any{"ok": true})
	}))
}

func (m *Module) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]any{"ideas": len(m.ideas), "visible": len(m.visible())}
}
