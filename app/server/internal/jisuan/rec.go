package jisuan

import (
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"sort"
	"strings"
)

// 推荐算法（《计算 02》第 2–4 节，原型 自检/rec.py 的 Go 版，逻辑一致）。

type Slot struct {
	BookID  string `json:"book_id"`
	Purpose string `json:"purpose"` // anchor | exploit | contrast | explore | wildcard
	Note    string `json:"note"`
	Pair    string `json:"pair,omitempty"`
}

type Round struct {
	N      int      `json:"n"`
	Shown  []Slot   `json:"shown"`
	Chosen string   `json:"chosen"`
	Ms     int      `json:"ms"`
	Opened []string `json:"opened"`
}

type TagDelta struct {
	Tag string  `json:"tag"`
	V   float64 `json:"v"`
	Why string  `json:"why"`
}

type RoundDelta struct {
	Round  int        `json:"round"`
	Chosen string     `json:"chosen"`
	Conf   float64    `json:"conf"`
	Ms     int        `json:"ms"`
	Deltas []TagDelta `json:"deltas"`
	Obs    []string   `json:"obs"`
	// Bars after this round has been "rewound" in (rounds 5..r), scaled by the final max: for the growing bars
	Bars map[string]float64 `json:"bars"`
}

type Profile struct {
	W      [32]float64 `json:"-"`
	P      [32]float64 `json:"-"`
	F      [6]float64  `json:"-"`
	Pf     [6]float64  `json:"-"`
	L      float64     `json:"l"`
	Top    []TagW      `json:"top"`
	Neg    []TagW      `json:"neg"`
	Form   string      `json:"form"`
	LenWord string     `json:"len_word"`
	All    []TagW      `json:"all"` // every non-zero tag, sorted
}

type TagW struct {
	Tag string  `json:"tag"`
	V   float64 `json:"v"`
}

func conf(ms int) float64 {
	switch {
	case ms < 4000:
		return 1.2
	case ms <= 12000:
		return 1.0
	}
	return 0.85
}

type Extra struct { // 再想想的附加信号
	Like, Dislike []string
	TagAdj        map[int]float64
	LenTo         *float64
}

