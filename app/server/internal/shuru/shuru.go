// Package shuru is the 输入 stage: 秋假攻略. Students give 元小智 their travel preferences
// (chips + a free sentence, typed or via the tablet IME's voice key); the model recommends
// places in China and writes a day-by-day guide. Teaching point: the input decides the output.
package shuru

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"yxz/internal/core"
	"yxz/internal/llm"
)

const scene = "shuru.guide"
const maxGuides = 6

type Group struct {
	K     string   `json:"k"`
	T     string   `json:"t"`
	Multi bool     `json:"multi"`
	Items []string `json:"items"`
}

type Chips struct {
	FromDefault string  `json:"from_default"`
	Groups      []Group `json:"groups"`
	TextPh      string  `json:"text_ph"`
}

type LocalPlace struct {
	Name  string   `json:"name"`
	Prov  string   `json:"prov"`
	Scene []string `json:"scene"`
	Play  []string `json:"play"`
	Why   string   `json:"why"`
	Trip  string   `json:"trip"`
}

type Place struct {
	Name string   `json:"name"`
	Prov string   `json:"prov"`
	Why  string   `json:"why"`
	Hits []string `json:"hits"`
	Trip string   `json:"trip"`
}

type Day struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

type Guide struct {
	N        int                 `json:"n"`
	Kind     string              `json:"kind"` // ask | retry | focus
	Prefs    map[string][]string `json:"prefs"`
	Text     string              `json:"text"`
	Voice    bool                `json:"voice,omitempty"` // 用了输入法语音
	Focus    string              `json:"focus,omitempty"`
	Sent     string              `json:"sent"` // 元小智真正收到的输入
	NInfo    int                 `json:"n_info"`
	Places   []Place             `json:"places"`
	Title    string              `json:"title"`
	Days     []Day               `json:"days"`
	Bring    []string            `json:"bring"`
	Tip      string              `json:"tip"`
	FirstMs  int                 `json:"first_ms"`
	Ms       int                 `json:"ms"`
	Fallback bool                `json:"fallback,omitempty"`
	At       time.Time           `json:"at"`
}

type Student struct {
	Seat   int      `json:"seat"`
	Guides []*Guide `json:"guides"`
	busy   bool
}

type Module struct {
	app       *core.App
	gw        *llm.Gateway
	rp        *llm.Replay
	hub       *core.Hub
	chips     Chips
	places    []LocalPlace
	prefix    string
	sensitive []string

	mu   sync.RWMutex
	stus map[int]*Student
}

func New(app *core.App, gw *llm.Gateway, rp *llm.Replay) (*Module, error) {
	m := &Module{app: app, gw: gw, rp: rp, hub: app.Hub("shuru"), stus: map[int]*Student{}}
	dir := filepath.Join(app.Content, "shuru")
	read := func(p string, v any) error {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, v); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		return nil
	}
	if err := read("chips.json", &m.chips); err != nil {
		return nil, err
	}
	if err := read("places.json", &m.places); err != nil {
		return nil, err
	}
	persona, _ := os.ReadFile(filepath.Join(app.Content, "common", "persona.md"))
	rules, err := os.ReadFile(filepath.Join(dir, "prompts", "guide_rules.md"))
	if err != nil {
		return nil, err
	}
	m.prefix = strings.TrimSpace(string(persona)) + "\n\n" + strings.TrimSpace(string(rules))
	b, _ := os.ReadFile(filepath.Join(app.Content, "common", "sensitive_words.txt"))
	for _, w := range strings.Split(string(b), "\n") {
		if w = strings.TrimSpace(w); w != "" && !strings.HasPrefix(w, "#") {
			m.sensitive = append(m.sensitive, w)
		}
	}
	return m, nil
}

func (m *Module) Name() string { return "shuru" }
func (m *Module) Snapshot() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.stus)
}
func (m *Module) Restore(b []byte) error { return json.Unmarshal(b, &m.stus) }
func (m *Module) Reset() {
	m.mu.Lock()
	m.stus = map[int]*Student{}
	m.mu.Unlock()
	m.hub.Publish("all", "reset", map[string]any{})
}
func (m *Module) Check(fs.FS) []error {
	if len(m.places) < 3 {
		return []error{fmt.Errorf("places.json 至少要 3 个地点")}
	}
	return nil
}

func (m *Module) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, s := range m.stus {
		n += len(s.Guides)
	}
	return map[string]any{"students": len(m.stus), "guides": n}
}

// ---------------- 输入 ----------------

