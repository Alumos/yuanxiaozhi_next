// Package shuchu is the 输出 stage: 疏散路线图发布会.
package shuchu

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	mrand "math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"yxz/internal/core"
	"yxz/internal/llm"
)

const scene = "shuchu.route_draw"
const maxVersions = 5

type Card struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
	Miss  string `json:"miss"`
}

type Input struct {
	Cards [6]bool   `json:"cards"`
	Notes [6]string `json:"notes"`
	Req   string    `json:"req"`
}

type Version struct {
	N        int        `json:"n"`
	Kind     string     `json:"kind"` // ask | retry
	Seat     int        `json:"seat"`
	Input    Input      `json:"input"`
	Plan     *GroupPlan `json:"plan,omitempty"`
	Spec     *Spec      `json:"spec,omitempty"`
	Image    string     `json:"image,omitempty"`
	Issues   []Issue    `json:"issues,omitempty"`
	Fallback bool       `json:"fallback,omitempty"`
	Verdict  string     `json:"verdict,omitempty"` // pass | fail
	At       time.Time  `json:"at"`
}

type ChatMsg struct {
	Role    string `json:"role"` // user | ai
	Seat    int    `json:"seat,omitempty"`
	Text    string `json:"text"`
	Version int    `json:"version,omitempty"`
	Thumb   bool   `json:"thumb,omitempty"` // 附带了校园平面图
	Pending bool   `json:"pending,omitempty"`
}

// Group is one tablet's workspace (每台平板独立). N is the tablet's group = the class it plans for.
type Group struct {
	N        int       `json:"n"`
	Seat     int       `json:"seat"`
	Input    Input     `json:"input"`
	Versions []*Version `json:"versions"`
	Chat     []ChatMsg `json:"chat"`
	Busy     int       `json:"busy"` // seat that is generating (0 none)
	LastBy   int       `json:"last_by"`
}

type FiveItem struct {
	I      int     `json:"i"`
	Spec   *Spec   `json:"spec"`
	Stair  string  `json:"stair"`
	Issues []Issue `json:"issues"`
	Ms     int     `json:"ms"`
}

type Five struct {
	Group  int            `json:"group"`
	Items  []*FiveItem    `json:"items"`
	Votes  map[int]string `json:"votes"` // seat -> same|diff
	Open   bool           `json:"open"`
	Running bool          `json:"running"`
}

type Module struct {
	app     *core.App
	gw      *llm.Gateway
	rp      *llm.Replay
	hub     *core.Hub
	G       *Graph
	classes []Class
	cards   []Card
	chips   json.RawMessage
	stairTx map[string]string
	th      Thresholds
	rules   string
	prefix  string
	up      string

	mu     sync.RWMutex
	ws     map[int]*Group // seat -> workspace
	plan   *PlanTrace
	five   *Five
	phase  int // 1 = 任务一：给元小智信息；2 = 任务二：优化路线图（教师在大屏切换）
}

func New(app *core.App, gw *llm.Gateway, rp *llm.Replay) (*Module, error) {
	dir := filepath.Join(app.Content, "shuchu")
	g, err := LoadGraph(filepath.Join(dir, "campus", "road_graph.json"))
	if err != nil {
		return nil, err
	}
	m := &Module{app: app, gw: gw, rp: rp, hub: app.Hub("shuchu"), G: g, ws: map[int]*Group{}}
	if err := loadJSON(filepath.Join(dir, "campus", "classes.json"), &m.classes); err != nil {
		return nil, err
	}
	if err := loadJSON(filepath.Join(dir, "cards.json"), &m.cards); err != nil {
		return nil, err
	}
	var st struct {
		StairText map[string]string  `json:"stair_text"`
		Colors    map[string]float64 `json:"colors"`
	}
	if err := loadJSON(filepath.Join(dir, "campus", "stairs.json"), &st); err != nil {
		return nil, err
	}
	m.stairTx, m.th = st.StairText, Thresholds{Green: st.Colors["green"], Red: st.Colors["red"]}
	m.chips, _ = os.ReadFile(filepath.Join(dir, "chips.json"))
	rb, _ := os.ReadFile(filepath.Join(dir, "campus", "rules.md"))
	m.rules = strings.TrimSpace(string(rb))
	persona, _ := os.ReadFile(filepath.Join(app.Content, "common", "persona.md"))
	rules, _ := os.ReadFile(filepath.Join(dir, "prompts", "route_rules.md"))
	var nt strings.Builder
	nt.WriteString("【节点表】编号|名称\n")
	for _, n := range g.Nodes {
		if n.Kind != "guess" {
			fmt.Fprintf(&nt, "%s|%s\n", n.ID, n.Name)
		}
	}
	m.prefix = strings.TrimSpace(string(persona)) + "\n\n" + strings.TrimSpace(string(rules)) + "\n\n" + strings.TrimSpace(nt.String())
	m.up = filepath.Join(app.Data, "uploads", "shuchu")
	os.MkdirAll(m.up, 0o755)
	m.reset()
	return m, nil
}