func (L *Library) profile(rounds []Round, ex *Extra, ops *int64) (Profile, []RoundDelta) {
	var p Profile
	var per []RoundDelta
	lens := 0.0
	contrib := make([][32]float64, len(rounds))
	for ri, r := range rounds {
		c := L.ByID[r.Chosen]
		if c == nil {
			continue
		}
		k := conf(r.Ms)
		agg := map[string]*TagDelta{}
		add := func(t int, v float64, why string) {
			p.W[t] += v
			contrib[ri][t] += v
			*ops++
			key := L.Names[t] + "|" + why
			if d, ok := agg[key]; ok {
				d.V += v
			} else {
				agg[key] = &TagDelta{Tag: L.Names[t], V: v, Why: why}
			}
		}
		for _, t := range c.Topic {
			i := L.TagIdx[t]
			add(i, 1.0*k*L.IDF[i], "选中")
		}
		for _, t := range c.Tone {
			i := L.TagIdx[t]
			add(i, 0.8*k*L.IDF[i], "选中")
		}
		p.F[c.form] += k
		lens += c.ln
		opened := map[string]bool{}
		for _, o := range r.Opened {
			opened[o] = true
		}
		var obs []string
		for _, s := range r.Shown {
			if s.BookID == c.ID {
				continue
			}
			u := L.ByID[s.BookID]
			if u == nil {
				continue
			}
			pen, why := 0.25, "没选"
			if opened[u.ID] {
				pen, why = 0.12, "看了没选"
			}
			for t := u.tags &^ c.tags; t != 0; t &= t - 1 {
				i := bits.TrailingZeros32(t)
				add(i, -pen*L.IDF[i], why)
			}
			if u.form != c.form {
				p.F[u.form] -= 0.2
			}
		}
		for _, s := range r.Shown {
			if s.Pair == "" || (c.ID != s.BookID && c.ID != s.Pair) {
				continue
			}
			oid := s.Pair
			if c.ID == s.Pair {
				oid = s.BookID
			}
			o := L.ByID[oid]
			if o == nil {
				break
			}
			for t := c.tones &^ o.tones; t != 0; t &= t - 1 {
				add(bits.TrailingZeros32(t), 0.5, "对照题选中")
			}
			for t := o.tones &^ c.tones; t != 0; t &= t - 1 {
				add(bits.TrailingZeros32(t), -0.3, "对照题没选")
			}
			if c.form != o.form {
				p.F[c.form] += 0.5
				p.F[o.form] -= 0.3
			}
			diff := L.tagList(c.tones &^ o.tones)
			if len(diff) > 0 {
				obs = append(obs, fmt.Sprintf("对照题：你选了%s的那本", diff[0]))
			} else if c.form != o.form {
				obs = append(obs, fmt.Sprintf("对照题：你选了%s", c.Form))
			}
			break
		}
		sec := float64(r.Ms) / 1000
		switch {
		case r.Ms < 4000:
			obs = append([]string{fmt.Sprintf("%.1f 秒 · 很果断 ×1.2", sec)}, obs...)
		case r.Ms > 12000:
			obs = append([]string{fmt.Sprintf("%.1f 秒 · 犹豫了一会儿 ×0.85", sec)}, obs...)
		default:
			obs = append([]string{fmt.Sprintf("%.1f 秒选定", sec)}, obs...)
		}
		for _, o := range r.Opened {
			if o != c.ID {
				if b := L.ByID[o]; b != nil {
					obs = append(obs, fmt.Sprintf("看了《%s》的简介，但没选", b.Title))
					break
				}
			}
		}
		if len(obs) > 2 {
			obs = obs[:2]
		}
		ds := make([]TagDelta, 0, len(agg))
		for _, d := range agg {
			d.V = math.Round(d.V*100) / 100
			ds = append(ds, *d)
		}
		sort.Slice(ds, func(i, j int) bool { return math.Abs(ds[i].V) > math.Abs(ds[j].V) || (math.Abs(ds[i].V) == math.Abs(ds[j].V) && ds[i].Tag < ds[j].Tag) })
		per = append(per, RoundDelta{Round: r.N, Chosen: c.ID, Conf: k, Ms: r.Ms, Deltas: ds, Obs: obs})
	}
	if ex != nil {
		for _, id := range ex.Dislike {
			if b := L.ByID[id]; b != nil {
				for t := b.tags; t != 0; t &= t - 1 {
					p.W[bits.TrailingZeros32(t)] -= 0.6
				}
			}
		}
		for _, id := range ex.Like {
			if b := L.ByID[id]; b != nil {
				for t := b.tags; t != 0; t &= t - 1 {
					p.W[bits.TrailingZeros32(t)] += 0.4
				}
			}
		}
		for t, v := range ex.TagAdj {
			p.W[t] += v
		}
	}
	m := 1e-9
	for i := 0; i < L.NTags; i++ {
		m = math.Max(m, math.Abs(p.W[i]))
	}
	for i := 0; i < L.NTags; i++ {
		p.P[i] = p.W[i] / m
	}
	// rewind bars: cumulative from round 5 back to round 1, scaled by final max
	var cum [32]float64
	for k := len(per) - 1; k >= 0; k-- {
		for i := 0; i < L.NTags; i++ {
			cum[i] += contrib[k][i]
		}
		bars := map[string]float64{}
		for i := 0; i < L.NTags; i++ {
			if v := cum[i] / m; math.Abs(v) >= 0.01 {
				bars[L.Names[i]] = math.Round(v*100) / 100
			}
		}
		per[k].Bars = bars
	}
	mf := 1e-9
	for _, v := range p.F {
		mf = math.Max(mf, math.Abs(v))
	}
	for i, v := range p.F {
		p.Pf[i] = v / mf
	}
	if len(rounds) > 0 {
		p.L = lens / float64(len(rounds))
	}
	if ex != nil && ex.LenTo != nil {
		p.L = *ex.LenTo
	}
	L.describe(&p)
	return p, per
}

