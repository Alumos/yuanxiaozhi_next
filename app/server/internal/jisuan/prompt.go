package jisuan

import (
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var purposeName = map[string]string{"anchor": "摸底", "exploit": "试探喜好", "contrast": "对照", "explore": "探索", "wildcard": "彩蛋"}

func secWord(ms int) string {
	s := fmt.Sprintf("%.1f 秒", float64(ms)/1000)
	if ms < 4000 {
		return s + "（很果断）"
	}
	if ms > 12000 {
		return s + "（犹豫了一会儿）"
	}
	return s
}

func (L *Library) bk(id string) string {
	b := L.ByID[id]
	return fmt.Sprintf("%s《%s》[%s;%s|%s|%s]", id, b.Title, strings.Join(b.Topic, ","), strings.Join(b.Tone, ","), b.Form, b.Length)
}

// UserMsg is the per-student part (《计算 02》6.2).
func (L *Library) UserMsg(rounds []Round, p *Profile, c []Candidate) string {
	var sb strings.Builder
	sb.WriteString("【5 轮选书记录】\n")
	for _, r := range rounds {
		items := make([]string, 0, 5)
		for _, s := range r.Shown {
			items = append(items, L.bk(s.BookID)+" 目的="+purposeName[s.Purpose])
		}
		var op []string
		for _, o := range r.Opened {
			if b := L.ByID[o]; b != nil {
				op = append(op, "《"+b.Title+"》")
			}
		}
		ops := "无"
		if len(op) > 0 {
			ops = strings.Join(op, "、")
		}
		fmt.Fprintf(&sb, "第%d轮：%s\n  选了 %s，用时 %s；看过简介：%s\n", r.N, strings.Join(items, " | "), L.bk(r.Chosen), secWord(r.Ms), ops)
	}
	var pos, neg []string
	for _, t := range p.All {
		if t.V > 0 && len(pos) < 6 {
			pos = append(pos, fmt.Sprintf("%s %.2f", t.Tag, t.V))
		}
	}
	for i := len(p.All) - 1; i >= 0 && len(neg) < 3; i-- {
		if p.All[i].V < 0 {
			neg = append(neg, fmt.Sprintf("%s %.2f", p.All[i].Tag, p.All[i].V))
		}
	}
	ns := "无"
	if len(neg) > 0 {
		ns = strings.Join(neg, "，")
	}
	fmt.Fprintf(&sb, "【阅读画像】喜欢：%s；不太感兴趣：%s\n", strings.Join(pos, "，"), ns)
	fmt.Fprintf(&sb, "喜欢的体裁：%s；篇幅：%s（%+.1f）\n", p.Form, p.LenWord, p.L)
	sb.WriteString("【候选书】\n")
	for _, x := range c {
		b := L.ByID[x.ID]
		fmt.Fprintf(&sb, "%s《%s》%s｜%s;%s｜%s｜%s｜参考感兴趣程度 %d｜简介：%s\n", b.ID, b.Title, b.Author, strings.Join(b.Topic, ","), strings.Join(b.Tone, ","), b.Form, b.Length, x.Interest, b.Summary)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ---------------- 本地兜底（《计算 02》第 10 节） ----------------

var adj = map[string]string{"搞笑": "快乐的", "紧张刺激": "勇敢的", "温暖感人": "温柔的", "烧脑": "爱动脑的", "想象力": "爱幻想的", "知识多": "好奇的", "励志": "不服输的", "安静优美": "安静的"}
var noun = map[string]string{"冒险": "冒险家", "动物": "动物朋友", "科学": "小科学家", "侦探推理": "小侦探", "太空宇宙": "星空旅人", "历史": "历史迷",
	"魔法奇幻": "魔法师", "童话寓言": "童话迷", "科幻": "未来探险家", "机器人科技": "小工程师", "自然植物": "自然观察家", "恐龙古生物": "恐龙迷",
	"海洋": "海洋探险家", "校园生活": "校园达人", "友谊": "好伙伴", "家庭亲情": "暖心读者", "成长": "追梦少年", "神话传说": "神话迷",
	"传统文化": "国风少年", "数学": "数学迷", "人体健康": "健康小卫士", "地理旅行": "小旅行家", "艺术体育": "小艺术家", "名人故事": "追星少年"}

func (L *Library) persona(p *Profile) (string, []string) {
	var tp, tn []string
	for _, t := range p.All {
		if t.V <= 0 {
			continue
		}
		if isTopic(L.TagIdx[t.Tag]) {
			tp = append(tp, t.Tag)
		} else {
			tn = append(tn, t.Tag)
		}
	}
	name := "爱读书的人"
	if len(tp) > 0 {
		name = noun[tp[0]]
		if name == "" {
			name = "读书人"
		}
		if len(tn) > 0 {
			if full := adj[tn[0]] + name; runeLen(full) <= 7 {
				name = full
			}
		}
	}
	tags := append([]string{}, tp[:min(2, len(tp))]...)
	if len(tn) > 0 {
		tags = append(tags, tn[0])
	}
	for _, t := range tp[min(2, len(tp)):] {
		if len(tags) >= 3 {
			break
		}
		tags = append(tags, t)
	}
	return name, tags
}

type Thought struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Ms   int    `json:"ms,omitempty"` // time since request (for replay pacing)
	Local bool  `json:"-"`
}

type Pick struct {
	BookID   string `json:"book_id"`
	Interest int    `json:"interest"`
	Reason   string `json:"reason"`
	Evidence []int  `json:"evidence"`
}

type Result struct {
	Persona  string   `json:"persona"`
	TopTags  []string `json:"top_tags"`
	Picks    []Pick   `json:"picks"`
	Fallback bool     `json:"-"`
	Note     string   `json:"note,omitempty"` // 再想想：改了什么
}

func (L *Library) localThoughts(rounds []Round, p *Profile, tags []string, seed uint64) []Thought {
	rng := rand.New(rand.NewPCG(seed, 77))
	pickS := func(a ...string) string { return a[rng.IntN(len(a))] }
	var out []Thought
	var t0 string
	for _, t := range p.All {
		if t.V > 0 && isTopic(L.TagIdx[t.Tag]) {
			t0 = t.Tag
			break
		}
	}
	var rs []int
	for _, r := range rounds {
		if b := L.ByID[r.Chosen]; b != nil && t0 != "" && b.topics&(1<<L.TagIdx[t0]) != 0 {
			rs = append(rs, r.N)
		}
	}
	if len(rs) >= 2 {
		out = append(out, Thought{Kind: "证据", Text: pickS(
			fmt.Sprintf("第%d轮和第%d轮，你都选了%s的书。", rs[0], rs[len(rs)-1], t0),
			fmt.Sprintf("%d 次选择里有 %d 次和%s有关，从第%d轮就开始了。", len(rounds), len(rs), t0, rs[0]))})
	}
	fast, slow := rounds[0], rounds[0]
	for _, r := range rounds {
		if r.Ms < fast.Ms {
			fast = r
		}
		if r.Ms > slow.Ms {
			slow = r
		}
	}
	ft := L.ByID[fast.Chosen].Title
	if fast.Ms < 4000 {
		out = append(out, Thought{Kind: "证据", Text: pickS(fmt.Sprintf("第%d轮你只用了%.0f秒就选了《%s》，很果断。", fast.N, math.Max(1, float64(fast.Ms)/1000), ft), fmt.Sprintf("第%d轮《%s》你几乎没犹豫，%.0f秒就定了。", fast.N, ft, math.Max(1, float64(fast.Ms)/1000)))})
	} else {
		out = append(out, Thought{Kind: "证据", Text: fmt.Sprintf("第%d轮你%.0f秒就选了《%s》，是你选得最快的一次。", fast.N, float64(fast.Ms)/1000, ft)})
	}
	if slow.Ms > 12000 {
		out = append(out, Thought{Kind: "推断", Text: pickS(fmt.Sprintf("第%d轮你犹豫了%.0f秒，几本书你都有点喜欢。", slow.N, float64(slow.Ms)/1000), fmt.Sprintf("第%d轮想了%.0f秒才选，说明那一组都挺合你胃口。", slow.N, float64(slow.Ms)/1000))})
	}
	for _, r := range rounds {
		for _, s := range r.Shown {
			if s.Purpose == "contrast" && s.Pair != "" && (r.Chosen == s.BookID || r.Chosen == s.Pair) {
				c := L.ByID[r.Chosen]
				if c != nil && len(out) < 4 {
					out = append(out, Thought{Kind: "推断", Text: clip(fmt.Sprintf("第%d轮是对照题，你选了%s的《%s》。", r.N, firstTone(c), c.Title), 40)})
				}
				goto doneContrast
			}
		}
	}
doneContrast:
	for i := len(p.All) - 1; i >= 0; i-- {
		t := p.All[i]
		if t.V >= 0 {
			break
		}
		ti := L.TagIdx[t.Tag]
		for _, r := range rounds {
			for _, s := range r.Shown {
				if b := L.ByID[s.BookID]; s.BookID != r.Chosen && b != nil && b.tags&(1<<ti) != 0 {
					out = append(out, Thought{Kind: "排除", Text: pickS(fmt.Sprintf("第%d轮有%s的书，你没有选它。", r.N, t.Tag), fmt.Sprintf("%s的书摆在你面前，你都没选，先放一放。", t.Tag))})
					goto doneNeg
				}
			}
		}
		break
	}
doneNeg:
	if p.LenWord != "适中" && len(out) < 5 {
		out = append(out, Thought{Kind: "推断", Text: fmt.Sprintf("你偏爱%s一点的书。", map[string]string{"偏短": "短", "偏长": "长"}[p.LenWord])})
	}
	out = append(out, Thought{Kind: "结论", Text: pickS("所以我在书库里重点找："+strings.Join(tags, "、")+"。", "所以我重点找："+strings.Join(tags, "、")+"的书。")})
	for i := range out {
		out[i].Text = clip(out[i].Text, 40)
		out[i].Local = true
	}
	return out
}

func (L *Library) localReason(rounds []Round, b *Book, used map[int]bool) (string, []int) {
	type rr struct {
		r          Round
		topic, all int
	}
	var ev []rr
	for _, r := range rounds {
		c := L.ByID[r.Chosen]
		if c == nil || c.tags&b.tags == 0 {
			continue
		}
		ev = append(ev, rr{r, bits.OnesCount32(c.topics & b.topics), bits.OnesCount32(c.tags & b.tags)})
	}
	sort.SliceStable(ev, func(i, j int) bool {
		if ev[i].topic != ev[j].topic {
			return ev[i].topic > ev[j].topic
		}
		if ev[i].all != ev[j].all {
			return ev[i].all > ev[j].all
		}
		return !used[ev[i].r.N] && used[ev[j].r.N]
	})
	if len(ev) > 2 {
		ev = ev[:2]
	}
	if len(ev) == 0 {
		return clip(b.Hook, 45), []int{1}
	}
	used[ev[0].r.N] = true
	c := L.ByID[ev[0].r.Chosen]
	common := L.tagList(c.topics & b.topics)
	common = append(common, L.tagList(c.tones&b.tones)...)
	if len(common) > 2 {
		common = common[:2]
	}
	reason := fmt.Sprintf("你第%d轮选了《%s》，这本也有%s，%s。", ev[0].r.N, c.Title, strings.Join(common, "、"), b.Hook)
	if runeLen(reason) > 45 {
		reason = fmt.Sprintf("你第%d轮选了《%s》，这本也是%s的书。", ev[0].r.N, c.Title, strings.Join(common, "、"))
	}
	if runeLen(reason) > 45 {
		reason = fmt.Sprintf("你第%d轮选的书也有%s。", ev[0].r.N, strings.Join(common, "、"))
	}
	var rs []int
	for _, e := range ev {
		rs = append(rs, e.r.N)
	}
	sort.Ints(rs)
	return reason, rs
}

func (L *Library) localPicks(rounds []Round, cands []Candidate, skip map[string]bool, n int) []Pick {
	used := map[int]bool{}
	cs := append([]Candidate(nil), cands...)
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Interest > cs[j].Interest })
	var out []Pick
	for _, c := range cs {
		if skip[c.ID] {
			continue
		}
		if len(out) >= n {
			break
		}
		r, ev := L.localReason(rounds, L.ByID[c.ID], used)
		out = append(out, Pick{BookID: c.ID, Interest: c.Interest, Reason: r, Evidence: ev})
	}
	return out
}

