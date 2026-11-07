package jisuan

import (
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
)

// Book is one library entry. Tags are a 32-bit set: bit 0..23 topics, 24..31 tones.
type Book struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	Origin   string   `json:"origin"`
	Category string   `json:"category"`
	Topic    []string `json:"topic"`
	Tone     []string `json:"tone"`
	Form     string   `json:"form"`
	Length   string   `json:"length"`
	Grade    string   `json:"grade"`
	Hook     string   `json:"hook"`
	Summary  string   `json:"summary"`
	Anchor   bool     `json:"anchor"`
	Series   string   `json:"series"`
	Q        float64  `json:"q"`

	idx    int
	tags   uint32
	topics uint32
	tones  uint32
	form   int
	ln     float64
	g0     int
	cat    int
}

type Category struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Motif string `json:"motif"`
	Icon  string `json:"icon"`
}

type Tags struct {
	Categories []Category `json:"categories"`
	Topics     []string   `json:"topics"`
	Tones      []string   `json:"tones"`
	Forms      []string   `json:"forms"`
}

type Library struct {
	Books  []*Book
	ByID   map[string]*Book
	ByTitle map[string]*Book
	Tags   Tags
	TagIdx map[string]int // topic i, tone 24+i
	Names  [32]string
	IDF    [32]float64
	NTags  int
	cats   []string
}

const nTopic = 24

func isTopic(i int) bool { return i < nTopic }

func LoadLibrary(dir string) (*Library, error) {
	var L Library
	b, err := os.ReadFile(filepath.Join(dir, "books.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &L.Books); err != nil {
		return nil, fmt.Errorf("books.json: %w", err)
	}
	tb, err := os.ReadFile(filepath.Join(dir, "tags.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(tb, &L.Tags); err != nil {
		return nil, fmt.Errorf("tags.json: %w", err)
	}
	if len(L.Tags.Topics) != nTopic || len(L.Tags.Tones) > 8 {
		return nil, fmt.Errorf("tags.json: 需要 24 个题材、不超过 8 种风格")
	}
	L.TagIdx = map[string]int{}
	for i, t := range L.Tags.Topics {
		L.TagIdx[t] = i
		L.Names[i] = t
	}
	for i, t := range L.Tags.Tones {
		L.TagIdx[t] = nTopic + i
		L.Names[nTopic+i] = t
	}
	L.NTags = nTopic + len(L.Tags.Tones)
	catIdx := map[string]int{}
	for i, c := range L.Tags.Categories {
		catIdx[c.Name] = i
		L.cats = append(L.cats, c.Name)
	}
	formIdx := map[string]int{}
	for i, f := range L.Tags.Forms {
		formIdx[f] = i
	}
	L.ByID = map[string]*Book{}
	L.ByTitle = map[string]*Book{}
	var df [32]int
	for i, bk := range L.Books {
		bk.idx = i
		for _, t := range bk.Topic {
			j, ok := L.TagIdx[t]
			if !ok || j >= nTopic {
				return nil, fmt.Errorf("books.json %s: 未知题材 %q", bk.ID, t)
			}
			bk.topics |= 1 << j
		}
		for _, t := range bk.Tone {
			j, ok := L.TagIdx[t]
			if !ok || j < nTopic {
				return nil, fmt.Errorf("books.json %s: 未知风格 %q", bk.ID, t)
			}
			bk.tones |= 1 << j
		}
		bk.tags = bk.topics | bk.tones
		f, ok := formIdx[bk.Form]
		if !ok {
			return nil, fmt.Errorf("books.json %s: 未知体裁 %q", bk.ID, bk.Form)
		}
		bk.form = f
		bk.ln = map[string]float64{"短": -1, "中": 0, "长": 1}[bk.Length]
		fmt.Sscanf(bk.Grade, "%d", &bk.g0)
		c, ok := catIdx[bk.Category]
		if !ok {
			return nil, fmt.Errorf("books.json %s: 未知大类 %q", bk.ID, bk.Category)
		}
		bk.cat = c
		if _, dup := L.ByID[bk.ID]; dup {
			return nil, fmt.Errorf("books.json: 编号重复 %s", bk.ID)
		}
		L.ByID[bk.ID] = bk
		L.ByTitle[bk.Title] = bk
		for t := bk.tags; t != 0; t &= t - 1 {
			df[bits.TrailingZeros32(t)]++
		}
	}
	// rarity weighting (测试报告: 按标签稀有度加权的画像), mean normalised to 1
	n := float64(len(L.Books))
	sum, cnt := 0.0, 0
	for i := 0; i < L.NTags; i++ {
		if df[i] > 0 {
			L.IDF[i] = math.Log(n / float64(df[i]))
			sum += L.IDF[i]
			cnt++
		}
	}
	for i := 0; i < L.NTags; i++ {
		if df[i] > 0 {
			L.IDF[i] /= sum / float64(cnt)
		}
	}
	return &L, nil
}

func (L *Library) tagList(m uint32) []string {
	out := make([]string, 0, bits.OnesCount32(m))
	for t := m; t != 0; t &= t - 1 {
		out = append(out, L.Names[bits.TrailingZeros32(t)])
	}
	return out
}

// tagsOrdered returns the book's tags in its own order (topics then tones).
func (b *Book) tagNames() []string { return append(append([]string{}, b.Topic...), b.Tone...) }

func jaccard(a, b *Book) float64 {
	u := bits.OnesCount32(a.tags | b.tags)
	if u == 0 {
		return 0
	}
	return float64(bits.OnesCount32(a.tags&b.tags)) / float64(u)
}

// BookView is what the tablet needs to draw a card / cover.
type BookView struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	Category string   `json:"category"`
	Cat      int      `json:"cat"`
	Tags     []string `json:"tags"` // 题材 1 + 风格 1 (card)
	AllTags  []string `json:"all_tags"`
	Form     string   `json:"form"`
	Length   string   `json:"length"`
	Hook     string   `json:"hook"`
	Summary  string   `json:"summary"`
}

func (L *Library) View(id string) *BookView {
	b := L.ByID[id]
	if b == nil {
		return nil
	}
	tags := []string{}
	if len(b.Topic) > 0 {
		tags = append(tags, b.Topic[0])
	}
	if len(b.Tone) > 0 {
		tags = append(tags, b.Tone[0])
	}
	return &BookView{ID: b.ID, Title: b.Title, Author: b.Author, Category: b.Category, Cat: b.cat, Tags: tags, AllTags: b.tagNames(), Form: b.Form, Length: b.Length, Hook: b.Hook, Summary: b.Summary}
}

func (L *Library) Views(ids []string) map[string]*BookView {
	m := make(map[string]*BookView, len(ids))
	for _, id := range ids {
		if v := L.View(id); v != nil {
			m[id] = v
		}
	}
	return m
}

func runeLen(s string) int { return len([]rune(s)) }

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func joinTags(L *Library, m uint32, sep string) string { return strings.Join(L.tagList(m), sep) }
