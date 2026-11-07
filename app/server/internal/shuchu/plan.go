package shuchu

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Node struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"` // class floor stair road open assembly guess
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Floor    int     `json:"floor,omitempty"`
	Building string  `json:"building,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
}

type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Floor int    `json:"floor,omitempty"`
}

type Bld struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	D    float64 `json:"d"`
}

type Graph struct {
	MPerPx    float64             `json:"m_per_px"`
	FloorM    float64             `json:"floor_m"`
	Speed     float64             `json:"speed"`
	Assembly  string              `json:"assembly"`
	Open      string              `json:"open"`
	Nodes     []*Node             `json:"nodes"`
	Edges     []Edge              `json:"edges"`
	Buildings []Bld               `json:"buildings"`
	StairEnds map[string][]string `json:"stair_ends"`
	Corridor  struct {
		X, Y1, Y2 float64
	} `json:"corridor"`

	by  map[string]*Node
	adj map[string][]string
}

type Class struct {
	Group    int    `json:"group"`
	Class    string `json:"class"`
	Building string `json:"building"`
	Floor    int    `json:"floor"`
	Position string `json:"position"`
	People   int    `json:"people"`
	Start    string `json:"start"`
}

func loadJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(p), err)
	}
	return nil
}

func LoadGraph(path string) (*Graph, error) {
	var g Graph
	if err := loadJSON(path, &g); err != nil {
		return nil, err
	}
	g.by = map[string]*Node{}
	g.adj = map[string][]string{}
	for _, n := range g.Nodes {
		g.by[n.ID] = n
	}
	for _, e := range g.Edges {
		if g.by[e.From] == nil || g.by[e.To] == nil {
			return nil, fmt.Errorf("road_graph.json: 边 %s-%s 的节点不存在", e.From, e.To)
		}
		g.adj[e.From] = append(g.adj[e.From], e.To)
		g.adj[e.To] = append(g.adj[e.To], e.From)
	}
	// 猜测的楼梯：没给“楼梯位置”时，元小智以为楼梯在楼的两头
	for b := range g.StairEnds {
		minX, maxX, y := math.Inf(1), math.Inf(-1), 0.0
		for _, r := range g.Buildings {
			if strings.HasPrefix(r.Name, b) {
				minX, maxX = math.Min(minX, r.X), math.Max(maxX, r.X+r.W)
				y = r.Y + r.D/2
			}
		}
		for _, end := range []struct {
			s string
			x float64
		}{{"W", minX + 4}, {"E", maxX - 4}} {
			id := "G" + end.s + "_" + b
			n := &Node{ID: id, Name: b + map[string]string{"W": "西头", "E": "东头"}[end.s] + "（猜的楼梯）", Kind: "guess", X: end.x, Y: y, Building: b}
			g.Nodes = append(g.Nodes, n)
			g.by[id] = n
		}
	}
	return &g, nil
}

func (g *Graph) px(a, b string) float64 {
	p, q := g.by[a], g.by[b]
	return math.Hypot(p.X-q.X, p.Y-q.Y)
}

func (g *Graph) hasEdge(a, b string) bool {
	for _, x := range g.adj[a] {
		if x == b {
			return true
		}
	}
	return false
}

type pqItem struct {
	id string
	d  float64
}
type pq []pqItem

func (p pq) Len() int            { return len(p) }
func (p pq) Less(i, j int) bool  { return p[i].d < p[j].d }
func (p pq) Swap(i, j int)       { p[i], p[j] = p[j], p[i] }
func (p *pq) Push(x any)         { *p = append(*p, x.(pqItem)) }
func (p *pq) Pop() any           { o := *p; it := o[len(o)-1]; *p = o[:len(o)-1]; return it }

// shortest path; allow(n) filters nodes (e.g. ground only, or no other stairs).
func (g *Graph) shortest(from, to string, allow func(string) bool) ([]string, float64) {
	dist := map[string]float64{from: 0}
	prev := map[string]string{}
	h := &pq{{from, 0}}
	for h.Len() > 0 {
		it := heap.Pop(h).(pqItem)
		if it.d > dist[it.id] {
			continue
		}
		if it.id == to {
			break
		}
		for _, nb := range g.adj[it.id] {
			if nb != to && !allow(nb) {
				continue
			}
			nd := it.d + g.px(it.id, nb)
			if d, ok := dist[nb]; !ok || nd < d {
				dist[nb] = nd
				prev[nb] = it.id
				heap.Push(h, pqItem{nb, nd})
			}
		}
	}
	d, ok := dist[to]
	if !ok {
		return nil, 0
	}
	path := []string{to}
	for c := to; c != from; {
		c = prev[c]
		path = append([]string{c}, path...)
	}
	return path, d
}

// ---------------- 规划器 ----------------

type Cands struct {
	Stairs bool // ② 楼梯位置
	Roads  bool // ③ 道路和距离
	Crowd  bool // ④ 拥堵情况
	Goal   bool // ⑤ 集合点
}

type Route struct {
	Path   []string `json:"path"`
	Stair  string   `json:"stair"`
	DistM  float64  `json:"dist_m"`
	WalkS  float64  `json:"walk_s"`
	Guess  bool     `json:"guess,omitempty"`
	Direct bool     `json:"direct,omitempty"` // 直线连接（没给道路）
}

func (g *Graph) routeLen(path []string) float64 {
	s := 0.0
	for i := 1; i < len(path); i++ {
		s += g.px(path[i-1], path[i])
	}
	return s
}

// candidates for one class under the information it was given.
func (g *Graph) candidates(c Class, info Cands) []Route {
	goal := g.Assembly
	if !info.Goal {
		goal = g.Open
	}
	floorS := float64(c.Floor-1) * g.FloorM / g.Speed
	var stairs []string
	if info.Stairs {
		stairs = g.StairEnds[c.Building]
	} else {
		stairs = []string{"GW_" + c.Building, "GE_" + c.Building}
	}
	var out []Route
	for _, st := range stairs {
		var path []string
		ok := true
		if info.Stairs {
			// upstairs part: along the floor corridor only
			p, _ := g.shortest(c.Start, st, func(id string) bool { k := g.by[id].Kind; return k == "class" || k == "floor" })
			if p == nil {
				ok = false
			}
			path = p
		} else {
			path = []string{c.Start, st}
		}
		if !ok {
			continue
		}
		r := Route{Stair: st, Guess: !info.Stairs, Direct: !info.Roads}
		if info.Roads {
			from := st
			if !info.Stairs { // guessed stair: walk to the nearest real ground node first
				best, bd := "", math.Inf(1)
				for _, n := range g.Nodes {
					if n.Kind == "road" || n.Kind == "stair" {
						if d := g.px(st, n.ID); d < bd {
							best, bd = n.ID, d
						}
					}
				}
				path = append(path, best)
				from = best
			}
			p, _ := g.shortest(from, goal, func(id string) bool { k := g.by[id].Kind; return k == "road" || k == "open" || k == "assembly" })
			if p == nil {
				continue
			}
			path = append(path, p[1:]...)
		} else {
			path = append(path, goal)
		}
		r.Path = path
		r.DistM = math.Round(g.routeLen(path)*g.MPerPx + float64(c.Floor-1)*g.FloorM)
		r.WalkS = math.Round(g.routeLen(path)*g.MPerPx/g.Speed + floorS)
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].WalkS < out[j].WalkS })
	return out
}

type StairLoad struct {
	People int     `json:"people"`
	QueueS float64 `json:"queue_s"`
	Color  string  `json:"color"`
	Groups []int   `json:"groups"`
}

type PlanRound struct {
	Assign map[int]string        `json:"assign"` // group -> stair
	Times  map[int]float64       `json:"times"`  // walk + queue
	Loads  map[string]*StairLoad `json:"loads"`
	Moved  *Move                 `json:"moved,omitempty"`
}

type Move struct {
	Group  int     `json:"group"`
	From   string  `json:"from"`
	To     string  `json:"to"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Why    string  `json:"why"`
}

