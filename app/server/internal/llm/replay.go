package llm

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is one live call kept for replay mode.
type Record struct {
	Scene    string            `json:"scene"`
	Features map[string]any    `json:"features"`
	Output   string            `json:"output"`
	Draft    string            `json:"draft,omitempty"`
	At       time.Time         `json:"at"`
}

// Replay stores live outputs per scene in <data>/replay/<scene>.jsonl.
type Replay struct {
	dir string
	mu  sync.Mutex
	mem map[string][]Record
}

func NewReplay(dir string) *Replay {
	r := &Replay{dir: dir, mem: map[string][]Record{}}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		for sc.Scan() {
			var rec Record
			if json.Unmarshal(sc.Bytes(), &rec) == nil {
				r.mem[rec.Scene] = append(r.mem[rec.Scene], rec)
			}
		}
		fh.Close()
	}
	return r
}

func (r *Replay) Add(rec Record) {
	rec.At = time.Now()
	b, _ := json.Marshal(rec)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mem[rec.Scene] = append(r.mem[rec.Scene], rec)
	f, err := os.OpenFile(filepath.Join(r.dir, rec.Scene+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		f.Write(append(b, '\n'))
		f.Close()
	}
}

// Best returns the record with the highest score (score<=0 means unusable).
func (r *Replay) Best(scene string, score func(Record) float64) (Record, bool) {
	r.mu.Lock()
	recs := r.mem[scene]
	r.mu.Unlock()
	var best Record
	bs := 0.0
	for _, rec := range recs {
		if s := score(rec); s > bs {
			bs, best = s, rec
		}
	}
	return best, bs > 0
}

func (r *Replay) Count() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for k, v := range r.mem {
		out[k] = len(v)
	}
	return out
}