var rePhone = regexp.MustCompile(`1[3-9]\d{9}|\d{7,}|\d{3,4}-\d{7,8}`)
var reAddr = regexp.MustCompile(`(路|街|巷|弄|小区|号楼|单元|栋|幢)\s*\d|\d+\s*(号|室|栋|幢|单元)|身份证|住在|我家在|电话`)

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > n {
		return string([]rune(s)[:n])
	}
	return s
}

// clean keeps only chips that exist; returns the composed sentence the model will receive.
func (m *Module) compose(prefs map[string][]string, text, focus string) (map[string][]string, string, int) {
	out := map[string][]string{}
	var parts []string
	n := 0
	for _, g := range m.chips.Groups {
		ok := map[string]bool{}
		for _, it := range g.Items {
			ok[it] = true
		}
		var sel []string
		for _, v := range prefs[g.K] {
			if ok[v] && (g.Multi || len(sel) == 0) {
				sel = append(sel, v)
			}
		}
		if len(sel) == 0 {
			continue
		}
		out[g.K] = sel
		n += len(sel)
		parts = append(parts, g.T+"："+strings.Join(sel, "、"))
	}
	if text != "" {
		parts = append(parts, "补充："+text)
		n++
	}
	sent := strings.Join(parts, "；")
	if sent == "" {
		sent = "（同学什么信息也没给）"
	}
	sent += "\n出发地：" + m.chips.FromDefault + "（学校所在地）"
	if focus != "" {
		sent += "\n同学想看「" + focus + "」的攻略：请把它放在第一个推荐，并写它的攻略。"
	}
	return out, sent, n
}

// ---------------- 解析 ----------------

var reMD = regexp.MustCompile(`^[#*\->\s]+|\*\*`)

func split(line string) []string {
	line = strings.ReplaceAll(line, "｜", "|")
	line = reMD.ReplaceAllString(strings.TrimSpace(line), "")
	ps := strings.Split(line, "|")
	for i := range ps {
		ps[i] = strings.TrimSpace(ps[i])
	}
	return ps
}

func fields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == ' ' })
}

var reDay = regexp.MustCompile(`^第\s*([0-9一二三四五六七])\s*天`)
var cn = map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7}

type parser struct {
	g    *Guide
	push func(ev string, v any)
}

func (p *parser) line(l string) {
	ps := split(l)
	if len(ps) == 0 || ps[0] == "" {
		return
	}
	g := p.g
	switch {
	case ps[0] == "推荐" && len(ps) >= 4 && len(g.Places) < 3:
		pl := Place{Name: clip(ps[1], 14), Prov: clip(ps[2], 8), Why: clip(ps[3], 50)}
		if len(ps) >= 5 {
			for _, h := range fields(ps[4]) {
				if len(pl.Hits) < 4 {
					pl.Hits = append(pl.Hits, clip(h, 8))
				}
			}
		}
		if len(ps) >= 6 {
			pl.Trip = clip(ps[5], 30)
		}
		if pl.Name == "" {
			return
		}
		for _, x := range g.Places {
			if x.Name == pl.Name {
				return
			}
		}
		g.Places = append(g.Places, pl)
		p.push("place", pl)
	case ps[0] == "攻略" && len(ps) >= 2 && g.Title == "":
		t := ""
		if len(ps) >= 3 {
			t = ps[2]
		}
		g.Title = clip(t, 40)
		p.push("title", map[string]string{"place": clip(ps[1], 14), "title": g.Title})
	case reDay.MatchString(ps[0]) && len(ps) >= 2 && len(g.Days) < 7:
		mm := reDay.FindStringSubmatch(ps[0])
		n, ok := cn[mm[1]]
		if !ok {
			n, _ = strconv.Atoi(mm[1])
		}
		d := Day{N: n, Text: clip(strings.Join(ps[1:], "；"), 90)}
		g.Days = append(g.Days, d)
		p.push("day", d)
	case ps[0] == "带上" && len(ps) >= 2 && len(g.Bring) == 0:
		for _, b := range fields(ps[1]) {
			if len(g.Bring) < 6 {
				g.Bring = append(g.Bring, clip(b, 10))
			}
		}
		p.push("bring", g.Bring)
	case ps[0] == "提醒" && len(ps) >= 2 && g.Tip == "":
		g.Tip = clip(ps[1], 50)
		p.push("tip", g.Tip)
	}
}

// ---------------- 本地兜底：按偏好给书库里的地点打分 ----------------