func (L *Library) describe(p *Profile) {
	p.All = p.All[:0]
	for i := 0; i < L.NTags; i++ {
		if math.Abs(p.P[i]) >= 0.005 {
			p.All = append(p.All, TagW{L.Names[i], math.Round(p.P[i]*100) / 100})
		}
	}
	sort.SliceStable(p.All, func(i, j int) bool { return p.All[i].V > p.All[j].V })
	p.Top, p.Neg = nil, nil
	for _, t := range p.All {
		if t.V > 0 && len(p.Top) < 5 {
			p.Top = append(p.Top, t)
		}
	}
	for i := len(p.All) - 1; i >= 0 && len(p.Neg) < 2; i-- {
		if p.All[i].V < 0 {
			p.Neg = append(p.Neg, p.All[i])
		}
	}
	best, bv := 0, -1e9
	for i, v := range p.Pf {
		if i < len(L.Tags.Forms) && v > bv {
			best, bv = i, v
		}
	}
	p.Form = L.Tags.Forms[best]
	switch {
	case p.L < -0.2:
		p.LenWord = "偏短"
	case p.L > 0.2:
		p.LenWord = "偏长"
	default:
		p.LenWord = "适中"
	}
}

// score: dense 32-dimension dot product (counted in ops) + form, length, quality.
func (L *Library) score(b *Book, p *Profile, ops *int64) float64 {
	dot := 0.0
	for i := 0; i < L.NTags; i++ {
		x := 0.0
		if b.topics&(1<<i) != 0 {
			x = 1
		} else if b.tones&(1<<i) != 0 {
			x = 0.7
		}
		dot += x * p.P[i]
	}
	*ops += int64(L.NTags + 3)
	n := float64(len(b.Topic) + len(b.Tone))
	return dot/math.Sqrt(n) + 0.35*p.Pf[b.form] - 0.15*math.Abs(b.ln-p.L) + 0.03*b.Q
}

type FunnelStep struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

type Candidate struct {
	ID       string   `json:"id"`
	Score    float64  `json:"score"`
	Interest int      `json:"interest"`
	Hit      []string `json:"hit"` // 命中的喜欢标签
}

type CandResult struct {
	Cands  []Candidate
	Scores []float32 // library order (all books)
	Funnel []FunnelStep
	Ops    int64
}

