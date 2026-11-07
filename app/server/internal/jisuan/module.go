package jisuan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"yxz/internal/core"
	"yxz/internal/llm"
)

type Module struct {
	app        *core.App
	L          *Library
	gw         *llm.Gateway
	rp         *llm.Replay
	hub        *core.Hub
	prefix     string
	prefixHash string
	sensitive  []string

	mu     sync.RWMutex
	sess   map[int]*Session
	paused bool
	warm   string
}

func New(app *core.App, gw *llm.Gateway, rp *llm.Replay) (*Module, error) {
	dir := filepath.Join(app.Content, "jisuan")
	L, err := LoadLibrary(dir)
	if err != nil {
		return nil, err
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(app.Content, p)); return strings.TrimSpace(string(b)) }
	prefix := read("common/persona.md") + "\n\n" + read("jisuan/prompts/recommend_rules.md") +
		"\n\n【示例】（示例只演示格式和写法；真实回答要更具体，引用更多细节）\n" + read("jisuan/prompts/example.md")
	h := sha256.Sum256([]byte(prefix))
	m := &Module{app: app, L: L, gw: gw, rp: rp, hub: app.Hub("jisuan"), prefix: prefix, prefixHash: hex.EncodeToString(h[:6]), sess: map[int]*Session{}, warm: "未预热"}
	for _, w := range strings.Split(read("common/sensitive_words.txt"), "\n") {
		if w = strings.TrimSpace(w); w != "" && !strings.HasPrefix(w, "#") {
			m.sensitive = append(m.sensitive, w)
		}
	}
	app.Logger.Printf("jisuan: %d books, prefix hash %s (%d chars)", len(L.Books), m.prefixHash, len([]rune(prefix)))
	return m, nil
}

func (m *Module) Name() string { return "jisuan" }

func (m *Module) Snapshot() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(map[string]any{"sessions": m.sess, "paused": m.paused})
}