// LocalOutput renders the fallback in the same line protocol (also used to build the prompt example).
func (L *Library) LocalOutput(rounds []Round, p *Profile, cands []Candidate, seed uint64) string {
	name, tags := L.persona(p)
	var sb strings.Builder
	for _, t := range L.localThoughts(rounds, p, tags, seed) {
		fmt.Fprintf(&sb, "想|%s|%s\n", t.Kind, t.Text)
	}
	fmt.Fprintf(&sb, "画像|%s|%s\n", name, strings.Join(tags, ","))
	for _, pk := range L.localPicks(rounds, cands, nil, 3) {
		ev := make([]string, len(pk.Evidence))
		for i, e := range pk.Evidence {
			ev[i] = strconv.Itoa(e)
		}
		fmt.Fprintf(&sb, "荐|%s|%d|%s|%s\n", pk.BookID, pk.Interest, pk.Reason, strings.Join(ev, ","))
	}
	sb.WriteString("完")
	return sb.String()
}

// ---------------- 解析与校验（《计算 02》第 7 节） ----------------

var reTitle = regexp.MustCompile(`《([^》]{1,30})》`)
var reRound = regexp.MustCompile(`第\s*([0-9一二三四五])\s*轮`)
var cnNum = map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5}

func roundNum(s string) int {
	if n, ok := cnNum[s]; ok {
		return n
	}
	n, _ := strconv.Atoi(s)
	return n
}