type GroupPlan struct {
	Group     int     `json:"group"`
	Class     string  `json:"class"`
	Known     bool    `json:"known"` // 给了①
	Info      Cands   `json:"info"`
	Cands     []Route `json:"cands"`
	Chosen    int     `json:"chosen"`
	TotalS    float64 `json:"total_s"`
	Color     string  `json:"color"`
}

type PlanTrace struct {
	Groups   []GroupPlan `json:"groups"`
	Rounds   []PlanRound `json:"rounds"`
	FirstRed int         `json:"first_red"` // round index, -1 none
	RedStair string      `json:"red_stair"`
	Ops      int         `json:"ops"`
}

type Thresholds struct {
	Green, Red float64
}

func color(q float64, t Thresholds) string {
	switch {
	case q >= t.Red:
		return "red"
	case q >= t.Green:
		return "yellow"
	}
	return "green"
}

// Plan runs the class-wide planner: everyone starts on the fastest walk; groups that gave ④
// take turns moving the slowest class to a faster stair (max 5 rounds) until nothing changes.
func (g *Graph) Plan(classes []Class, info map[int]Cands, known map[int]bool, th Thresholds) *PlanTrace {
	tr := &PlanTrace{FirstRed: -1}
	byGroup := map[int]*GroupPlan{}
	peopleOf := map[int]int{}
	for _, c := range classes {
		in := info[c.Group]
		if !known[c.Group] { // 没给①：真实世界里这个班照样要下楼，按真实情况算人数
			in = Cands{Stairs: true, Roads: true, Goal: true}
		}
		gp := GroupPlan{Group: c.Group, Class: c.Class, Known: known[c.Group], Info: info[c.Group], Cands: g.candidates(c, in)}
		tr.Ops += len(gp.Cands) * 40
		tr.Groups = append(tr.Groups, gp)
		peopleOf[c.Group] = c.People
	}
	for i := range tr.Groups {
		byGroup[tr.Groups[i].Group] = &tr.Groups[i]
	}
	realStair := func(gp *GroupPlan) string {
		if len(gp.Cands) == 0 {
			return ""
		}
		r := gp.Cands[gp.Chosen]
		if r.Guess { // 猜的楼梯：实际会走离它最近的真楼梯
			best, bd := "", math.Inf(1)
			for _, s := range g.StairEnds[g.by[r.Stair].Building] {
				if d := g.px(s, r.Stair); d < bd {
					best, bd = s, d
				}
			}
			return best
		}
		return r.Stair
	}
	eval := func() PlanRound {
		pr := PlanRound{Assign: map[int]string{}, Times: map[int]float64{}, Loads: map[string]*StairLoad{}}
		for _, gp := range tr.Groups {
			st := realStair(byGroup[gp.Group])
			if st == "" {
				continue
			}
			pr.Assign[gp.Group] = st
			l := pr.Loads[st]
			if l == nil {
				l = &StairLoad{}
				pr.Loads[st] = l
			}
			l.People += peopleOf[gp.Group]
			l.Groups = append(l.Groups, gp.Group)
		}
		for st, l := range pr.Loads {
			l.QueueS = math.Round(float64(l.People) / g.by[st].Rate)
			l.Color = color(l.QueueS, th)
			tr.Ops += len(l.Groups) * 3
		}
		for _, gp := range tr.Groups {
			if st, ok := pr.Assign[gp.Group]; ok {
				pr.Times[gp.Group] = byGroup[gp.Group].Cands[byGroup[gp.Group].Chosen].WalkS + pr.Loads[st].QueueS
			}
		}
		return pr
	}
	cur := eval()
	tr.Rounds = append(tr.Rounds, cur)
	mark := func(k int, pr PlanRound) {
		if tr.FirstRed >= 0 {
			return
		}
		for st, l := range pr.Loads {
			if l.Color == "red" {
				tr.FirstRed, tr.RedStair = k, st
				return
			}
		}
	}
	mark(0, cur)
	moved := map[int]bool{}
	for round := 1; round <= 5; round++ {
		// slowest group that gave ④ and has an alternative
		var order []int
		for _, gp := range tr.Groups {
			if gp.Known && gp.Info.Crowd && len(gp.Cands) > 1 {
				order = append(order, gp.Group)
			}
		}
		sort.SliceStable(order, func(i, j int) bool { return cur.Times[order[i]] > cur.Times[order[j]] })
		var mv *Move
		for _, gid := range order {
			gp := byGroup[gid]
			old := gp.Chosen
			best, bt := old, cur.Times[gid]
			for k := range gp.Cands {
				if k == old {
					continue
				}
				gp.Chosen = k
				t := eval()
				tr.Ops += 20
				if t.Times[gid] < bt-1 {
					best, bt = k, t.Times[gid]
				}
			}
			gp.Chosen = old
			if best != old {
				from, to := realStair(gp), ""
				gp.Chosen = best
				to = realStair(gp)
				mv = &Move{Group: gid, From: from, To: to, Before: cur.Times[gid], After: bt,
					Why: fmt.Sprintf("%s班换到%s：%.0f 秒 → %.0f 秒", gp.Class, g.by[to].Name, cur.Times[gid], bt)}
				moved[gid] = true
				break
			}
		}
		if mv == nil {
			break
		}
		cur = eval()
		cur.Moved = mv
		tr.Rounds = append(tr.Rounds, cur)
		mark(len(tr.Rounds)-1, cur)
	}
	for i := range tr.Groups {
		gp := &tr.Groups[i]
		gp.TotalS = cur.Times[gp.Group]
		if st, ok := cur.Assign[gp.Group]; ok {
			gp.Color = cur.Loads[st].Color
		}
	}
	return tr
}

