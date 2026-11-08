package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Event is one pushed message. Audience: "screen", "student:all", "student:<seat>", "group:<n>", "all".
type Event struct {
	ID       uint64
	Audience string
	Name     string
	Data     []byte
}

type client struct {
	ch    chan *Event
	role  string // screen | student
	seat  int
	group int
}

func (c *client) wants(aud string) bool {
	switch aud {
	case "all":
		return true
	case "screen":
		return c.role == "screen"
	case "student:all":
		return c.role == "student"
	}
	if len(aud) > 8 && aud[:8] == "student:" {
		n, _ := strconv.Atoi(aud[8:])
		return c.role == "student" && c.seat == n
	}
	if len(aud) > 6 && aud[:6] == "group:" {
		n, _ := strconv.Atoi(aud[6:])
		return c.role == "student" && c.group == n
	}
	return false
}

// Hub fans out events of one module; keeps the last 500 for Last-Event-ID replay.
type Hub struct {
	mu      sync.RWMutex
	seq     uint64
	ring    []*Event
	clients map[*client]struct{}
}

func NewHub() *Hub { return &Hub{clients: map[*client]struct{}{}} }

func (h *Hub) Publish(aud, name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.seq++
	e := &Event{ID: h.seq, Audience: aud, Name: name, Data: b}
	h.ring = append(h.ring, e)
	if len(h.ring) > 500 {
		h.ring = h.ring[len(h.ring)-500:]
	}
	cs := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		if c.wants(aud) {
			cs = append(cs, c)
		}
	}
	h.mu.Unlock()
	for _, c := range cs {
		select {
		case c.ch <- e:
		default: // slow client: drop; it will resync from snapshot on reconnect
		}
	}
}

func (h *Hub) Count() (screens, students int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.role == "screen" {
			screens++
		} else {
			students++
		}
	}
	return
}

// Serve streams events to one client until it disconnects.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, role string, seat, group int) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("Connection", "keep-alive")
	hd.Set("X-Accel-Buffering", "no")
	c := &client{ch: make(chan *Event, 256), role: role, seat: seat, group: group}
	last, _ := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64)
	h.mu.Lock()
	var backlog []*Event
	if last > 0 {
		for _, e := range h.ring {
			if e.ID > last && c.wants(e.Audience) {
				backlog = append(backlog, e)
			}
		}
	}
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.clients, c); h.mu.Unlock() }()

	fmt.Fprintf(w, "retry: 2000\n: hi\n\n")
	for _, e := range backlog {
		writeEvent(w, e)
	}
	fl.Flush()
	tk := time.NewTicker(15 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-c.ch:
			writeEvent(w, e)
			// drain what is queued, then flush once
			for n := len(c.ch); n > 0; n-- {
				writeEvent(w, <-c.ch)
			}
			fl.Flush()
		case <-tk.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, e *Event) {
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Name, e.Data)
}

// Stream is a per-request SSE response (used for model output).
type Stream struct {
	w  http.ResponseWriter
	fl http.Flusher
	mu     sync.Mutex
	closed bool
	stop   chan struct{}
}

func NewStream(w http.ResponseWriter) *Stream {
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("X-Accel-Buffering", "no")
	fl, _ := w.(http.Flusher)
	s := &Stream{w: w, fl: fl, stop: make(chan struct{})}
	fmt.Fprint(w, ": open\n\n")
	if fl != nil {
		fl.Flush()
	}
	// 等模型的时候可能十几秒没有数据：定时发注释行，免得代理或旧 WebView 把连接当成断了
	go func() {
		t := time.NewTicker(8 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.Ping()
			case <-s.stop:
				return
			}
		}
	}()
	return s
}

func (s *Stream) Send(name string, v any) {
	b, _ := json.Marshal(v)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, b)
	if s.fl != nil {
		s.fl.Flush()
	}
}

// Ping writes an SSE comment so proxies and old WebViews don't drop an idle stream.
func (s *Stream) Ping() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	fmt.Fprint(s.w, ": ping\n\n")
	if s.fl != nil {
		s.fl.Flush()
	}
}

// Close must be called before the handler returns (callers defer it).
func (s *Stream) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.stop)
	}
	s.mu.Unlock()
}
