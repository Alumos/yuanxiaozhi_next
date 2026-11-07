package jisuan

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"yxz/internal/core"
)

// Job is one running analysis. It outlives the HTTP request (a refresh re-attaches).
type Job struct {
	mu     sync.Mutex
	evs    []jobEv
	done   bool
	notify chan struct{}
}

type jobEv struct {
	name string
	data json.RawMessage
}

func newJob() *Job { return &Job{notify: make(chan struct{})} }

func (j *Job) push(name string, v any) {
	b, _ := json.Marshal(v)
	j.mu.Lock()
	j.evs = append(j.evs, jobEv{name, b})
	close(j.notify)
	j.notify = make(chan struct{})
	j.mu.Unlock()
}

func (j *Job) finish() {
	j.mu.Lock()
	j.done = true
	close(j.notify)
	j.notify = make(chan struct{})
	j.mu.Unlock()
}

// follow streams all events (from the start) to the request until the job is done.
func (j *Job) follow(ctx context.Context, w http.ResponseWriter) {
	s := core.NewStream(w)
	defer s.Close()
	i := 0
	for {
		j.mu.Lock()
		batch := append([]jobEv(nil), j.evs[i:]...)
		done := j.done
		ch := j.notify
		j.mu.Unlock()
		for _, e := range batch {
			s.Send(e.name, e.data)
		}
		i += len(batch)
		if done {
			return
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return
		}
	}
}