var thoughtKinds = map[string]bool{"证据": true, "推断": true, "排除": true, "发现": true, "结论": true}

// Checker validates model lines against this student's real record.
type Checker struct {
	L        *Library
	Rounds   []Round
	Cands    map[string]Candidate
	shownIn  map[string]int // book id -> round
	titles   map[string]string // title -> id (shown + candidates)
	Picked   map[string]bool
	Banned   map[string]bool // already recommended / disliked (再想想)
}

func NewChecker(L *Library, rounds []Round, cands []Candidate, banned map[string]bool) *Checker {
	c := &Checker{L: L, Rounds: rounds, Cands: map[string]Candidate{}, shownIn: map[string]int{}, titles: map[string]string{}, Picked: map[string]bool{}, Banned: banned}
	for _, r := range rounds {
		for _, s := range r.Shown {
			c.shownIn[s.BookID] = r.N
			c.titles[L.ByID[s.BookID].Title] = s.BookID
		}
	}
	for _, x := range cands {
		c.Cands[x.ID] = x
		c.titles[L.ByID[x.ID].Title] = x.ID
	}
	return c
}

func normPunct(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "｜", "|")
	return s
}

func (c *Checker) textOK(s string, maxLen int) bool {
	if s == "" || runeLen(s) > maxLen {
		return false
	}
	for _, kw := range []string{"分数", "权重", "算法", "候选", "编号", "b0"} {
		if strings.Contains(s, kw) {
			return false
		}
	}
	rounds := reRound.FindAllStringSubmatch(s, -1)
	for _, m := range rounds {
		if n := roundNum(m[1]); n < 1 || n > len(c.Rounds) {
			return false
		}
	}
	for _, m := range reTitle.FindAllStringSubmatch(s, -1) {
		id, ok := c.titles[m[1]]
		if !ok {
			return false
		}
		// "第n轮…《X》": a shown book must really be in that round
		if rn, shown := c.shownIn[id]; shown && len(rounds) == 1 {
			if roundNum(rounds[0][1]) != rn {
				return false
			}
		}
	}
	return true
}