var bringBy = map[string][]string{"大海": {"防晒霜", "小水桶和铲子", "防滑凉鞋"}, "高山": {"舒服的运动鞋", "外套", "登山杖"}, "城市": {"公交卡", "舒服的鞋"}, "草原": {"厚外套", "帽子"}, "沙漠": {"防风纱巾", "墨镜"}, "森林": {"驱蚊水", "长袖长裤"}}

func (m *Module) local(prefs map[string][]string, text, focus string, rng *rand.Rand) string {
	want := map[string]bool{}
	for _, v := range prefs["scene"] {
		want[v] = true
	}
	for _, v := range prefs["play"] {
		want[v] = true
	}
	type sc struct {
		p    LocalPlace
		s    float64
		hits []string
	}
	var all []sc
	for _, p := range m.places {
		var hits []string
		for _, t := range append(append([]string{}, p.Scene...), p.Play...) {
			if want[t] || (text != "" && strings.Contains(text, t)) {
				hits = append(hits, t)
			}
		}
		s := float64(len(hits)) + rng.Float64()*0.6
		if focus != "" && strings.Contains(p.Name, focus) {
			s += 100
		}
		all = append(all, sc{p, s, hits})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].s > all[j].s })
	from := m.chips.FromDefault
	if f := prefs["from"]; len(f) > 0 {
		from = f[0]
	}
	days := 3
	if d := prefs["days"]; len(d) > 0 {
		days, _ = strconv.Atoi(strings.TrimSuffix(d[0], "天"))
	}
	var sb strings.Builder
	for _, x := range all[:3] {
		hits := x.hits
		if len(hits) == 0 {
			hits = x.p.Scene[:1]
		}
		why := "你说想要" + strings.Join(hits[:min(2, len(hits))], "、") + "，" + x.p.Why
		fmt.Fprintf(&sb, "推荐|%s|%s|%s|%s|从%s出发，%s\n", x.p.Name, x.p.Prov, clip(why, 40), strings.Join(hits, ","), from, x.p.Trip)
	}
	top := all[0].p
	fmt.Fprintf(&sb, "攻略|%s|%d 天把%s玩个痛快\n", top.Name, days, top.Name)
	acts := append([]string{}, top.Play...)
	for d := 1; d <= days; d++ {
		a := acts[(d-1)%len(acts)]
		switch {
		case d == 1:
			fmt.Fprintf(&sb, "第%d天|上午从%s出发；下午到%s住下，先%s；晚上早点休息\n", d, from, top.Name, a)
		case d == days:
			fmt.Fprintf(&sb, "第%d天|上午%s；下午买点纪念品；傍晚返程回%s\n", d, a, from)
		default:
			fmt.Fprintf(&sb, "第%d天|上午%s；下午%s；晚上和家人尝尝当地小吃\n", d, a, acts[d%len(acts)])
		}
	}
	bring := []string{"水杯", "充电宝"}
	for _, s := range top.Scene {
		bring = append(bring, bringBy[s]...)
	}
	fmt.Fprintf(&sb, "带上|%s\n", strings.Join(bring[:min(5, len(bring))], ","))
	sb.WriteString("提醒|人多的地方一定跟紧大人，记住爸爸妈妈的电话\n完")
	return sb.String()
}

// ---------------- 生成 ----------------

