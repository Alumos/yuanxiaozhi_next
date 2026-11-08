// Package llm is the shared model gateway: OpenAI-compatible streaming over HTTP/1.1,
// effort mapping, a priority queue with adaptive concurrency, and per-scene stats.
package llm

import (
	"bufio"
	"math/rand/v2"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Req struct {
	Scene       string
	Messages    []Msg
	Effort      string // off|low|medium|high
	Temperature float64
	MaxTokens   int
	JSON        bool
	Priority    int // 0 teacher demo, 1 first request, 2 follow-up
	CacheKey    string
}

// Delta is one streamed piece.
type Delta struct {
	Content   string
	Reasoning string
}

type Usage struct {
	Prompt     int `json:"prompt_tokens"`
	Completion int `json:"completion_tokens"`
	Cached     int `json:"cached"`
}

type Gateway struct {
	Base, Key, Model string
	EffortStyle      string
	client           *http.Client

	mu       sync.Mutex
	limit    int // current adaptive limit
	maxLimit int
	running  int
	waiters  [3][]chan struct{}
	backoff  time.Time
	lastCut  time.Time
	Log      func(format string, a ...any)

	stats sync.Map // scene -> *SceneStats
	ok    atomic.Int64
	fail  atomic.Int64
	lastErr atomic.Value
}

type SceneStats struct {
	Calls     atomic.Int64
	Fallbacks atomic.Int64
	Errors    atomic.Int64
	Prompt    atomic.Int64
	Cached    atomic.Int64
	Compl     atomic.Int64
	TotalMs   atomic.Int64
}

func New(base, key, model, effortStyle string, maxConc int) *Gateway {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         d.DialContext,
		ForceAttemptHTTP2:   false,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{}, // HTTP/1.1 only (gateway breaks on h2)
		TLSClientConfig:     &tls.Config{NextProtos: []string{"http/1.1"}, MinVersion: tls.VersionTLS12},
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  true, // SSE must not be buffered by gzip
	}
	if maxConc <= 0 {
		maxConc = 10
	}
	g := &Gateway{Base: strings.TrimRight(base, "/"), Key: key, Model: model, EffortStyle: effortStyle, client: &http.Client{Transport: tr}, limit: maxConc, maxLimit: maxConc}
	g.lastErr.Store("")
	go g.recover()
	return g
}

func (g *Gateway) Configured() bool { return g.Base != "" && g.Key != "" && g.Model != "" }

func (g *Gateway) SetMax(n int) {
	if n <= 0 {
		return
	}
	g.mu.Lock()
	g.maxLimit = n
	if g.limit > n {
		g.limit = n
	}
	g.mu.Unlock()
	g.wake()
}

// acquire waits for a slot respecting priority (lower first).
func (g *Gateway) acquire(ctx context.Context, prio int) error {
	if prio < 0 {
		prio = 0
	}
	if prio > 2 {
		prio = 2
	}
	g.mu.Lock()
	if g.running < g.limit && g.queued() == 0 {
		g.running++
		g.mu.Unlock()
		return nil
	}
	ch := make(chan struct{}, 1)
	g.waiters[prio] = append(g.waiters[prio], ch)
	g.mu.Unlock()
	g.wake()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		g.mu.Lock()
		removed := false
		for i, c := range g.waiters[prio] {
			if c == ch {
				g.waiters[prio] = append(g.waiters[prio][:i], g.waiters[prio][i+1:]...)
				removed = true
				break
			}
		}
		g.mu.Unlock()
		if !removed { // got the slot concurrently: give it back
			g.release()
		}
		return ctx.Err()
	}
}

func (g *Gateway) queued() int { return len(g.waiters[0]) + len(g.waiters[1]) + len(g.waiters[2]) }

func (g *Gateway) release() {
	g.mu.Lock()
	g.running--
	g.mu.Unlock()
	g.wake()
}

func (g *Gateway) wake() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.running < g.limit {
		var ch chan struct{}
		for p := 0; p < 3; p++ {
			if len(g.waiters[p]) > 0 {
				ch = g.waiters[p][0]
				g.waiters[p] = g.waiters[p][1:]
				break
			}
		}
		if ch == nil {
			return
		}
		g.running++
		ch <- struct{}{}
	}
}

