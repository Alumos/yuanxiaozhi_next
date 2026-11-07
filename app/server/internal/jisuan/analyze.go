package jisuan

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"yxz/internal/llm"
)

const scene = "jisuan.recommend"

// runModel produces thoughts / persona / picks for one analysis and pushes them as job events.
// Rules (《计算 01》头部): 25 s without a first thought → whole local; 35 s → fill the rest locally.
func (m *Module) runModel(seat int, j *Job, tr *Trace, msgs []llm.Msg, chk *Checker, localRaw string, banned map[string]bool, prio int) (*Result, string) {
	cfg := m.app.Cfg()
	mode := m.app.ModeName()
	t0 := time.Now()
	res := &Result{}
	var mu sync.Mutex
	var raw strings.Builder
	var draft strings.Builder
	gotThought := make(chan struct{})
	var once sync.Once
	stopped := false

	handle := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return
		}
		line = normPunct(line)
		raw.WriteString(line + "\n")
		parts := strings.Split(line, "|")
		switch strings.TrimSpace(parts[0]) {
		case "想":
			if t, ok := chk.Thought(parts); ok && len(tr.LLM.Thoughts) < 6 {
				t.Ms = int(time.Since(t0).Milliseconds())
				tr.LLM.Thoughts = append(tr.LLM.Thoughts, t)
				j.push("thought", map[string]any{"i": len(tr.LLM.Thoughts) - 1, "kind": t.Kind, "text": t.Text})
				once.Do(func() { close(gotThought) })
			}
		case "画像":
			if res.Persona == "" {
				if name, tags, ok := chk.Persona(parts); ok {
					res.Persona, res.TopTags = name, tags
					j.push("persona", map[string]any{"name": name, "tags": tags})
				}
			}
		case "荐":
			if len(res.Picks) < 3 {
				if p, ok := chk.Pick(parts); ok {
					res.Picks = append(res.Picks, p)
					j.push("pick", p)
				}
			}
		}
	}
	onDraft := func(s string) {
		mu.Lock()
		if !stopped && draft.Len() < 12000 {
			draft.WriteString(s)
		}
		mu.Unlock()
		m.hub.Publish("screen", "draft", map[string]any{"seat": seat, "text": s})
	}
	// simulated streaming for mock / replay / local-only
	simulate := func(text string, src string) {
		tr.LLM.Source = src
		rng := rand.New(rand.NewPCG(uint64(seat), uint64(t0.UnixNano())))
		scale := cfg.AnimScale
		if scale <= 0 {
			scale = 1
		}
		for _, l := range strings.Split(text, "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			time.Sleep(time.Duration(float64(300+rng.IntN(300))*scale) * time.Millisecond)
			handle(l)
		}
	}

	useLive := mode == "live" && cfg.UseLLM && m.gw.Configured()
	switch {
	case useLive:
		tr.LLM.Source = "live"
		effort := cfg.Effort[scene]
		if effort == "" {
			effort = "low"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		go func() {
			select {
			case <-gotThought:
			case <-time.After(25 * time.Second):
				mu.Lock()
				noThought := len(tr.LLM.Thoughts) == 0
				mu.Unlock()
				if noThought {
					cancel()
				}
			case <-ctx.Done():
			}
		}()
		onDelta, flush := llm.Lines(handle, onDraft)
		u, err := m.gw.Stream(ctx, llm.Req{Scene: scene, Messages: msgs, Effort: effort, Temperature: 0.7, MaxTokens: 6000, Priority: prio, CacheKey: scene + m.prefixHash}, onDelta)
		flush()
		cancel()
		tr.LLM.Prompt, tr.LLM.Compl = u.Prompt, u.Completion
		mu.Lock()
		failed := err != nil
		if len(tr.LLM.Thoughts) == 0 && len(res.Picks) == 0 {
			// nothing usable: whole local
			tr.LLM.Fallback = true
			stopped = false
		}
		mu.Unlock()
		if tr.LLM.Fallback {
			m.app.Logger.Printf("jisuan seat %d: model fallback (err=%v)", seat, err)
			simulateFast(handle, localRaw)
			tr.LLM.Source = "local"
		} else if failed || len(res.Picks) < 3 {
			tr.LLM.Fallback = true
		}
		if !tr.LLM.Fallback {
			m.rp.Add(llm.Record{Scene: scene, Features: map[string]any{"tags": topTags(tr), "cands": candIDs(tr)}, Output: raw.String(), Draft: draft.String()})
		}
	case mode == "replay":
		rec, ok := m.rp.Best(scene, func(r llm.Record) float64 { return replayScore(r, tr) })
		if ok {
			simulate(rec.Output, "replay")
			if rec.Draft != "" {
				m.hub.Publish("screen", "draft", map[string]any{"seat": seat, "text": clip(rec.Draft, 3000)})
				draft.WriteString(rec.Draft)
			}
		} else {
			tr.LLM.Fallback = true
			simulate(localRaw, "local")
		}
	default: // mock or model switched off
		tr.LLM.Fallback = mode == "mock" || !cfg.UseLLM
		simulate(localRaw, map[bool]string{true: "mock", false: "local"}[mode == "mock"])
	}

	mu.Lock()
	stopped = true
	// fill what is missing from the local result (same format)
	if len(tr.LLM.Thoughts) < 3 {
		have := map[string]bool{}
		for _, t := range tr.LLM.Thoughts {
			have[t.Text] = true
		}
		for _, t := range m.L.localThoughtsFrom(localRaw) {
			if len(tr.LLM.Thoughts) >= 4 {
				break
			}
			if !have[t.Text] {
				t.Ms = int(time.Since(t0).Milliseconds())
				tr.LLM.Thoughts = append(tr.LLM.Thoughts, t)
				j.push("thought", map[string]any{"i": len(tr.LLM.Thoughts) - 1, "kind": t.Kind, "text": t.Text})
			}
		}
		tr.LLM.Fallback = true
	}
	if res.Persona == "" {
		name, tags := m.L.persona(&tr.Profile)
		res.Persona, res.TopTags = name, tags
		j.push("persona", map[string]any{"name": name, "tags": tags})
	}
	if len(res.Picks) < 3 {
		skip := map[string]bool{}
		for k := range banned {
			skip[k] = true
		}
		for _, p := range res.Picks {
			skip[p.BookID] = true
		}
		for _, p := range m.L.localPicks(tr.Rounds, tr.Candidates, skip, 3-len(res.Picks)) {
			res.Picks = append(res.Picks, p)
			j.push("pick", p)
		}
	}
	orderPicks(res.Picks)
	res.Fallback = tr.LLM.Fallback
	tr.LLM.Ms = int(time.Since(t0).Milliseconds())
	tr.LLM.Draft = draft.String()
	out := raw.String()
	mu.Unlock()
	if out == "" {
		out = localRaw
	}
	return res, out
}