// ---------------- 核验 ----------------

type Issue struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	Node  string `json:"node,omitempty"`
	Warn  bool   `json:"warn,omitempty"` // 提醒，不算不通过
}

func segRect(ax, ay, bx, by float64, r Bld) bool {
	// Liang–Barsky clip against a slightly shrunk rect (edges touching walls are fine)
	x0, y0, x1, y1 := r.X+4, r.Y+4, r.X+r.W-4, r.Y+r.D-4
	dx, dy := bx-ax, by-ay
	t0, t1 := 0.0, 1.0
	for _, pq := range [][2]float64{{-dx, ax - x0}, {dx, x1 - ax}, {-dy, ay - y0}, {dy, y1 - ay}} {
		p, q := pq[0], pq[1]
		if p == 0 {
			if q < 0 {
				return false
			}
			continue
		}
		t := q / p
		if p < 0 {
			t0 = math.Max(t0, t)
		} else {
			t1 = math.Min(t1, t)
		}
		if t0 > t1 {
			return false
		}
	}
	return t1-t0 > 0.02
}

func (g *Graph) crosses(a, b string) bool {
	p, q := g.by[a], g.by[b]
	for _, r := range g.Buildings {
		// segments starting/ending inside a building (classrooms, stairs) only count if they cross another one
		if inside(p, r) || inside(q, r) {
			continue
		}
		if segRect(p.X, p.Y, q.X, q.Y, r) {
			return true
		}
	}
	return false
}