func (c *Checker) Thought(parts []string) (Thought, bool) {
	if len(parts) < 3 {
		return Thought{}, false
	}
	kind, text := strings.TrimSpace(parts[1]), strings.TrimSpace(strings.Join(parts[2:], "，"))
	if !thoughtKinds[kind] || kind == "发现" || !c.textOK(text, 44) {
		return Thought{}, false
	}
	return Thought{Kind: kind, Text: text}, true
}

func (c *Checker) Persona(parts []string) (string, []string, bool) {
	if len(parts) < 3 {
		return "", nil, false
	}
	name := strings.TrimSpace(parts[1])
	if name == "称号" && len(parts) >= 4 {
		name, parts = strings.TrimSpace(parts[2]), append(parts[:2], parts[3:]...)
	}
	name = strings.Trim(name, "“”\"'「」")
	if name == "" || runeLen(name) > 8 {
		return "", nil, false
	}
	var tags []string
	for _, t := range strings.FieldsFunc(parts[2], func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == ' ' }) {
		if _, ok := c.L.TagIdx[t]; ok && len(tags) < 3 {
			tags = append(tags, t)
		}
	}
	if len(tags) == 0 {
		return "", nil, false
	}
	return name, tags, true
}

func (c *Checker) Pick(parts []string) (Pick, bool) {
	if len(parts) < 4 {
		return Pick{}, false
	}
	id := strings.TrimSpace(parts[1])
	cand, ok := c.Cands[id]
	if !ok || c.Picked[id] || c.Banned[id] {
		return Pick{}, false
	}
	if _, shown := c.shownIn[id]; shown {
		return Pick{}, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(parts[2], "%")))
	if err != nil {
		return Pick{}, false
	}
	v = max(cand.Interest-8, min(cand.Interest+8, v))
	v = max(50, min(97, v))
	reason := strings.TrimSpace(parts[3])
	b := c.L.ByID[id]
	var ev []int
	if len(parts) >= 5 {
		for _, f := range strings.FieldsFunc(parts[4], func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == ' ' }) {
			n := roundNum(strings.TrimSuffix(strings.TrimPrefix(f, "第"), "轮"))
			if n < 1 || n > len(c.Rounds) {
				continue
			}
			ch := c.L.ByID[c.Rounds[n-1].Chosen]
			if ch != nil && ch.tags&b.tags != 0 {
				ev = append(ev, n)
			}
		}
	}
	if !c.textOK(reason, 50) || len(ev) == 0 {
		used := map[int]bool{}
		reason, ev = c.L.localReason(c.Rounds, b, used)
	}
	c.Picked[id] = true
	return Pick{BookID: id, Interest: v, Reason: reason, Evidence: ev}, true
}

// orderPicks sorts by interest and makes it strictly decreasing.
func orderPicks(p []Pick) {
	sort.SliceStable(p, func(i, j int) bool { return p[i].Interest > p[j].Interest })
	for i := 1; i < len(p); i++ {
		if p[i].Interest >= p[i-1].Interest {
			p[i].Interest = p[i-1].Interest - 1
		}
	}
}