func simulateFast(handle func(string), text string) {
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			handle(l)
		}
	}
}

// localThoughtsFrom re-parses the "想" lines of a local output.
func (L *Library) localThoughtsFrom(raw string) []Thought {
	var out []Thought
	for _, l := range strings.Split(raw, "\n") {
		p := strings.Split(l, "|")
		if len(p) >= 3 && p[0] == "想" {
			out = append(out, Thought{Kind: p[1], Text: p[2], Local: true})
		}
	}
	return out
}

func topTags(tr *Trace) []string {
	var out []string
	for _, t := range tr.Profile.Top {
		if len(out) < 3 {
			out = append(out, t.Tag)
		}
	}
	return out
}

func candIDs(tr *Trace) []string {
	out := make([]string, len(tr.Candidates))
	for i, c := range tr.Candidates {
		out[i] = c.ID
	}
	return out
}

// replayScore: overlap of top tags and candidate sets; every replayed pick must be a current candidate.
func replayScore(r llm.Record, tr *Trace) float64 {
	cur := map[string]bool{}
	for _, c := range tr.Candidates {
		cur[c.ID] = true
	}
	for _, l := range strings.Split(r.Output, "\n") {
		p := strings.Split(l, "|")
		if len(p) >= 2 && p[0] == "荐" && !cur[strings.TrimSpace(p[1])] {
			return 0
		}
	}
	s := 0.0
	if v, ok := r.Features["cands"].([]any); ok {
		for _, x := range v {
			if id, _ := x.(string); cur[id] {
				s++
			}
		}
	}
	tt := map[string]bool{}
	for _, t := range topTags(tr) {
		tt[t] = true
	}
	if v, ok := r.Features["tags"].([]any); ok {
		for _, x := range v {
			if t, _ := x.(string); tt[t] {
				s += 2
			}
		}
	}
	return s
}