func (m *Module) generate(seat int, g *Guide, push func(string, any)) {
	cfg := m.app.Cfg()
	mode := m.app.ModeName()
	t0 := time.Now()
	var mu sync.Mutex
	stopped := false
	p := &parser{g: g, push: func(ev string, v any) { push(ev, v) }}
	gotFirst := make(chan struct{})
	var once sync.Once
	handle := func(l string) {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return
		}
		before := len(g.Places)
		p.line(l)
		if len(g.Places) > before {
			once.Do(func() { g.FirstMs = int(time.Since(t0).Milliseconds()); close(gotFirst) })
		}
	}
	rng := rand.New(rand.NewPCG(uint64(seat), uint64(t0.UnixNano())))
	simulate := func(text string) {
		for _, l := range strings.Split(text, "\n") {
			if strings.TrimSpace(l) != "" {
				time.Sleep(time.Duration(250+rng.IntN(350)) * time.Millisecond)
				handle(l)
			}
		}
	}
	localText := func() string { return m.local(g.Prefs, g.Text, g.Focus, rng) }
	switch {
	case mode == "live" && cfg.UseLLM && m.gw.Configured():
		effort := cfg.Effort[scene]
		temp := 0.8
		if g.Kind == "retry" {
			temp = 1.0
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		go func() {
			select {
			case <-gotFirst:
			case <-time.After(30 * time.Second): // 30 秒还没出第一个推荐：改用本地结果
				cancel()
			case <-ctx.Done():
			}
		}()
		var raw strings.Builder
		onDelta, flush := llm.Lines(func(l string) { raw.WriteString(l + "\n"); handle(l) }, nil)
		_, err := m.gw.Stream(ctx, llm.Req{Scene: scene, Messages: []llm.Msg{{Role: "user", Content: m.prefix + "\n\n===== 下面是这位同学给你的信息 =====\n" + g.Sent}},
			Effort: effort, Temperature: temp, MaxTokens: 1500, Priority: 1}, onDelta)
		flush()
		cancel()
		mu.Lock()
		empty := len(g.Places) == 0
		mu.Unlock()
		if empty {
			m.gw.Stats(scene).Fallbacks.Add(1)
			m.app.Logger.Printf("shuru seat %d: fallback (err=%v) raw=%q", seat, err, clip(raw.String(), 200))
			g.Fallback = true
			simulate(localText())
		} else if err == nil {
			m.rp.Add(llm.Record{Scene: scene, Features: map[string]any{"sent": g.Sent}, Output: raw.String()})
		}
	case mode == "replay":
		if rec, ok := m.rp.Best(scene, func(r llm.Record) float64 {
			s, _ := r.Features["sent"].(string)
			if s == g.Sent {
				return 10 + rng.Float64()
			}
			n := 0.0
			for _, part := range strings.Split(g.Sent, "；") {
				if strings.Contains(s, part) {
					n++
				}
			}
			return n
		}); ok {
			simulate(rec.Output)
		} else {
			g.Fallback = true
			simulate(localText())
		}
	default:
		g.Fallback = true
		simulate(localText())
	}
	mu.Lock()
	stopped = true
	// 缺什么补什么（格式和模型一样）
	if len(g.Places) == 0 || len(g.Days) == 0 {
		lp := &parser{g: &Guide{}, push: func(string, any) {}}
		for _, l := range strings.Split(localText(), "\n") {
			lp.line(l)
		}
		if len(g.Places) == 0 {
			g.Places = lp.g.Places
			for _, pl := range g.Places {
				push("place", pl)
			}
		}
		if len(g.Days) == 0 && strings.Contains(lp.g.Title, "") {
			if g.Title == "" {
				g.Title = lp.g.Title
				push("title", map[string]string{"place": g.Places[0].Name, "title": g.Title})
			}
			g.Days = lp.g.Days
			for _, d := range g.Days {
				push("day", d)
			}
		}
		if len(g.Bring) == 0 {
			g.Bring = lp.g.Bring
			push("bring", g.Bring)
		}
		if g.Tip == "" {
			g.Tip = lp.g.Tip
			push("tip", g.Tip)
		}
		g.Fallback = true
	}
	g.Ms = int(time.Since(t0).Milliseconds())
	if g.FirstMs == 0 {
		g.FirstMs = g.Ms
	}
	mu.Unlock()
}

// ---------------- 视图 ----------------

func (m *Module) summary(s *Student) map[string]any {
	v := map[string]any{"seat": s.Seat, "group": m.app.GroupOf(s.Seat), "busy": s.busy, "guides": len(s.Guides)}
	if n := len(s.Guides); n > 0 {
		g := s.Guides[n-1]
		v["sent"], v["n_info"], v["prefs"], v["text"], v["voice"] = g.Sent, g.NInfo, g.Prefs, g.Text, g.Voice
		if len(g.Places) > 0 {
			v["top"] = g.Places[0].Name
		}
	}
	return v
}

func (m *Module) stu(seat int) *Student {
	s := m.stus[seat]
	if s == nil {
		s = &Student{Seat: seat}
		m.stus[seat] = s
	}
	return s
}