// throttle halves concurrency on 429/5xx; recover() restores it step by step after 30 s.
// throttle: a burst of failures counts once (at most one cut per 10 s), and the limit never drops
// below a classroom-safe floor, so one bad moment cannot collapse 25 tablets into a single queue.
func (g *Gateway) throttle() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.backoff = time.Now().Add(20 * time.Second)
	if time.Since(g.lastCut) < 10*time.Second {
		return
	}
	g.lastCut = time.Now()
	floor := min(8, g.maxLimit)
	g.limit = max(floor, (g.limit+1)/2)
}

func (g *Gateway) recover() {
	for range time.Tick(3 * time.Second) {
		g.mu.Lock()
		if g.limit < g.maxLimit && time.Now().After(g.backoff) {
			g.limit = min(g.maxLimit, g.limit+2)
		}
		g.mu.Unlock()
		g.wake()
	}
}

func (g *Gateway) Stats(scene string) *SceneStats {
	v, _ := g.stats.LoadOrStore(scene, &SceneStats{})
	return v.(*SceneStats)
}

func (g *Gateway) Status() map[string]any {
	scenes := map[string]any{}
	g.stats.Range(func(k, v any) bool {
		s := v.(*SceneStats)
		calls := s.Calls.Load()
		hit := 0.0
		if p := s.Prompt.Load(); p > 0 {
			hit = float64(s.Cached.Load()) / float64(p)
		}
		avg := int64(0)
		if calls > 0 {
			avg = s.TotalMs.Load() / calls
		}
		scenes[k.(string)] = map[string]any{"calls": calls, "fallbacks": s.Fallbacks.Load(), "errors": s.Errors.Load(), "cache_hit": hit, "avg_ms": avg, "prompt_tokens": s.Prompt.Load(), "completion_tokens": s.Compl.Load()}
		return true
	})
	g.mu.Lock()
	defer g.mu.Unlock()
	return map[string]any{"configured": g.Configured(), "model": g.Model, "limit": g.limit, "max": g.maxLimit, "running": g.running, "queued": g.queued(), "ok": g.ok.Load(), "fail": g.fail.Load(), "last_error": g.lastErr.Load(), "scenes": scenes}
}

var ErrStatus = errors.New("gateway status")

// Stream calls the model and invokes onDelta for every piece. Retries once on 429/5xx before any data.
func (g *Gateway) Stream(ctx context.Context, r Req, onDelta func(Delta)) (Usage, error) {
	if !g.Configured() {
		return Usage{}, errors.New("model not configured")
	}
	st := g.Stats(r.Scene)
	st.Calls.Add(1)
	t0 := time.Now()
	if err := g.acquire(ctx, r.Priority); err != nil {
		return Usage{}, err
	}
	defer g.release()
	var u Usage
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		var got bool
		u, got, err = g.once(ctx, r, onDelta)
		if err != nil && g.Log != nil && ctx.Err() == nil {
			g.Log("llm %s attempt %d: %v", r.Scene, attempt+1, err)
		}
		if err == nil || got || ctx.Err() != nil || !errors.Is(err, ErrStatus) {
			break
		}
		g.throttle()
		select {
		case <-time.After(time.Duration(600+rand.IntN(900)) * time.Millisecond):
		case <-ctx.Done():
		}
	}
	st.TotalMs.Add(time.Since(t0).Milliseconds())
	st.Prompt.Add(int64(u.Prompt))
	st.Cached.Add(int64(u.Cached))
	st.Compl.Add(int64(u.Completion))
	if err != nil {
		st.Errors.Add(1)
		g.fail.Add(1)
		g.lastErr.Store(time.Now().Format("15:04:05") + " " + err.Error())
	} else {
		g.ok.Add(1)
	}
	return u, err
}