func inside(n *Node, r Bld) bool { return n.X >= r.X-2 && n.X <= r.X+r.W+2 && n.Y >= r.Y-2 && n.Y <= r.Y+r.D+2 }

// Verify checks a drawn route (系统核验, revealed at the 发布会).
func (g *Graph) Verify(c Class, path []string, gp *GroupPlan, blocked map[string]bool, safety bool) []Issue {
	var out []Issue
	if len(path) < 2 {
		return []Issue{{Kind: "empty", Label: "没有画出路线"}}
	}
	if path[0] != c.Start {
		out = append(out, Issue{Kind: "start", Label: "起点不是本班教室", Node: path[0]})
	}
	if last := path[len(path)-1]; last != g.Assembly {
		out = append(out, Issue{Kind: "goal", Label: "终点不是集合点", Node: last})
	}
	crossed := false
	for i := 1; i < len(path); i++ {
		a, b := path[i-1], path[i]
		if g.hasEdge(a, b) {
			continue
		}
		if g.by[a].Kind == "guess" || g.by[b].Kind == "guess" {
			if g.by[a].Kind == "class" || g.by[b].Kind == "class" {
				continue // 楼上走到猜的楼梯：楼梯本身单独报
			}
		}
		if g.crosses(a, b) {
			out = append(out, Issue{Kind: "cross", Label: "穿过了楼房", From: a, To: b})
			crossed = true
		} else if !crossed {
			out = append(out, Issue{Kind: "offroad", Label: "没有沿道路走", From: a, To: b})
		}
	}
	for _, id := range path {
		n := g.by[id]
		if n.Kind == "guess" {
			real := false
			for _, s := range g.StairEnds[n.Building] {
				if g.px(s, id) < 16 {
					real = true
				}
			}
			if !real {
				out = append(out, Issue{Kind: "stair", Label: "这里没有楼梯", Node: id})
			}
		}
		if blocked[id] {
			out = append(out, Issue{Kind: "blocked", Label: "走了暂停使用的楼梯", Node: id})
		}
	}
	if gp != nil && gp.Color == "red" {
		for _, id := range path {
			if g.by[id].Kind == "stair" {
				out = append(out, Issue{Kind: "crowd", Label: "这部楼梯会很挤", Node: id})
				break
			}
		}
	}
	if gp != nil && len(gp.Cands) > 0 {
		best := gp.Cands[0].WalkS
		for _, c := range gp.Cands {
			best = math.Min(best, c.WalkS)
		}
		if l := g.routeLen(path)*g.MPerPx/g.Speed + float64(c.Floor-1)*g.FloorM/g.Speed; l > best*1.3 && l-best > 40 {
			out = append(out, Issue{Kind: "slow", Label: "绕远了", Node: path[len(path)/2], Warn: true})
		}
	}
	if !safety {
		out = append(out, Issue{Kind: "safety", Label: "缺少安全提醒", Node: path[len(path)-1], Warn: true})
	}
	return out
}

func failed(is []Issue) bool {
	for _, i := range is {
		if !i.Warn {
			return true
		}
	}
	return false
}
