package jisuan

import (
	"time"
)

type LLMTrace struct {
	Thoughts []Thought `json:"thoughts"`
	Draft    string    `json:"draft,omitempty"` // reasoning text: screen only
	Ms       int       `json:"ms"`
	Fallback bool      `json:"fallback"`
	Source   string    `json:"source"` // live | replay | mock | local
	Prompt   int       `json:"prompt_tokens,omitempty"`
	Compl    int       `json:"completion_tokens,omitempty"`
}

type Trace struct {
	Kind       string               `json:"kind"` // first | again
	Rounds     []Round              `json:"rounds"`
	PerRound   []RoundDelta         `json:"per_round,omitempty"`
	Profile    Profile              `json:"profile"`
	Scores     []float32            `json:"scores"`
	Funnel     []FunnelStep         `json:"funnel"`
	Candidates []Candidate          `json:"candidates"`
	Ops        int64                `json:"ops"`
	ComputeMs  float64              `json:"compute_ms"`
	Books      map[string]*BookView `json:"books"`
	Changes    []TagDelta           `json:"changes,omitempty"`
	Struck     []string             `json:"struck,omitempty"`
	Liked      []string             `json:"liked,omitempty"`
	Text       string               `json:"text,omitempty"`
	OldPicks   []string             `json:"old_picks,omitempty"`
	LLM        LLMTrace             `json:"llm"`
}

type AttemptRecord struct {
	Attempt    int     `json:"attempt"`
	Rounds     []Round `json:"rounds"`
	Trace      *Trace  `json:"trace,omitempty"`
	Result     *Result `json:"result,omitempty"`
	Again      *Result `json:"again,omitempty"`
	AgainTrace *Trace  `json:"again_trace,omitempty"`
	Stars      int     `json:"stars"`
}

type Session struct {
	Seat       int             `json:"seat"`
	Stage      string          `json:"stage"` // intro | picking | analyzing | result | again | done
	Attempt    int             `json:"attempt"`
	History    []AttemptRecord `json:"history"`
	Rounds     []Round         `json:"rounds"`
	ShownAt    time.Time       `json:"shown_at"`
	Trace      *Trace          `json:"trace,omitempty"`
	Result     *Result         `json:"result,omitempty"`
	Again      *Result         `json:"again,omitempty"`
	AgainTrace *Trace          `json:"again_trace,omitempty"`
	Stars      int             `json:"stars"`
	Prompt     string          `json:"prompt,omitempty"` // first user message (for 再想想)
	Raw        string          `json:"raw,omitempty"`    // first assistant output
	Avoid      []string        `json:"avoid,omitempty"`

	job      *Job
	againJob *Job
}

func (s *Session) picked() int {
	n := 0
	for _, r := range s.Rounds {
		if r.Chosen != "" {
			n++
		}
	}
	return n
}

func (s *Session) shownIDs() map[string]bool {
	m := map[string]bool{}
	for _, r := range s.Rounds {
		for _, x := range r.Shown {
			m[x.BookID] = true
		}
	}
	return m
}

func (s *Session) avoidSet() map[string]bool {
	m := map[string]bool{}
	for _, id := range s.Avoid {
		m[id] = true
	}
	return m
}

func (s *Session) current() AttemptRecord {
	return AttemptRecord{Attempt: s.Attempt, Rounds: s.Rounds, Trace: s.Trace, Result: s.Result, Again: s.Again, AgainTrace: s.AgainTrace, Stars: s.Stars}
}

// final is the latest result of the current attempt (再想想 wins).
func (s *Session) final() *Result {
	if s.Again != nil {
		return s.Again
	}
	return s.Result
}