func (m *Module) Restore(b []byte) error {
	var v struct {
		Sessions map[int]*Session `json:"sessions"`
		Paused   bool             `json:"paused"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Sessions != nil {
		m.sess = v.Sessions
	}
	for _, s := range m.sess { // an analysis that was cut by a restart is redone on request
		if s.Stage == "analyzing" && s.Result == nil {
			s.Stage = "analyzing"
		}
		if s.Stage == "again" && s.Again == nil {
			s.Stage = "result"
		}
	}
	m.paused = v.Paused
	return nil
}

func (m *Module) Reset() {
	m.mu.Lock()
	m.sess = map[int]*Session{}
	m.paused = false
	m.mu.Unlock()
	m.hub.Publish("all", "reset", map[string]any{})
}

func (m *Module) Check(content fs.FS) []error {
	var errs []error
	for seed := uint64(1); seed <= 40; seed++ {
		var rounds []Round
		seen := map[string]bool{}
		for n := 1; n <= 5; n++ {
			sl := m.L.GenRound(n, rounds, seed, nil)
			if len(sl) != 5 {
				errs = append(errs, fmt.Errorf("出题：种子 %d 第 %d 轮只有 %d 本", seed, n, len(sl)))
			}
			for _, s := range sl {
				if seen[s.BookID] {
					errs = append(errs, fmt.Errorf("出题：种子 %d 重复的书 %s", seed, s.BookID))
				}
				seen[s.BookID] = true
			}
			rounds = append(rounds, Round{N: n, Shown: sl, Chosen: sl[int(seed)%len(sl)].BookID, Ms: 5000})
		}
	}
	return errs
}

// ---------------- helpers ----------------

func (m *Module) get(seat int) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sess[seat]
	if s == nil {
		s = &Session{Seat: seat, Stage: "intro", Attempt: 1}
		m.sess[seat] = s
	}
	return s
}

func (m *Module) seat(w http.ResponseWriter, r *http.Request) int {
	s := m.app.SeatOf(r)
	if s == 0 {
		core.Err(w, 401, "先选一下座位号吧", "no_seat")
	}
	return s
}

func (m *Module) seed(s *Session) uint64 { return uint64(s.Seat)*7919 + uint64(s.Attempt)*104729 }

type roundView struct {
	N     int         `json:"n"`
	Books []*BookView `json:"books"`
}

func (m *Module) stateView(s *Session) map[string]any {
	chosen := []*BookView{}
	for _, r := range s.Rounds {
		if r.Chosen != "" {
			chosen = append(chosen, m.L.View(r.Chosen))
		}
	}
	hist := []map[string]any{}
	all := append(append([]AttemptRecord{}, s.History...), s.current())
	for _, h := range all {
		f := h.Result
		if h.Again != nil {
			f = h.Again
		}
		e := map[string]any{"attempt": h.Attempt, "stars": h.Stars}
		if f != nil {
			e["result"] = f
			e["first"] = h.Result
			ids := []string{}
			for _, p := range f.Picks {
				ids = append(ids, p.BookID)
			}
			if h.Result != nil {
				for _, p := range h.Result.Picks {
					ids = append(ids, p.BookID)
				}
			}
			e["books"] = m.L.Views(ids)
			e["has_again"] = h.Again != nil
		}
		hist = append(hist, e)
	}
	return map[string]any{
		"seat": s.Seat, "stage": s.Stage, "attempt": s.Attempt, "max_attempts": m.app.Cfg().MaxAttempts,
		"picked": s.picked(), "chosen": chosen, "attempts": hist, "stars": s.Stars, "paused": m.paused,
		"again_used": s.Again != nil || s.AgainTrace != nil,
	}
}

func (m *Module) seatView(s *Session) map[string]any {
	v := map[string]any{"seat": s.Seat, "group": m.app.GroupOf(s.Seat), "stage": s.Stage, "picked": s.picked(), "attempt": s.Attempt, "stars": s.Stars}
	if f := s.final(); f != nil && len(f.Picks) > 0 {
		v["persona"] = f.Persona
		v["top"] = m.L.View(f.Picks[0].BookID)
		v["fallback"] = f.Fallback
	}
	// 两次结果不一样：给教师挑投屏
	if len(s.History) > 0 {
		personas := map[string]bool{}
		for _, h := range s.History {
			if h.Result != nil {
				personas[h.Result.Persona] = true
			}
		}
		if f := s.final(); f != nil {
			personas[f.Persona] = true
		}
		v["differ"] = len(personas) > 1
	}
	return v
}

func (m *Module) progress(s *Session) {
	m.mu.RLock()
	v := m.seatView(s)
	m.mu.RUnlock()
	m.hub.Publish("screen", "seat_progress", v)
	m.app.MarkDirty("jisuan")
}

// ---------------- routes ----------------

func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/jisuan/meta", func(w http.ResponseWriter, r *http.Request) {
		core.JSON(w, map[string]any{"categories": m.L.Tags.Categories, "topics": m.L.Tags.Topics, "tones": m.L.Tags.Tones, "books": len(m.L.Books), "tags": m.L.NTags, "spines": m.spines()})
	})
	mux.HandleFunc("GET /api/jisuan/state", func(w http.ResponseWriter, r *http.Request) {
		seat := m.seat(w, r)
		if seat == 0 {
			return
		}
		s := m.get(seat)
		m.mu.RLock()
		v := m.stateView(s)
		m.mu.RUnlock()
		core.JSON(w, v)
	})
	mux.HandleFunc("POST /api/jisuan/start", func(w http.ResponseWriter, r *http.Request) {
		seat := m.seat(w, r)
		if seat == 0 {
			return
		}
		s := m.get(seat)
		m.mu.Lock()
		if s.Stage == "intro" {
			s.Stage = "picking"
		}
		v := m.stateView(s)
		m.mu.Unlock()
		m.progress(s)
		core.JSON(w, v)
	})
	mux.HandleFunc("GET /api/jisuan/round", func(w http.ResponseWriter, r *http.Request) {
		seat := m.seat(w, r)
		if seat == 0 {
			return
		}
		s := m.get(seat)
		m.mu.Lock()
		defer m.mu.Unlock()
		if s.Stage != "picking" {
			core.Err(w, 409, "现在不是选书的时候", "stage")
			return
		}
		n := len(s.Rounds)
		if n == 0 || s.Rounds[n-1].Chosen != "" {
			if n >= 5 {
				core.Err(w, 409, "5 轮已经选完啦", "done")
				return
			}
			done := append([]Round(nil), s.Rounds...)
			slots := m.L.GenRound(n+1, done, m.seed(s), s.avoidSet())
			s.Rounds = append(s.Rounds, Round{N: n + 1, Shown: slots})
			s.ShownAt = time.Now()
			m.app.MarkDirty("jisuan")
		}
		cur := s.Rounds[len(s.Rounds)-1]
		rv := roundView{N: cur.N}
		for _, sl := range cur.Shown {
			rv.Books = append(rv.Books, m.L.View(sl.BookID))
		}
		core.JSON(w, rv)
	})
	mux.HandleFunc("POST /api/jisuan/pick", m.pick)
	mux.HandleFunc("POST /api/jisuan/analyze", m.analyze)
	mux.HandleFunc("POST /api/jisuan/again", m.again)
	mux.HandleFunc("POST /api/jisuan/stars", func(w http.ResponseWriter, r *http.Request) {
		seat := m.seat(w, r)
		if seat == 0 {
			return
		}
		var in struct {
			Stars int `json:"stars"`
		}
		core.Body(r, &in)
		s := m.get(seat)
		m.mu.Lock()
		s.Stars = max(0, min(5, in.Stars))
		m.mu.Unlock()
		m.hub.Publish("screen", "stars", map[string]int{"seat": seat, "stars": s.Stars})
		m.progress(s)
		core.JSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/jisuan/retry", func(w http.ResponseWriter, r *http.Request) {
		seat := m.seat(w, r)
		if seat == 0 {
			return
		}
		s := m.get(seat)
		m.mu.Lock()
		if s.Attempt >= m.app.Cfg().MaxAttempts {
			m.mu.Unlock()
			core.Err(w, 409, fmt.Sprintf("已经试了 %d 次啦", s.Attempt), "max_attempts")
			return
		}
		if s.Result == nil || s.Stage == "analyzing" || s.Stage == "again" {
			m.mu.Unlock()
			core.Err(w, 409, "等这次分析完再试吧", "busy")
			return
		}
		for id := range s.shownIDs() {
			s.Avoid = append(s.Avoid, id)
		}
		if f := s.final(); f != nil {
			for _, p := range f.Picks {
				s.Avoid = append(s.Avoid, p.BookID)
			}
		}
		s.History = append(s.History, s.current())
		s.Attempt++
		s.Rounds, s.Trace, s.Result, s.Again, s.AgainTrace, s.Stars, s.Prompt, s.Raw = nil, nil, nil, nil, nil, 0, "", ""
		s.job, s.againJob = nil, nil
		s.Stage = "picking"
		v := m.stateView(s)
		m.mu.Unlock()
		m.progress(s)
		m.app.SaveNow("jisuan")
		core.JSON(w, v)
	})
	mux.HandleFunc("POST /api/jisuan/finish", func(w http.ResponseWriter, r *http.Request) {
		seat := m.seat(w, r)
		if seat == 0 {
			return
		}
		s := m.get(seat)
		m.mu.Lock()
		if s.Result != nil {
			s.Stage = "done"
		}
		m.mu.Unlock()
		m.progress(s)
		core.JSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/jisuan/trace", func(w http.ResponseWriter, r *http.Request) {
		seat := m.seat(w, r)
		if seat == 0 {
			return
		}
		m.traceOut(w, r, seat, false)
	})
	mux.HandleFunc("GET /api/jisuan/books", func(w http.ResponseWriter, r *http.Request) {
		core.JSON(w, m.L.Views(strings.Split(r.URL.Query().Get("ids"), ",")))
	})
	mux.HandleFunc("GET /api/jisuan/screen/state", m.app.Teacher(m.screenState))
	mux.HandleFunc("GET /api/jisuan/screen/trace", m.app.Teacher(func(w http.ResponseWriter, r *http.Request) {
		seat, _ := strconv.Atoi(r.URL.Query().Get("seat"))
		m.traceOut(w, r, seat, true)
	}))
	mux.HandleFunc("POST /api/jisuan/screen/control", m.app.Teacher(m.control))
}

// spines: per-book category index in library order (for the book wall / tablet spines).
func (m *Module) spines() []int {
	out := make([]int, len(m.L.Books))
	for i, b := range m.L.Books {
		out[i] = b.cat
	}
	return out
}

func (m *Module) pick(w http.ResponseWriter, r *http.Request) {
	seat := m.seat(w, r)
	if seat == 0 {
		return
	}
	var in struct {
		Round  int      `json:"round"`
		BookID string   `json:"book_id"`
		Ms     int      `json:"ms"`
		Opened []string `json:"opened"`
	}
	if core.Body(r, &in) != nil {
		core.Err(w, 400, "格式不对", "bad_body")
		return
	}
	s := m.get(seat)
	m.mu.Lock()
	if m.paused {
		m.mu.Unlock()
		core.Err(w, 423, "请看大屏", "paused")
		return
	}
	n := len(s.Rounds)
	if s.Stage != "picking" || n == 0 || s.Rounds[n-1].N != in.Round {
		m.mu.Unlock()
		core.Err(w, 409, "这一轮已经选过了", "stage")
		return
	}
	cur := &s.Rounds[n-1]
	if cur.Chosen != "" {
		m.mu.Unlock()
		core.JSON(w, map[string]any{"next": map[bool]string{true: "analyze", false: "round"}[n >= 5]})
		return
	}
	ok := false
	shown := map[string]bool{}
	for _, sl := range cur.Shown {
		shown[sl.BookID] = true
		if sl.BookID == in.BookID {
			ok = true
		}
	}
	if !ok {
		m.mu.Unlock()
		core.Err(w, 400, "这本书不在这一轮里", "bad_book")
		return
	}
	ms := in.Ms
	if srv := int(time.Since(s.ShownAt).Milliseconds()); ms <= 0 || ms > srv+2000 {
		ms = srv
	}
	cur.Chosen, cur.Ms = in.BookID, max(300, min(ms, 120000))
	cur.Opened = nil
	for _, o := range in.Opened {
		if shown[o] && len(cur.Opened) < 5 {
			cur.Opened = append(cur.Opened, o)
		}
	}
	next := "round"
	if n >= 5 {
		next = "analyze"
		s.Stage = "analyzing"
	}
	b := m.L.ByID[in.BookID]
	m.mu.Unlock()
	m.hub.Publish("screen", "taste_update", map[string]any{"seat": seat, "tags": b.tagNames()})
	m.progress(s)
	core.JSON(w, map[string]any{"next": next})
}

// local computation for the first analysis (< 50 ms)
func (m *Module) compute(s *Session) *Trace {
	t0 := time.Now()
	var ops int64
	rounds := append([]Round(nil), s.Rounds...)
	p, per := m.L.profile(rounds, nil, &ops)
	cr := m.L.candidates(&p, s.shownIDs(), 12, 40, ops)
	tr := &Trace{Kind: "first", Rounds: rounds, PerRound: per, Profile: p, Scores: cr.Scores, Funnel: cr.Funnel, Candidates: cr.Cands, Ops: cr.Ops}
	tr.ComputeMs = math.Round(float64(time.Since(t0).Microseconds())/10) / 100
	ids := []string{}
	for _, r := range rounds {
		for _, x := range r.Shown {
			ids = append(ids, x.BookID)
		}
	}
	for _, c := range cr.Cands {
		ids = append(ids, c.ID)
	}
	tr.Books = m.L.Views(ids)
	return tr
}

func publicTrace(tr *Trace) *Trace {
	c := *tr
	c.LLM.Draft = ""
	return &c
}

func (m *Module) analyze(w http.ResponseWriter, r *http.Request) {
	seat := m.seat(w, r)
	if seat == 0 {
		return
	}
	s := m.get(seat)
	m.mu.Lock()
	if s.picked() < 5 {
		m.mu.Unlock()
		core.Err(w, 409, "还没选完 5 轮", "stage")
		return
	}
	if s.Result != nil { // replay the recorded trajectory
		j := m.replayJob(s.Trace, s.Result)
		m.mu.Unlock()
		j.follow(r.Context(), w)
		return
	}
	j := s.job
	if j == nil {
		j = newJob()
		s.job = j
		s.Stage = "analyzing"
		tr := m.compute(s)
		j.push("trace", publicTrace(tr))
		um := m.L.UserMsg(tr.Rounds, &tr.Profile, tr.Candidates)
		first := m.prefix + "\n\n===== 下面是这位学生的数据 =====\n" + um
		local := m.L.LocalOutput(tr.Rounds, &tr.Profile, tr.Candidates, m.seed(s))
		chk := NewChecker(m.L, tr.Rounds, tr.Candidates, nil)
		attempt := s.Attempt
		go func() {
			res, raw := m.runModel(seat, j, tr, []llm.Msg{{Role: "user", Content: first}}, chk, local, nil, 1)
			m.mu.Lock()
			if s.Attempt == attempt && s.job == j {
				s.Trace, s.Result, s.Prompt, s.Raw = tr, res, first, raw
				s.Stage = "result"
			}
			m.mu.Unlock()
			j.push("final", map[string]any{"result": res})
			j.push("done", map[string]any{})
			j.finish()
			m.hub.Publish("screen", "analysis_done", map[string]any{"seat": seat, "fallback": res.Fallback, "ops": tr.Ops, "ms": tr.LLM.Ms})
			m.progress(s)
			m.app.SaveNow("jisuan")
		}()
		m.mu.Unlock()
		m.progress(s)
	} else {
		m.mu.Unlock()
	}
	j.follow(r.Context(), w)
}

// replayJob rebuilds the event stream of a finished analysis (再看一遍 / 刷新后).
func (m *Module) replayJob(tr *Trace, res *Result) *Job {
	j := newJob()
	if tr != nil {
		j.push("trace", publicTrace(tr))
		for i, t := range tr.LLM.Thoughts {
			j.push("thought", map[string]any{"i": i, "kind": t.Kind, "text": t.Text})
		}
	}
	if res != nil {
		j.push("persona", map[string]any{"name": res.Persona, "tags": res.TopTags})
		for _, p := range res.Picks {
			j.push("pick", p)
		}
		j.push("final", map[string]any{"result": res})
	}
	j.push("done", map[string]any{})
	j.finish()
	return j
}

// ---------------- 再想想 ----------------

var rePhone = regexp.MustCompile(`1[3-9]\d{9}|\d{7,}|\d{3,4}-\d{7,8}`)
var reAddr = regexp.MustCompile(`(路|街|巷|弄|小区|号楼|单元|栋|幢)\s*\d|\d+\s*(号|室|栋|幢|单元)|身份证|住在|我家在|电话`)

var synonyms = map[string][]string{
	"搞笑": {"搞笑", "好笑", "有趣", "幽默", "逗"}, "紧张刺激": {"刺激", "紧张", "惊险"}, "温暖感人": {"感人", "温暖", "感动", "催泪"},
	"烧脑": {"烧脑", "动脑", "谜题"}, "想象力": {"想象", "神奇", "脑洞"}, "知识多": {"知识", "涨知识", "学到"}, "励志": {"励志", "坚持", "奋斗"},
	"安静优美": {"优美", "安静", "美文"}, "冒险": {"冒险", "探险"}, "动物": {"动物", "小猫", "小狗", "猫", "狗"}, "科学": {"科学", "实验"},
	"侦探推理": {"侦探", "破案", "推理", "悬疑"}, "太空宇宙": {"太空", "宇宙", "星球", "外星"}, "历史": {"历史", "古代", "朝代"},
	"魔法奇幻": {"魔法", "巫师", "奇幻"}, "童话寓言": {"童话", "寓言"}, "科幻": {"科幻", "未来"}, "机器人科技": {"机器人", "科技"},
	"恐龙古生物": {"恐龙"}, "海洋": {"海洋", "大海", "海底"}, "校园生活": {"校园", "学校", "同学"}, "友谊": {"友谊", "朋友"},
	"家庭亲情": {"亲情", "家人", "妈妈", "爸爸"}, "神话传说": {"神话", "传说"}, "传统文化": {"传统", "国学"}, "数学": {"数学"},
	"地理旅行": {"旅行", "地理", "环游"}, "艺术体育": {"艺术", "体育", "运动", "画画", "音乐"}, "名人故事": {"名人", "传记", "科学家"},
}
var negators = []string{"不喜欢", "不想", "不爱", "讨厌", "不要", "别", "不太", "不怎么", "没意思", "受不了", "怕"}

func negated(text string, at int) bool {
	start := max(0, at-12) // bytes; ~4 CJK runes
	pre := text[start:at]
	for _, n := range negators {
		if strings.Contains(pre, n) {
			return true
		}
	}
	return false
}

// parseText turns the student's sentence into tag adjustments and a length preference.
func (m *Module) parseText(text string) (map[int]float64, *float64, []string) {
	adj := map[int]float64{}
	var notes []string
	for tag, words := range synonyms {
		for _, w := range words {
			if i := strings.Index(text, w); i >= 0 {
				ti, ok := m.L.TagIdx[tag]
				if !ok {
					break
				}
				if negated(text, i) {
					adj[ti] = -0.5
					notes = append(notes, "少推"+tag+"的书")
				} else {
					adj[ti] = 0.5
					notes = append(notes, "多找"+tag+"的书")
				}
				break
			}
		}
	}
	var L *float64
	set := func(v float64, n string) { L = &v; notes = append(notes, n) }
	switch {
	case strings.Contains(text, "太长") || strings.Contains(text, "太厚"):
		set(-1, "把厚书都往后排了")
	case strings.Contains(text, "长") || strings.Contains(text, "厚"):
		i := strings.Index(text, "长")
		if i < 0 {
			i = strings.Index(text, "厚")
		}
		if negated(text, i) {
			set(-1, "把厚书都往后排了")
		} else {
			set(1, "多找厚一点的书")
		}
	case strings.Contains(text, "短") || strings.Contains(text, "薄"):
		set(-1, "多找薄一点的书")
	}
	sort.Strings(notes)
	return adj, L, notes
}

func (m *Module) again(w http.ResponseWriter, r *http.Request) {
	seat := m.seat(w, r)
	if seat == 0 {
		return
	}
	var in struct {
		Likes map[string]bool `json:"likes"`
		Text  string          `json:"text"`
	}
	if core.Body(r, &in) != nil {
		core.Err(w, 400, "格式不对", "bad_body")
		return
	}
	in.Text = clip(strings.TrimSpace(in.Text), 60)
	if rePhone.MatchString(in.Text) || reAddr.MatchString(in.Text) {
		core.Err(w, 400, "不用告诉元小智这些哦", "personal_info")
		return
	}
	for _, sw := range m.sensitive {
		if strings.Contains(in.Text, sw) {
			core.Err(w, 400, "这句话换个说法吧", "sensitive")
			return
		}
	}
	s := m.get(seat)
	m.mu.Lock()
	if s.AgainTrace != nil && s.Again != nil {
		j := m.replayJob(s.AgainTrace, s.Again)
		m.mu.Unlock()
		j.follow(r.Context(), w)
		return
	}
	if s.againJob != nil {
		j := s.againJob
		m.mu.Unlock()
		j.follow(r.Context(), w)
		return
	}
	if s.Result == nil {
		m.mu.Unlock()
		core.Err(w, 409, "先拿到推荐再说", "stage")
		return
	}
	t0 := time.Now()
	var like, dislike []string
	old := map[string]bool{}
	for _, p := range s.Result.Picks {
		old[p.BookID] = true
		if v, ok := in.Likes[p.BookID]; ok {
			if v {
				like = append(like, p.BookID)
			} else {
				dislike = append(dislike, p.BookID)
			}
		}
	}
	adj, lenTo, notes := m.parseText(in.Text)
	var ops int64
	rounds := s.Rounds
	before := s.Trace.Profile
	p, _ := m.L.profile(rounds, &Extra{Like: like, Dislike: dislike, TagAdj: adj, LenTo: lenTo}, &ops)
	pb, _ := m.L.profile(rounds, nil, &ops)
	_ = before
	ex := s.shownIDs()
	banned := map[string]bool{}
	for id := range old {
		ex[id] = true
		banned[id] = true
	}
	cr := m.L.candidates(&p, ex, 12, 40, ops)
	var changes []TagDelta
	for i := 0; i < m.L.NTags; i++ {
		if d := p.P[i] - pb.P[i]; math.Abs(d) >= 0.05 {
			changes = append(changes, TagDelta{Tag: m.L.Names[i], V: math.Round(d*100) / 100})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return math.Abs(changes[i].V) > math.Abs(changes[j].V) })
	if len(changes) > 6 {
		changes = changes[:6]
	}
	tr := &Trace{Kind: "again", Rounds: rounds, Profile: p, Scores: cr.Scores, Funnel: cr.Funnel[4:], Candidates: cr.Cands, Ops: cr.Ops, Changes: changes, Struck: dislike, Liked: like, Text: in.Text}
	for id := range old {
		tr.OldPicks = append(tr.OldPicks, id)
	}
	sort.Strings(tr.OldPicks)
	ids := append([]string{}, tr.OldPicks...)
	for _, c := range cr.Cands {
		ids = append(ids, c.ID)
	}
	for _, r := range rounds {
		ids = append(ids, r.Chosen)
	}
	tr.Books = m.L.Views(ids)
	tr.ComputeMs = math.Round(float64(time.Since(t0).Microseconds())/10) / 100

	titles := func(ids []string) string {
		var t []string
		for _, id := range ids {
			t = append(t, m.L.ByID[id].Title)
		}
		return strings.Join(t, "、")
	}
	fb := "【学生的反馈】想读：" + orNone(titles(like)) + "；不想读：" + orNone(titles(dislike)) + "\n"
	fb += "【学生补充】“" + orNone(in.Text) + "”\n"
	fb += fmt.Sprintf("【更新后的画像】喜欢：%s；喜欢的体裁：%s；篇幅：%s\n", tagWs(p.Top), p.Form, p.LenWord)
	fb += "【新的候选书】\n"
	for _, c := range cr.Cands {
		b := m.L.ByID[c.ID]
		fb += fmt.Sprintf("%s《%s》%s｜%s;%s｜%s｜%s｜参考感兴趣程度 %d｜简介：%s\n", b.ID, b.Title, b.Author, strings.Join(b.Topic, ","), strings.Join(b.Tone, ","), b.Form, b.Length, c.Interest, b.Summary)
	}
	fb += "请按同样的格式重新推荐 3 本（不要重复之前推荐过的书），并在第一行“想|证据|”里说明你根据反馈改了什么。"
	msgs := []llm.Msg{{Role: "user", Content: s.Prompt}, {Role: "assistant", Content: s.Raw}, {Role: "user", Content: fb}}

	note := "我根据你的反馈重新找了一遍。"
	if len(dislike) > 0 {
		note = fmt.Sprintf("你不想读《%s》，和它像的书我都往后排了。", m.L.ByID[dislike[0]].Title)
	}
	if len(notes) > 0 {
		note = "你说“" + clip(in.Text, 14) + "”，我" + strings.Join(notes[:min(2, len(notes))], "，") + "。"
	}
	local := "想|证据|" + clip(note, 40) + "\n" + m.L.LocalOutput(rounds, &p, cr.Cands, m.seed(s)+99)
	chk := NewChecker(m.L, rounds, cr.Cands, banned)
	j := newJob()
	s.againJob = j
	s.Stage = "again"
	attempt := s.Attempt
	j.push("trace", publicTrace(tr))
	m.mu.Unlock()
	m.progress(s)
	go func() {
		res, _ := m.runModel(seat, j, tr, msgs, chk, local, banned, 2)
		res.Note = note
		m.mu.Lock()
		if s.Attempt == attempt && s.againJob == j {
			s.Again, s.AgainTrace, s.Stage = res, tr, "result"
		}
		m.mu.Unlock()
		j.push("final", map[string]any{"result": res})
		j.push("done", map[string]any{})
		j.finish()
		m.progress(s)
		m.app.SaveNow("jisuan")
	}()
	j.follow(r.Context(), w)
}

func orNone(s string) string {
	if s == "" {
		return "无"
	}
	return s
}

func tagWs(t []TagW) string {
	var out []string
	for _, x := range t {
		out = append(out, fmt.Sprintf("%s %.2f", x.Tag, x.V))
	}
	return strings.Join(out, "，")
}

// ---------------- trace & screen ----------------

func (m *Module) traceOut(w http.ResponseWriter, r *http.Request, seat int, screen bool) {
	m.mu.RLock()
	s := m.sess[seat]
	if s == nil {
		m.mu.RUnlock()
		core.Err(w, 404, "这台平板还没有记录", "none")
		return
	}
	att, _ := strconv.Atoi(r.URL.Query().Get("attempt"))
	rec := s.current()
	for _, h := range s.History {
		if h.Attempt == att {
			rec = h
		}
	}
	m.mu.RUnlock()
	fix := func(t *Trace) *Trace {
		if t == nil || screen {
			return t
		}
		return publicTrace(t)
	}
	out := map[string]any{"seat": seat, "attempt": rec.Attempt, "trace": fix(rec.Trace), "result": rec.Result, "again": rec.Again, "again_trace": fix(rec.AgainTrace), "stars": rec.Stars}
	if screen {
		out["attempts"] = len(s.History) + 1
		if rec.Result != nil {
			out["fallback"] = rec.Result.Fallback
		}
	}
	core.JSON(w, out)
}

func (m *Module) screenState(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	seats := []map[string]any{}
	taste := map[string]int{}
	recCount := map[string]int{}
	var interests []int
	stars := [6]int{}
	picks, ops := 0, int64(0)
	computeMs := 0.0
	add := func(a AttemptRecord) {
		for _, rd := range a.Rounds {
			if rd.Chosen != "" {
				picks++
				for _, t := range m.L.ByID[rd.Chosen].tagNames() {
					taste[t]++
				}
			}
		}
		for _, t := range []*Trace{a.Trace, a.AgainTrace} {
			if t != nil {
				ops += t.Ops
				computeMs += t.ComputeMs
			}
		}
		f := a.Result
		if a.Again != nil {
			f = a.Again
		}
		if f != nil {
			for _, p := range f.Picks {
				recCount[p.BookID]++
				interests = append(interests, p.Interest)
			}
		}
		if a.Stars > 0 {
			stars[a.Stars]++
		}
	}
	for _, s := range m.sess {
		seats = append(seats, m.seatView(s))
		for _, h := range s.History {
			add(h)
		}
		add(s.current())
	}
	type kv struct {
		ID string `json:"id"`
		N  int    `json:"n"`
	}
	var top []kv
	for id, n := range recCount {
		top = append(top, kv{id, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].N > top[j].N || (top[i].N == top[j].N && top[i].ID < top[j].ID) })
	if len(top) > 5 {
		top = top[:5]
	}
	ids := []string{}
	for _, t := range top {
		ids = append(ids, t.ID)
	}
	core.JSON(w, map[string]any{
		"seats": seats, "all_seats": m.app.Seats, "taste": taste, "picks": picks, "ops": ops, "compute_ms": computeMs,
		"interests": interests, "stars": stars, "top_books": top, "books": m.L.Views(ids), "paused": m.paused,
		"mode": m.app.ModeName(), "effort": m.app.Cfg().Effort[scene], "show_draft": m.app.Cfg().ShowDraft, "warm": m.warm,
	})
}

func (m *Module) control(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
		Effort string `json:"effort"`
		Seat   int    `json:"seat"`
	}
	if core.Body(r, &in) != nil {
		core.Err(w, 400, "格式不对", "bad_body")
		return
	}
	switch in.Action {
	case "start":
		m.mu.Lock()
		m.paused = false
		m.mu.Unlock()
		m.hub.Publish("student:all", "paused", map[string]bool{"paused": false})
		go m.Warmup()
	case "pause", "resume":
		m.mu.Lock()
		m.paused = in.Action == "pause"
		m.mu.Unlock()
		m.hub.Publish("student:all", "paused", map[string]bool{"paused": in.Action == "pause"})
	case "reset":
		m.Reset()
		m.app.SaveNow("jisuan")
	case "effort":
		if in.Effort == "off" || in.Effort == "low" || in.Effort == "medium" {
			m.app.SetCfg(func(c *core.Config) {
				if c.Effort == nil {
					c.Effort = map[string]string{}
				}
				c.Effort[scene] = in.Effort
			})
		}
	case "draft":
		m.app.SetCfg(func(c *core.Config) { c.ShowDraft = !c.ShowDraft })
	case "project": // 投屏：让这台平板显示“你的分析正在大屏上”
		m.hub.Publish("student:"+strconv.Itoa(in.Seat), "projected", map[string]any{})
	}
	m.app.MarkDirty("jisuan")
	core.JSON(w, map[string]any{"ok": true, "paused": m.paused, "effort": m.app.Cfg().Effort[scene], "show_draft": m.app.Cfg().ShowDraft})
}

// Warmup sends the fixed prefix once (max_tokens 1) so a supporting gateway caches it.
func (m *Module) Warmup() {
	if !m.gw.Configured() || m.app.ModeName() != "live" {
		m.warm = "跳过（非实时模式）"
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t0 := time.Now()
	u, err := m.gw.Stream(ctx, llm.Req{Scene: "jisuan.warmup", Messages: []llm.Msg{{Role: "user", Content: m.prefix + "\n\n===== 下面是这位学生的数据 =====\n（预热，回复“完”即可）"}}, Effort: "off", MaxTokens: 1, Priority: 0}, func(llm.Delta) {})
	if err != nil && ctx.Err() == nil {
		m.warm = "失败：" + clip(err.Error(), 60)
		return
	}
	m.warm = fmt.Sprintf("已预热 %s（%d token，%.1f 秒）", time.Now().Format("15:04"), u.Prompt, time.Since(t0).Seconds())
}

func (m *Module) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]any{"sessions": len(m.sess), "paused": m.paused, "warm": m.warm, "prefix_hash": m.prefixHash}
}