func (L *Library) candidates(p *Profile, exclude map[string]bool, k, pool int, ops int64) CandResult {
	res := CandResult{Scores: make([]float32, len(L.Books)), Ops: ops}
	type sc struct {
		b *Book
		s float64
	}
	all := make([]sc, 0, len(L.Books))
	positive := 0
	for _, b := range L.Books {
		s := L.score(b, p, &res.Ops)
		res.Scores[b.idx] = float32(math.Round(s*1000) / 1000)
		if exclude[b.ID] {
			continue
		}
		all = append(all, sc{b, s})
		if s > 0 {
			positive++
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].s > all[j].s })
	if pool > len(all) {
		pool = len(all)
	}
	top := all[:pool]
	var chosen []sc
	authors := map[string]int{}
	series := map[string]bool{}
	cats := map[int]int{}
	used := make([]bool, len(top))
	for len(chosen) < k {
		bi, bv := -1, -1e18
		for i, t := range top {
			if used[i] {
				continue
			}
			b := t.b
			if authors[b.Author] >= 1 && b.Author != "佚名" {
				continue
			}
			if b.Series != "" && series[b.Series] {
				continue
			}
			sim := 0.0
			for _, c := range chosen {
				sim = math.Max(sim, jaccard(b, c.b))
				res.Ops++
			}
			v := 0.7*t.s - 0.3*sim
			if len(chosen) >= k-3 && len(cats) < 3 && cats[b.cat] > 0 {
				v -= 1
			}
			if v > bv {
				bi, bv = i, v
			}
		}
		if bi < 0 {
			break
		}
		used[bi] = true
		b := top[bi].b
		chosen = append(chosen, top[bi])
		authors[b.Author]++
		cats[b.cat]++
		if b.Series != "" {
			series[b.Series] = true
		}
	}
	s1, s40 := 0.0, 0.0
	if len(top) > 0 {
		s1, s40 = top[0].s, top[len(top)-1].s
	}
	for _, c := range chosen {
		z := (c.s - s40) / math.Max(1e-9, s1-s40)
		var hit []string
		for t := c.b.tags; t != 0; t &= t - 1 {
			i := bits.TrailingZeros32(t)
			if p.P[i] > 0.15 {
				hit = append(hit, L.Names[i])
			}
		}
		res.Cands = append(res.Cands, Candidate{ID: c.b.ID, Score: math.Round(c.s*1000) / 1000, Interest: int(math.Round(58 + 36*z)), Hit: hit})
	}
	res.Funnel = []FunnelStep{
		{len(L.Books), "书库里的全部书"},
		{len(L.Books) - len(exclude), fmt.Sprintf("去掉你已经看过的 %d 本", len(exclude))},
		{positive, "和你的口味有关的"},
		{len(top), "最接近你口味的"},
		{len(chosen), fmt.Sprintf("挑出 %d 本风格不一样的，交给元小智细读", len(chosen))},
		{3, "元小智推荐给你的"},
	}
	return res
}

// ---------------- 出题 ----------------

type gen struct {
	L      *Library
	rng    *rand.Rand
	shown  map[string]bool
	avoid  map[string]bool
	author map[string]int
	slots  []Slot
}

func (g *gen) eligible() []*Book {
	out := make([]*Book, 0, len(g.L.Books))
	for _, b := range g.L.Books {
		if g.shown[b.ID] || g.avoid[b.ID] || b.g0 > 6 {
			continue
		}
		if g.author[b.Author] >= 2 && b.Author != "佚名" {
			continue
		}
		out = append(out, b)
	}
	return out
}

func (g *gen) add(b *Book, purpose, note, pair string) bool {
	if b == nil {
		return false
	}
	g.slots = append(g.slots, Slot{BookID: b.ID, Purpose: purpose, Note: note, Pair: pair})
	g.shown[b.ID] = true
	g.author[b.Author]++
	return true
}

func (g *gen) pick(c []*Book, key func(*Book) float64, top int) *Book {
	if len(c) == 0 {
		return nil
	}
	if key != nil {
		c = append([]*Book(nil), c...)
		sort.SliceStable(c, func(i, j int) bool { return key(c[i]) < key(c[j]) })
		if len(c) > top {
			c = c[:top]
		}
	}
	return c[g.rng.IntN(len(c))]
}

func filter(c []*Book, f func(*Book) bool) []*Book {
	out := c[:0:0]
	for _, b := range c {
		if f(b) {
			out = append(out, b)
		}
	}
	return out
}