func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/shuru/meta", func(w http.ResponseWriter, r *http.Request) { core.JSON(w, m.chips) })
	mux.HandleFunc("GET /api/shuru/state", func(w http.ResponseWriter, r *http.Request) {
		seat := m.app.SeatOf(r)
		if seat == 0 {
			core.Err(w, 401, "先选一下座位号吧", "no_seat")
			return
		}
		m.mu.RLock()
		defer m.mu.RUnlock()
		var gs []*Guide
		if s := m.stus[seat]; s != nil {
			gs = s.Guides
		}
		core.JSON(w, map[string]any{"guides": gs, "max": maxGuides})
	})
	mux.HandleFunc("POST /api/shuru/ask", m.ask)
	mux.HandleFunc("GET /api/shuru/screen/state", m.app.Teacher(func(w http.ResponseWriter, r *http.Request) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		list := []map[string]any{}
		for _, s := range m.stus {
			list = append(list, m.summary(s))
		}
		core.JSON(w, map[string]any{"students": list, "all_seats": m.app.Seats, "chips": m.chips, "mode": m.app.ModeName()})
	}))
	mux.HandleFunc("GET /api/shuru/screen/seat", m.app.Teacher(func(w http.ResponseWriter, r *http.Request) {
		seat, _ := strconv.Atoi(r.URL.Query().Get("seat"))
		m.mu.RLock()
		defer m.mu.RUnlock()
		s := m.stus[seat]
		if s == nil {
			core.Err(w, 404, "这台平板还没有提交", "none")
			return
		}
		core.JSON(w, map[string]any{"seat": seat, "guides": s.Guides})
	}))
	mux.HandleFunc("POST /api/shuru/screen/control", m.app.Teacher(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Action string }
		core.Body(r, &in)
		switch in.Action {
		case "look":
			m.hub.Publish("student:all", "look", map[string]any{"on": true})
		case "unlook":
			m.hub.Publish("student:all", "look", map[string]any{"on": false})
		case "reset":
			m.Reset()
			m.app.SaveNow("shuru")
		}
		core.JSON(w, map[string]any{"ok": true})
	}))
}

func (m *Module) ask(w http.ResponseWriter, r *http.Request) {
	seat := m.app.SeatOf(r)
	if seat == 0 {
		core.Err(w, 401, "先选一下座位号吧", "no_seat")
		return
	}
	var in struct {
		Prefs map[string][]string `json:"prefs"`
		Text  string              `json:"text"`
		Voice bool                `json:"voice"`
		Retry bool                `json:"retry"`
		Focus string              `json:"focus"`
	}
	if core.Body(r, &in) != nil {
		core.Err(w, 400, "格式不对", "bad_body")
		return
	}
	in.Text = clip(strings.Join(strings.Fields(in.Text), " "), 80)
	in.Focus = clip(in.Focus, 14)
	if rePhone.MatchString(in.Text) || reAddr.MatchString(in.Text) {
		core.Err(w, 400, "不用告诉元小智电话、住址这些哦，删掉再发吧", "personal_info")
		return
	}
	for _, sw := range m.sensitive {
		if strings.Contains(in.Text, sw) {
			core.Err(w, 400, "这句话换个说法吧", "sensitive")
			return
		}
	}
	m.mu.Lock()
	s := m.stu(seat)
	if s.busy {
		m.mu.Unlock()
		core.Err(w, 409, "元小智正在写上一份攻略，等一下", "busy")
		return
	}
	if len(s.Guides) >= maxGuides {
		m.mu.Unlock()
		core.Err(w, 409, fmt.Sprintf("已经生成了 %d 份攻略啦，去看看大屏吧", maxGuides), "max")
		return
	}
	g := &Guide{N: len(s.Guides) + 1, Kind: "ask", At: time.Now()}
	if in.Retry && len(s.Guides) > 0 { // 再试一次：输入完全不变
		last := s.Guides[len(s.Guides)-1]
		g.Kind, g.Prefs, g.Text, g.Voice, g.Focus, g.Sent, g.NInfo = "retry", last.Prefs, last.Text, last.Voice, last.Focus, last.Sent, last.NInfo
	} else {
		g.Prefs, g.Sent, g.NInfo = m.compose(in.Prefs, in.Text, in.Focus)
		g.Text, g.Voice, g.Focus = in.Text, in.Voice, in.Focus
		if in.Focus != "" {
			g.Kind = "focus"
		}
	}
	s.busy = true
	sum := m.summary(s)
	m.mu.Unlock()
	m.hub.Publish("screen", "seat_input", sum)

	st := core.NewStream(w)
	defer st.Close()
	st.Send("input", map[string]any{"n": g.N, "sent": g.Sent, "n_info": g.NInfo, "kind": g.Kind})
	m.generate(seat, g, func(ev string, v any) { st.Send(ev, v) })

	m.mu.Lock()
	s.Guides = append(s.Guides, g)
	s.busy = false
	sum = m.summary(s)
	m.mu.Unlock()
	m.hub.Publish("screen", "seat_guide", sum)
	m.app.SaveNow("shuru")
	st.Send("final", g)
	st.Send("done", map[string]any{})
}