func (g *Gateway) body(r Req) map[string]any {
	b := map[string]any{"model": g.Model, "stream": true, "stream_options": map[string]bool{"include_usage": true}, "messages": r.Messages}
	if r.Temperature > 0 {
		b["temperature"] = r.Temperature
	}
	if r.MaxTokens > 0 {
		b["max_tokens"] = r.MaxTokens
	}
	if r.JSON {
		b["response_format"] = map[string]string{"type": "json_object"}
	}
	if r.CacheKey != "" && g.EffortStyle == "openai" {
		b["prompt_cache_key"] = r.CacheKey
	}
	e := r.Effort
	switch g.EffortStyle {
	case "qwen":
		if e == "" || e == "off" {
			b["enable_thinking"] = false
		} else {
			b["enable_thinking"] = true
			b["thinking_budget"] = map[string]int{"low": 512, "medium": 1500, "high": 4000}[e]
		}
	case "deepseek":
		if e == "" || e == "off" {
			b["thinking"] = map[string]string{"type": "disabled"}
		} else {
			b["thinking"] = map[string]string{"type": "enabled"}
		}
	case "none":
	default: // openai style; measured on the classroom gateway: reasoning_effort works, thinking:disabled turns it off
		if e == "" || e == "off" {
			b["thinking"] = map[string]string{"type": "disabled"}
		} else {
			b["reasoning_effort"] = e
		}
	}
	return b
}

func (g *Gateway) once(ctx context.Context, r Req, onDelta func(Delta)) (Usage, bool, error) {
	var u Usage
	payload, _ := json.Marshal(g.body(r))
	hr, _ := http.NewRequestWithContext(ctx, "POST", g.Base+"/chat/completions", bytes.NewReader(payload))
	hr.Header.Set("Authorization", "Bearer "+g.Key)
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "text/event-stream")
	resp, err := g.client.Do(hr)
	if err != nil {
		return u, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		err := fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return u, false, fmt.Errorf("%w: %v", ErrStatus, err)
		}
		return u, false, err
	}
	got := false
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		d := bytes.TrimSpace(line[5:])
		if string(d) == "[DONE]" {
			break
		}
		var ch chunk
		if json.Unmarshal(d, &ch) != nil {
			continue
		}
		if ch.Error != nil && !got {
			return u, false, fmt.Errorf("%w: stream error: %s", ErrStatus, ch.Error.Message)
		}
		if ch.Usage != nil {
			u.Prompt = ch.Usage.Prompt
			u.Completion = ch.Usage.Completion
			u.Cached = ch.Usage.CacheHit
			if ch.Usage.Details != nil && ch.Usage.Details.Cached > u.Cached {
				u.Cached = ch.Usage.Details.Cached
			}
		}
		for _, c := range ch.Choices {
			rc := c.Delta.ReasoningContent
			if rc == "" {
				rc = c.Delta.Reasoning
			}
			if rc != "" || c.Delta.Content != "" {
				got = true
				onDelta(Delta{Content: c.Delta.Content, Reasoning: rc})
			}
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return u, got, err
	}
	if !got && ctx.Err() == nil { // 网关偶尔返回 200 却一个字也没有：当作可重试的错误
		return u, false, fmt.Errorf("%w: empty response", ErrStatus)
	}
	return u, got, ctx.Err()
}

type chunk struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
		CacheHit   int `json:"prompt_cache_hit_tokens"`
		Details    *struct {
			Cached int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// Lines wraps onDelta: calls onLine for each complete content line and onDraft for reasoning text.
func Lines(onLine func(string), onDraft func(string)) (func(Delta), func()) {
	var buf strings.Builder
	return func(d Delta) {
			if d.Reasoning != "" && onDraft != nil {
				onDraft(d.Reasoning)
			}
			if d.Content == "" {
				return
			}
			buf.WriteString(d.Content)
			s := buf.String()
			for {
				i := strings.IndexByte(s, '\n')
				if i < 0 {
					break
				}
				if l := strings.TrimSpace(s[:i]); l != "" {
					onLine(l)
				}
				s = s[i+1:]
			}
			buf.Reset()
			buf.WriteString(s)
		}, func() {
			if l := strings.TrimSpace(buf.String()); l != "" {
				onLine(l)
			}
			buf.Reset()
		}
}