// GenRound builds round n's 5 books from the previous rounds (deterministic for a seed).
func (L *Library) GenRound(n int, rounds []Round, seed uint64, avoid map[string]bool) []Slot {
	g := &gen{L: L, rng: rand.New(rand.NewPCG(seed, uint64(n)*0x9E3779B97F4A7C15)), shown: map[string]bool{}, avoid: avoid, author: map[string]int{}}
	seenCats := map[int]bool{}
	for _, r := range rounds {
		for _, s := range r.Shown {
			if b := L.ByID[s.BookID]; b != nil {
				g.shown[b.ID] = true
				g.author[b.Author]++
				seenCats[b.cat] = true
			}
		}
	}
	// keep the avoid list only while the pool stays large enough
	if len(g.eligible()) < 60 {
		g.avoid = nil
	}
	var p *Profile
	var sc func(*Book) float64
	var ops int64
	if len(rounds) > 0 {
		pp, _ := L.profile(rounds, nil, &ops)
		p = &pp
		sc = func(b *Book) float64 { return -L.score(b, p, &ops) }
	}
	anchorOr := func(c []*Book) []*Book {
		if a := filter(c, func(b *Book) bool { return b.Anchor }); len(a) > 0 {
			return a
		}
		return c
	}
	slotCats := func() map[int]bool {
		m := map[int]bool{}
		for _, s := range g.slots {
			m[L.ByID[s.BookID].cat] = true
		}
		return m
	}
	explore := func(k int) {
		for i := 0; i < k; i++ {
			sc := slotCats()
			var us []int
			for c := range L.cats {
				if !seenCats[c] && !sc[c] {
					us = append(us, c)
				}
			}
			cat := -1
			if len(us) > 0 {
				cat = us[g.rng.IntN(len(us))]
			}
			cand := filter(g.eligible(), func(b *Book) bool { return cat < 0 || b.cat == cat })
			name := "书"
			if cat >= 0 {
				name = L.cats[cat]
			}
			g.add(g.pick(anchorOr(cand), nil, 0), "explore", "探索：你还没见过"+name+"类的书", "")
		}
	}
	exploit := func(k int) {
		for i := 0; i < k; i++ {
			b := g.pick(g.eligible(), sc, 4)
			note := "试探：和你选过的书口味相近"
			if b != nil {
				if hit := L.topHit(b, p); hit != "" {
					note = "试探：你好像喜欢" + hit + "，这本也是"
				}
			}
			g.add(b, "exploit", note, "")
		}
	}
	contrastTone := func(base *Book) bool {
		cand := filter(g.eligible(), func(b *Book) bool { return b.topics&base.topics != 0 && b.tones&base.tones == 0 })
		b := g.pick(cand, sc, 4)
		if b == nil {
			return false
		}
		same := strings.Join(L.tagList(b.topics&base.topics), "、")
		return g.add(b, "contrast", fmt.Sprintf("对照题：都是%s，一本%s、一本%s", same, firstTone(base), firstTone(b)), base.ID)
	}
	if n == 1 {
		perm := g.rng.Perm(len(L.cats))[:5]
		for _, c := range perm {
			cand := filter(g.eligible(), func(b *Book) bool { return b.cat == c })
			g.add(g.pick(anchorOr(cand), nil, 0), "anchor", "摸底：先看看你对"+L.cats[c]+"有没有兴趣", "")
		}
		allNovel := true
		for _, s := range g.slots {
			if L.ByID[s.BookID].Form != "小说" {
				allNovel = false
			}
		}
		if allNovel && len(g.slots) > 0 {
			last := g.slots[len(g.slots)-1]
			g.slots = g.slots[:len(g.slots)-1]
			delete(g.shown, last.BookID)
			used := map[int]bool{}
			for _, c := range perm {
				used[c] = true
			}
			g.add(g.pick(filter(g.eligible(), func(b *Book) bool { return b.Anchor && b.Form != "小说" && !used[b.cat] }), nil, 0), "anchor", "摸底：换一种体裁看看", "")
		}
	} else {
		last := L.ByID[rounds[len(rounds)-1].Chosen]
		switch n {
		case 2:
			c := filter(g.eligible(), func(b *Book) bool { return bits.OnesCount32(b.tags&last.tags) >= 2 })
			g.add(g.pick(c, sc, 4), "exploit", fmt.Sprintf("试探：上一轮你选了《%s》，这本口味相近", last.Title), "")
			c = filter(g.eligible(), func(b *Book) bool { return b.cat == last.cat && b.tones&last.tones == 0 })
			if b := g.pick(c, nil, 0); b != nil {
				g.add(b, "contrast", fmt.Sprintf("对照题：和《%s》同一类，风格不同", last.Title), "")
			}
			explore(5 - len(g.slots))
		case 3:
			exploit(2)
			if len(g.slots) > 0 {
				if !contrastTone(L.ByID[g.slots[0].BookID]) {
					explore(1)
				}
			}
			explore(5 - len(g.slots))
		case 4:
			exploit(2)
			tt := -1
			for i := 0; i < nTopic; i++ {
				if p.W[i] != 0 && (tt < 0 || p.P[i] > p.P[tt]) {
					tt = i
				}
			}
			if tt >= 0 && len(g.slots) > 0 {
				base := L.ByID[g.slots[0].BookID]
				c := filter(g.eligible(), func(b *Book) bool { return b.topics&(1<<tt) != 0 && b.form != base.form })
				if b := g.pick(c, sc, 4); b != nil {
					pair := ""
					if base.topics&(1<<tt) != 0 {
						pair = base.ID
					}
					g.add(b, "contrast", fmt.Sprintf("对照题：都是%s，体裁不同（%s / %s）", L.Names[tt], b.Form, base.Form), pair)
				}
			}
			explore(1)
			c := append([]*Book(nil), g.eligible()...)
			sort.SliceStable(c, func(i, j int) bool { return c[i].Q > c[j].Q })
			if len(c) > 60 {
				c = c[:60]
			}
			g.add(g.pick(c, nil, 0), "wildcard", "彩蛋：书库里口碑很好的一本", "")
		default:
			var tp []int
			for i := 0; i < nTopic; i++ {
				if p.W[i] != 0 {
					tp = append(tp, i)
				}
			}
			sort.SliceStable(tp, func(a, b int) bool { return p.P[tp[a]] > p.P[tp[b]] })
			if len(tp) >= 2 {
				h1, h2 := tp[0], tp[1]
				for _, hh := range [][2]int{{h1, h2}, {h2, h1}} {
					h, o := hh[0], hh[1]
					c := filter(g.eligible(), func(b *Book) bool { return b.topics&(1<<h) != 0 && b.topics&(1<<o) == 0 })
					for k := 0; k < 2; k++ {
						b := g.pick(c, sc, 5)
						if b == nil {
							break
						}
						g.add(b, "exploit", fmt.Sprintf("二选一：你更喜欢%s，还是%s？", L.Names[h], L.Names[o]), "")
						c = filter(c, func(x *Book) bool { return x.ID != b.ID && !g.shown[x.ID] })
					}
				}
			}
			long := p.L <= 0
			c := filter(g.eligible(), func(b *Book) bool { return (b.ln > 0) == long })
			w := "短"
			if long {
				w = "长"
			}
			g.add(g.pick(c, sc, 4), "contrast", "篇幅对照：一本"+w+"一点的书", "")
		}
	}
	for len(g.slots) < 5 {
		e := g.eligible()
		if len(e) == 0 {
			break
		}
		g.add(e[g.rng.IntN(len(e))], "explore", "探索：换个口味看看", "")
	}
	if len(g.slots) > 5 {
		g.slots = g.slots[:5]
	}
	g.rng.Shuffle(len(g.slots), func(i, j int) { g.slots[i], g.slots[j] = g.slots[j], g.slots[i] })
	return g.slots
}

func firstTone(b *Book) string {
	if len(b.Tone) > 0 {
		return b.Tone[0]
	}
	return b.Form
}

func (L *Library) topHit(b *Book, p *Profile) string {
	if p == nil {
		return ""
	}
	best, bv := -1, 0.2
	for t := b.topics; t != 0; t &= t - 1 {
		i := bits.TrailingZeros32(t)
		if p.P[i] > bv {
			best, bv = i, p.P[i]
		}
	}
	if best < 0 {
		return ""
	}
	return L.Names[best]
}