func (m *Module) reset() {
	m.ws = map[int]*Group{}
	m.plan, m.five = nil, nil
	m.phase = 1
}

func (m *Module) Name() string { return "shuchu" }

func (m *Module) Snapshot() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(map[string]any{"seats": m.ws, "plan": m.plan, "five": m.five, "phase": m.phase})
}

func (m *Module) Restore(b []byte) error {
	var v struct {
		Seats  map[int]*Group `json:"seats"`
		Plan   *PlanTrace     `json:"plan"`
		Five   *Five          `json:"five"`
		Phase  int            `json:"phase"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	for k, g := range v.Seats {
		g.Busy = 0
		for i := range g.Chat {
			g.Chat[i].Pending = false
		}
		m.ws[k] = g
	}
	m.plan, m.five = v.Plan, v.Five
	if v.Phase == 2 {
		m.phase = 2
	}
	if m.five != nil {
		m.five.Running = false
	}
	return nil
}

func (m *Module) Reset() {
	m.mu.Lock()
	m.reset()
	m.mu.Unlock()
	m.hub.Publish("all", "reset", map[string]any{})
}

func (m *Module) Check(content fs.FS) []error {
	var errs []error
	all := map[int]Cands{}
	known := map[int]bool{}
	for _, c := range m.classes {
		if m.G.by[c.Start] == nil {
			errs = append(errs, fmt.Errorf("classes.json：%s班的起点 %s 不在道路网里", c.Class, c.Start))
			continue
		}
		info := Cands{Stairs: true, Roads: true, Crowd: true, Goal: true}
		all[c.Group], known[c.Group] = info, true
		cs := m.G.candidates(c, info)
		stairs := map[string]bool{}
		for _, r := range cs {
			stairs[r.Stair] = true
			if is := m.G.Verify(c, r.Path, nil, nil, true); failed(is) {
				errs = append(errs, fmt.Errorf("%s班候选路线核验没通过：%v", c.Class, is))
			}
		}
		if len(stairs) < 2 {
			errs = append(errs, fmt.Errorf("%s班不到 2 条不同楼梯的路线", c.Class))
		}
	}
	tr := m.G.Plan(m.classes, all, known, m.th)
	if tr.FirstRed < 0 {
		errs = append(errs, fmt.Errorf("规划器：全班都给了信息时没有出现拥堵（红色），课堂上的“暂停判断”会落空"))
	}
	return errs
}

func (m *Module) class(n int) Class {
	for _, c := range m.classes {
		if c.Group == n {
			return c
		}
	}
	return Class{}
}

// cardText fills one card's sentence for a class.
func (m *Module) cardText(c Class, id int) string {
	t := m.cards[id-1].Text
	cong := ""
	if id == 4 {
		// who shares this class's usual (nearest) stair
		base := m.G.Plan(m.classes, map[int]Cands{}, map[int]bool{}, m.th)
		st := base.Rounds[0].Assign[c.Group]
		var others []string
		for _, g := range base.Rounds[0].Loads[st].Groups {
			if g != c.Group {
				others = append(others, m.class(g).Class)
			}
		}
		if len(others) > 0 {
			cong = strings.Join(others, "、") + "也走" + m.G.by[st].Name + "，"
		}
	}
	r := strings.NewReplacer("{class}", c.Class, "{building}", c.Building, "{floor}", strconv.Itoa(c.Floor), "{position}", c.Position,
		"{people}", strconv.Itoa(c.People), "{stairs}", m.stairTx[c.Building], "{congestion}", cong)
	return r.Replace(t)
}

func (m *Module) infoOf(in Input) Cands {
	return Cands{Stairs: in.Cards[1], Roads: in.Cards[2], Crowd: in.Cards[3], Goal: in.Cards[4]}
}

// latest returns the most recent version submitted by any tablet of group n.
func (m *Module) latest(n int) *Version {
	var best *Version
	for _, g := range m.ws {
		if g.N != n || len(g.Versions) == 0 {
			continue
		}
		if v := g.Versions[len(g.Versions)-1]; best == nil || v.At.After(best.At) {
			best = v
		}
	}
	return best
}

// runPlanLocked plans the whole school: each class uses the latest map one of its tablets asked for.
// With self != nil, that tablet's class uses in (the request being made right now).
func (m *Module) runPlanLocked(self *Group, in Input) *PlanTrace {
	info := map[int]Cands{}
	known := map[int]bool{}
	for _, c := range m.classes {
		if v := m.latest(c.Group); v != nil {
			info[c.Group], known[c.Group] = m.infoOf(v.Input), v.Input.Cards[0]
		}
	}
	if self != nil {
		info[self.N], known[self.N] = m.infoOf(in), in.Cards[0]
	}
	m.plan = m.G.Plan(m.classes, info, known, m.th)
	return m.plan
}

func (m *Module) groupPlan(tr *PlanTrace, n int) *GroupPlan {
	for i := range tr.Groups {
		if tr.Groups[i].Group == n {
			gp := tr.Groups[i]
			return &gp
		}
	}
	return nil
}

// ---------------- views ----------------

func (m *Module) groupView(g *Group) map[string]any {
	c := m.class(g.N)
	cards := make([]map[string]any, len(m.cards))
	for i, cd := range m.cards {
		cards[i] = map[string]any{"id": cd.ID, "title": cd.Title, "text": m.cardText(c, cd.ID), "miss": cd.Miss, "on": g.Input.Cards[i], "note": g.Input.Notes[i]}
	}
	vs := make([]map[string]any, 0, len(g.Versions))
	for _, v := range g.Versions {
		vs = append(vs, map[string]any{"n": v.N, "kind": v.Kind, "seat": v.Seat, "spec": v.Spec, "image": v.Image, "plan": v.Plan, "input": v.Input})
	}
	return map[string]any{"group": g.N, "class": c, "cards": cards, "req": g.Input.Req, "versions": vs, "chat": g.Chat, "busy": g.Busy, "last_by": g.LastBy, "max_versions": maxVersions, "phase": m.phase}
}

// pushGroup sends a tablet its own workspace (only that tablet) and the screen its class summary.
func (m *Module) pushGroup(g *Group, by int, what string) {
	v := m.groupView(g)
	v["seat"] = g.Seat
	m.hub.Publish("student:"+strconv.Itoa(g.Seat), "group_update", v)
	m.hub.Publish("screen", "group_update", m.groupSummary(g.N))
	m.app.MarkDirty("shuchu")
}

// tablets of group n, by seat number.
func (m *Module) tablets(n int) []*Group {
	var out []*Group
	for _, g := range m.ws {
		if g.N == n {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seat < out[j].Seat })
	return out
}

// wall lists, for the screen, the newest map of each tablet in group n (index+1 is the screen's version number).
func (m *Module) wall(n int) []*Group {
	var out []*Group
	for _, g := range m.tablets(n) {
		if len(g.Versions) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// groupSummary is one class card on the big screen, built from all tablets of that group.
func (m *Module) groupSummary(n int) map[string]any {
	c := m.class(n)
	vs := []map[string]any{}
	for i, g := range m.wall(n) {
		v := g.Versions[len(g.Versions)-1]
		vs = append(vs, map[string]any{"n": i + 1, "seat": g.Seat, "sn": v.N, "kind": v.Kind, "image": v.Image, "spec": v.Spec, "issues": v.Issues, "verdict": v.Verdict, "fallback": v.Fallback, "stair": stairOf(m.G, v.Spec)})
	}
	var counts [6]int
	tabs := []map[string]any{}
	busy, reqLen := 0, 0
	for _, g := range m.tablets(n) {
		for i, on := range g.Input.Cards {
			if on {
				counts[i]++
			}
		}
		if g.Busy != 0 {
			busy++
		}
		if l := len([]rune(g.Input.Req)); l > reqLen {
			reqLen = l
		}
		tabs = append(tabs, map[string]any{"seat": g.Seat, "cards": g.Input.Cards, "versions": len(g.Versions), "busy": g.Busy != 0})
	}
	return map[string]any{"group": n, "class": c.Class, "counts": counts, "tablets": tabs, "req_len": reqLen, "versions": vs, "busy": busy}
}

func stairOf(g *Graph, sp *Spec) string {
	if sp == nil {
		return ""
	}
	for _, id := range sp.Path {
		if n := g.by[id]; n != nil && (n.Kind == "stair" || n.Kind == "guess") {
			return n.Name
		}
	}
	return ""
}

func (m *Module) mine(w http.ResponseWriter, r *http.Request) (int, *Group) {
	seat := m.app.SeatOf(r)
	if seat == 0 {
		core.Err(w, 401, "先选一下座位号吧", "no_seat")
		return 0, nil
	}
	g := m.ws[seat]
	if g == nil {
		n := m.app.GroupOf(seat)
		if m.class(n).Group == 0 {
			core.Err(w, 404, "没有找到你的小组", "no_group")
			return 0, nil
		}
		g = &Group{N: n, Seat: seat}
		m.ws[seat] = g
	}
	return seat, g
}

// ---------------- routes ----------------

func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/shuchu/graph", func(w http.ResponseWriter, r *http.Request) {
		core.JSON(w, map[string]any{"nodes": m.G.Nodes, "edges": m.G.Edges, "assembly": m.G.Assembly, "classes": m.classes, "chips": json.RawMessage(m.chips), "thresholds": m.th})
	})
	mux.HandleFunc("GET /api/shuchu/group", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		seat, g := m.mine(w, r)
		if g == nil {
			return
		}
		v := m.groupView(g)
		v["seat"] = seat
		if m.five != nil && m.five.Open {
			v["vote"] = map[string]any{"open": true, "group": m.five.Group, "mine": m.five.Votes[seat]}
		}
		core.JSON(w, v)
	})
	mux.HandleFunc("POST /api/shuchu/group/cards", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Card *int    `json:"card"`
			On   *bool   `json:"on"`
			Note *string `json:"note"`
			Req  *string `json:"req"`
		}
		if core.Body(r, &in) != nil {
			core.Err(w, 400, "格式不对", "bad_body")
			return
		}
		m.mu.Lock()
		seat, g := m.mine(w, r)
		if g == nil {
			m.mu.Unlock()
			return
		}
		what := ""
		if in.Card != nil && *in.Card >= 1 && *in.Card <= 6 {
			i := *in.Card - 1
			if in.On != nil {
				g.Input.Cards[i] = *in.On
				what = fmt.Sprintf("%s了%s", map[bool]string{true: "勾选", false: "取消"}[*in.On], "①②③④⑤⑥"[i*3:i*3+3])
			}
			if in.Note != nil {
				g.Input.Notes[i] = clip(*in.Note, 40)
				what = "补充了" + "①②③④⑤⑥"[i*3:i*3+3]
			}
		}
		if in.Req != nil {
			g.Input.Req = clip(*in.Req, 120)
			what = "改了对路线图的要求"
		}
		_ = what
		g.LastBy = seat
		m.pushGroup(g, seat, "")
		m.mu.Unlock()
		core.JSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/shuchu/group/ask", func(w http.ResponseWriter, r *http.Request) { m.ask(w, r, false) })
	mux.HandleFunc("POST /api/shuchu/group/retry", func(w http.ResponseWriter, r *http.Request) { m.ask(w, r, true) })
	mux.HandleFunc("POST /api/shuchu/group/image", m.image)
	mux.HandleFunc("GET /uploads/shuchu/{file}", func(w http.ResponseWriter, r *http.Request) {
		f := filepath.Base(r.PathValue("file"))
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		http.ServeFile(w, r, filepath.Join(m.up, f))
	})
	mux.HandleFunc("POST /api/shuchu/vote", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Choice string `json:"choice"`
		}
		core.Body(r, &in)
		seat := m.app.SeatOf(r)
		m.mu.Lock()
		if m.five == nil || !m.five.Open || seat == 0 || (in.Choice != "same" && in.Choice != "diff") {
			m.mu.Unlock()
			core.Err(w, 409, "现在不能投票", "closed")
			return
		}
		m.five.Votes[seat] = in.Choice
		tally := m.tally()
		m.mu.Unlock()
		m.hub.Publish("screen", "vote_update", tally)
		m.app.MarkDirty("shuchu")
		core.JSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/shuchu/screen/state", m.app.Teacher(m.screenState))
	mux.HandleFunc("POST /api/shuchu/screen/control", m.app.Teacher(m.control))
}

func (m *Module) tally() map[string]int {
	t := map[string]int{"same": 0, "diff": 0}
	if m.five != nil {
		for _, v := range m.five.Votes {
			t[v]++
		}
	}
	return t
}

// ---------------- 提问：规划 + 画图说明书 ----------------

func (m *Module) userMsg(c Class, in Input, gp *GroupPlan) string {
	var sb strings.Builder
	sb.WriteString("【同学给你的信息】\n")
	n := 0
	for i, on := range in.Cards {
		if !on {
			continue
		}
		n++
		fmt.Fprintf(&sb, "%s %s：%s", "①②③④⑤⑥"[i*3:i*3+3], m.cards[i].Title, m.cardText(c, i+1))
		if in.Notes[i] != "" {
			sb.WriteString("（补充：" + in.Notes[i] + "）")
		}
		sb.WriteString("\n")
		if i == 5 {
			sb.WriteString("学校规定：" + m.rules + "\n")
		}
	}
	if n == 0 {
		sb.WriteString("（没有给任何信息）\n")
	}
	sb.WriteString("【计算工具算出的候选路线】\n")
	for k, r := range gp.Cands {
		var names []string
		for _, id := range r.Path {
			names = append(names, id)
		}
		tag := ""
		if k == gp.Chosen {
			tag = "【推荐】"
		}
		fmt.Fprintf(&sb, "路线%d%s：%s｜约 %.0f 米｜步行约 %.0f 秒｜楼梯：%s", k+1, tag, strings.Join(names, "→"), r.DistM, r.WalkS, m.G.by[r.Stair].Name)
		if r.Direct {
			sb.WriteString("｜不知道道路，直线连接")
		}
		sb.WriteString("\n")
	}
	if gp.Info.Crowd {
		fmt.Fprintf(&sb, "算上楼梯排队，推荐路线总用时约 %.0f 秒，楼梯拥堵：%s\n", gp.TotalS, map[string]string{"green": "不挤", "yellow": "有点挤", "red": "很挤"}[gp.Color])
	}
	req := strings.TrimSpace(in.Req)
	if req == "" {
		req = "（没有写要求）"
	}
	sb.WriteString("【对路线图的要求】" + req + "\n")
	fmt.Fprintf(&sb, "请输出画图说明书 JSON。起点是 %s（%s班教室）。", gp.Cands[gp.Chosen].Path[0], c.Class)
	return sb.String()
}

func (m *Module) ask(w http.ResponseWriter, r *http.Request, retry bool) {
	m.mu.Lock()
	seat, g := m.mine(w, r)
	if g == nil {
		m.mu.Unlock()
		return
	}
	if g.Busy != 0 {
		m.mu.Unlock()
		core.Err(w, 409, "元小智正在画上一张图，稍等一下", "busy")
		return
	}
	if len(g.Versions) >= maxVersions {
		m.mu.Unlock()
		core.Err(w, 409, fmt.Sprintf("已经画了 %d 版啦，去看看大屏吧", maxVersions), "max_versions")
		return
	}
	in := g.Input
	if m.phase < 2 { // 任务一只给信息，要求留到任务二
		in.Req = ""
	}
	kind := "ask"
	if retry {
		if len(g.Versions) == 0 {
			m.mu.Unlock()
			core.Err(w, 409, "先请元小智画一版", "none")
			return
		}
		in = g.Versions[len(g.Versions)-1].Input
		kind = "retry"
	}
	c := m.class(g.N)
	st := core.NewStream(w)
	defer st.Close()
	userText := strings.TrimSpace(in.Req)
	if retry {
		userText = "再试一次（要求不变）"
	} else if userText == "" {
		userText = "请帮我们画出疏散路线图。"
	}
	g.Chat = append(g.Chat, ChatMsg{Role: "user", Seat: seat, Text: userText, Thumb: !retry})
	if !in.Cards[0] { // 没给①：元小智反问
		var miss []string
		for i, on := range in.Cards {
			if !on {
				miss = append(miss, m.cards[i].Title)
			}
		}
		reply := "我还不知道你们班在哪栋楼、几楼、有多少人，没法算路线呀。能告诉我：" + strings.Join(miss[:min(3, len(miss))], "、") + "吗？"
		g.Chat = append(g.Chat, ChatMsg{Role: "ai", Text: reply})
		m.pushGroup(g, seat, "")
		m.mu.Unlock()
		st.Send("reply", map[string]string{"text": reply})
		st.Send("done", map[string]any{})
		return
	}
	g.Busy = seat
	g.Chat = append(g.Chat, ChatMsg{Role: "ai", Text: "收到！我先算一算路线……", Pending: true})
	tr := m.runPlanLocked(g, in)
	gp := m.groupPlan(tr, g.N)
	m.pushGroup(g, seat, "")
	m.mu.Unlock()
	m.hub.Publish("screen", "plan_trace", tr)
	st.Send("plan", map[string]any{"plan": gp, "trace_ops": tr.Ops})

	sp, fb, ms := m.draw(c, in, gp, retry, 1)
	issues := m.G.Verify(c, sp.Path, gp, nil, hasSafety(sp))

	m.mu.Lock()
	v := &Version{N: len(g.Versions) + 1, Kind: kind, Seat: seat, Input: in, Plan: gp, Spec: sp, Issues: issues, Fallback: fb, At: time.Now()}
	g.Versions = append(g.Versions, v)
	g.Busy = 0
	for i := len(g.Chat) - 1; i >= 0; i-- {
		if g.Chat[i].Pending {
			g.Chat = append(g.Chat[:i], g.Chat[i+1:]...)
			break
		}
	}
	label := fmt.Sprintf("改好了！这是第 %d 版", v.N)
	if retry {
		label += " · 再试"
	}
	g.Chat = append(g.Chat, ChatMsg{Role: "ai", Text: label + "。" + sp.Explain, Version: v.N})
	m.pushGroup(g, seat, "")
	m.mu.Unlock()
	m.app.SaveNow("shuchu")
	st.Send("spec", map[string]any{"version": v.N, "spec": sp, "ms": ms})
	st.Send("done", map[string]any{})
}

// draw gets a spec: live model → replay → local rules. temp 1.0 for retries / 问 5 次.
func (m *Module) draw(c Class, in Input, gp *GroupPlan, vary bool, prio int) (*Spec, bool, int) {
	t0 := time.Now()
	var allowed [][]string
	for _, r := range gp.Cands {
		allowed = append(allowed, r.Path)
	}
	rec := gp.Cands[gp.Chosen].Path
	cfg := m.app.Cfg()
	mode := m.app.ModeName()
	feat := map[string]any{"group": c.Group, "cards": fmt.Sprint(in.Cards), "req": in.Req}
	rng := mrand.New(mrand.NewPCG(uint64(time.Now().UnixNano()), uint64(c.Group)))
	local := func() *Spec {
		sp := m.G.localSpec(in.Req, rec, in.Cards[5], rng, vary)
		m.G.normalize(sp, allowed, rec)
		return sp
	}
	if mode == "live" && cfg.UseLLM && m.gw.Configured() {
		effort := cfg.Effort[scene]
		temp := 0.7
		if vary {
			temp = 1.0
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		var out strings.Builder
		_, err := m.gw.Stream(ctx, llm.Req{Scene: scene, Messages: []llm.Msg{{Role: "user", Content: m.prefix + "\n\n===== 下面是这一组的信息 =====\n" + m.userMsg(c, in, gp)}},
			Effort: effort, Temperature: temp, MaxTokens: 1500, JSON: true, Priority: prio}, func(d llm.Delta) { out.WriteString(d.Content) })
		if sp, ok := parseSpec(out.String()); ok && err == nil {
			m.G.normalize(sp, allowed, rec)
			if sp.Explain == "" {
				sp.Explain = local().Explain
			}
			m.rp.Add(llm.Record{Scene: scene, Features: feat, Output: out.String()})
			return sp, false, int(time.Since(t0).Milliseconds())
		}
		m.gw.Stats(scene).Fallbacks.Add(1)
		m.app.Logger.Printf("shuchu group %d: fallback (err=%v)", c.Group, err)
		return local(), true, int(time.Since(t0).Milliseconds())
	}
	time.Sleep(time.Duration(1200+rng.IntN(1200)) * time.Millisecond)
	if mode == "replay" {
		recs, ok := m.rp.Best(scene, func(r llm.Record) float64 {
			s := 0.0
			if fmt.Sprint(r.Features["group"]) == fmt.Sprint(c.Group) {
				s++
			} else {
				return 0
			}
			if r.Features["cards"] == feat["cards"] {
				s += 2
			}
			if r.Features["req"] == in.Req {
				s += 3
			}
			return s + rng.Float64()*0.5
		})
		if ok {
			if sp, ok := parseSpec(recs.Output); ok {
				m.G.normalize(sp, allowed, rec)
				return sp, false, int(time.Since(t0).Milliseconds())
			}
		}
	}
	return local(), true, int(time.Since(t0).Milliseconds())
}

func (m *Module) image(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int    `json:"version"`
		Data    string `json:"data"`
		Five    int    `json:"five"` // >0: 问 5 次的第几张（大屏上传）
	}
	if core.Body(r, &in) != nil {
		core.Err(w, 400, "格式不对", "bad_body")
		return
	}
	const pre = "data:image/jpeg;base64,"
	if !strings.HasPrefix(in.Data, pre) {
		core.Err(w, 400, "只收 JPEG", "bad_image")
		return
	}
	b, err := base64.StdEncoding.DecodeString(in.Data[len(pre):])
	if err != nil || len(b) > 600<<10 {
		core.Err(w, 400, "图片太大了", "bad_image")
		return
	}
	m.mu.Lock()
	seat, g := m.mine(w, r)
	if g == nil {
		m.mu.Unlock()
		return
	}
	if in.Version < 1 || in.Version > len(g.Versions) {
		m.mu.Unlock()
		core.Err(w, 404, "没有这一版", "no_version")
		return
	}
	v := g.Versions[in.Version-1]
	if v.Image != "" { // 已经传过（重试上传）：保留第一张
		m.mu.Unlock()
		core.JSON(w, map[string]any{"image": v.Image})
		return
	}
	rb := make([]byte, 4)
	rand.Read(rb)
	name := fmt.Sprintf("g%d-s%d-v%d-%s.jpg", g.N, g.Seat, v.N, hex.EncodeToString(rb))
	if err := os.WriteFile(filepath.Join(m.up, name), b, 0o644); err != nil {
		m.mu.Unlock()
		core.Err(w, 500, "保存失败", "io")
		return
	}
	v.Image = "/uploads/shuchu/" + name
	m.pushGroup(g, seat, "")
	m.hub.Publish("screen", "gallery_update", m.groupSummary(g.N))
	m.mu.Unlock()
	core.JSON(w, map[string]any{"image": v.Image})
}

// ---------------- 大屏 ----------------

func (m *Module) screenState(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	if m.plan == nil {
		m.runPlanLocked(nil, Input{})
	}
	gs := []map[string]any{}
	for _, c := range m.classes {
		gs = append(gs, m.groupSummary(c.Group))
	}
	out := map[string]any{"groups": gs, "plan": m.plan, "five": m.five, "tally": m.tally(), "mode": m.app.ModeName(), "phase": m.phase}
	m.mu.Unlock()
	core.JSON(w, out)
}

func (m *Module) control(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action  string `json:"action"`
		Group   int    `json:"group"`
		Version int    `json:"version"`
		Phase   int    `json:"phase"`
		Seat    int    `json:"seat"` // 验收：哪台平板（编号位置会随新提交变化，以座位为准）
	}
	if core.Body(r, &in) != nil {
		core.Err(w, 400, "格式不对", "bad_body")
		return
	}
	switch in.Action {
	case "run_plan":
		m.mu.Lock()
		tr := m.runPlanLocked(nil, Input{})
		m.mu.Unlock()
		m.hub.Publish("screen", "plan_trace", tr)
		core.JSON(w, tr)
		return
	case "approve", "reject":
		m.mu.Lock()
		wl := m.wall(in.Group) // the screen's version number = position on the wall
		if in.Version < 1 || in.Version > len(wl) {
			m.mu.Unlock()
			core.Err(w, 404, "没有这一版", "no_version")
			return
		}
		g := wl[in.Version-1]
		if in.Seat > 0 {
			for k, x := range wl {
				if x.Seat == in.Seat {
					g, in.Version = x, k+1
				}
			}
		}
		v := g.Versions[len(g.Versions)-1]
		v.Verdict = map[string]string{"approve": "pass", "reject": "fail"}[in.Action]
		m.hub.Publish("screen", "verdict", map[string]any{"group": g.N, "version": in.Version, "verdict": v.Verdict})
		m.hub.Publish("student:"+strconv.Itoa(g.Seat), "verdict", map[string]any{"version": v.N, "verdict": v.Verdict})
		m.mu.Unlock()
	case "vote_open", "vote_close":
		m.mu.Lock()
		if in.Action == "vote_open" {
			m.five = &Five{Group: in.Group, Votes: map[int]string{}, Open: true}
		} else if m.five != nil {
			m.five.Open = false
		}
		f := m.five
		m.mu.Unlock()
		m.hub.Publish("student:all", "vote", map[string]any{"open": in.Action == "vote_open", "group": in.Group, "class": m.class(in.Group).Class})
		m.hub.Publish("screen", "vote_update", m.tally())
		_ = f
	case "five_start":
		m.mu.Lock()
		lv := m.latest(in.Group)
		if lv == nil {
			m.mu.Unlock()
			core.Err(w, 409, "这一组还没有路线图", "none")
			return
		}
		if m.five == nil || m.five.Group != in.Group {
			m.five = &Five{Group: in.Group, Votes: map[int]string{}}
		}
		if m.five.Running {
			m.mu.Unlock()
			core.Err(w, 409, "正在生成", "busy")
			return
		}
		m.five.Open, m.five.Running, m.five.Items = false, true, nil
		in0 := lv.Input
		tr := m.runPlanLocked(nil, Input{})
		gp := m.groupPlan(tr, in.Group)
		c := m.class(in.Group)
		m.mu.Unlock()
		m.hub.Publish("student:all", "vote", map[string]any{"open": false})
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sp, _, ms := m.draw(c, in0, gp, true, 0)
				it := &FiveItem{I: i, Spec: sp, Stair: stairOf(m.G, sp), Issues: m.G.Verify(c, sp.Path, gp, nil, hasSafety(sp)), Ms: ms}
				m.mu.Lock()
				m.five.Items = append(m.five.Items, it)
				m.mu.Unlock()
				m.hub.Publish("screen", "five_item", it)
			}(i)
		}
		go func() {
			wg.Wait()
			m.mu.Lock()
			m.five.Running = false
			m.mu.Unlock()
			m.hub.Publish("screen", "five_done", map[string]any{})
			m.app.SaveNow("shuchu")
		}()
	case "phase":
		if in.Phase != 1 && in.Phase != 2 {
			core.Err(w, 400, "只有任务一和任务二", "bad_phase")
			return
		}
		m.mu.Lock()
		m.phase = in.Phase
		m.mu.Unlock()
		m.hub.Publish("all", "phase", map[string]any{"phase": in.Phase})
	case "look":
		m.hub.Publish("student:all", "look", map[string]any{"on": in.Group == 1})
	case "reset":
		m.Reset()
		m.app.SaveNow("shuchu")
	}
	m.app.MarkDirty("shuchu")
	core.JSON(w, map[string]any{"ok": true})
}

func (m *Module) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, g := range m.ws {
		n += len(g.Versions)
	}
	return map[string]any{"versions": n, "tablets": len(m.ws)}
}
