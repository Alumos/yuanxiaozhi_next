package shuchu

import (
	"encoding/json"
	"math/rand/v2"
	"regexp"
	"strings"
)

type Line struct {
	Color      string `json:"color"`
	Width      string `json:"width"`
	Style      string `json:"style"`
	Arrow      bool   `json:"arrow"`
	ArrowShape string `json:"arrow_shape"`
}

type Marker struct {
	Type  string `json:"type"`
	At    string `json:"at"`
	Text  string `json:"text"`
	Color string `json:"color"`
}

type Spec struct {
	Path     []string `json:"path"`
	Line     Line     `json:"line"`
	Markers  []Marker `json:"markers"`
	Numbers  bool     `json:"numbers"`
	Title    string   `json:"title"`
	Labels   string   `json:"labels"`
	Theme    string   `json:"theme"`
	Audience string   `json:"audience"`
	Explain  string   `json:"explain"`
}

var (
	colors   = []string{"默认", "绿色", "红色", "橙色", "蓝色", "紫色", "黄色", "粉色", "青色", "黑色", "棕色"}
	widths   = []string{"默认", "细", "中", "粗", "特粗"}
	styles   = []string{"实线", "虚线", "点线", "双线", "发光"}
	shapes   = []string{"三角", "燕尾", "脚印"}
	mtypes   = []string{"五角星", "红旗", "安全出口", "暂停使用", "文字", "圆圈", "警告", "小人", "灭火器", "对勾", "爱心"}
	themes   = []string{"默认", "简洁", "醒目", "卡通", "夜间", "黑白"}
	audience = []string{"未说明", "低年级", "高年级", "家长", "消防员", "老师"}
)

func oneOf(v string, opts []string, d string) string {
	v = strings.TrimSpace(v)
	for _, o := range opts {
		if v == o {
			return v
		}
	}
	return d
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}

// parseSpec reads the model's JSON loosely (it may wrap it in text or code fences).
func parseSpec(s string) (*Spec, bool) {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return nil, false
	}
	var raw map[string]any
	if json.Unmarshal([]byte(s[i:j+1]), &raw) != nil {
		return nil, false
	}
	b, _ := json.Marshal(raw)
	var sp Spec
	if json.Unmarshal(b, &sp) != nil {
		// tolerate "arrow": "三角" etc.
		if l, ok := raw["line"].(map[string]any); ok {
			if a, ok := l["arrow"].(string); ok {
				l["arrow"] = a != "" && a != "false" && a != "无"
				if sp.Line.ArrowShape == "" {
					l["arrow_shape"] = a
				}
			}
		}
		b, _ = json.Marshal(raw)
		if json.Unmarshal(b, &sp) != nil {
			return nil, false
		}
	}
	return &sp, true
}

// normalize clamps every field to what the renderer supports; path must be one of the allowed routes.
func (g *Graph) normalize(sp *Spec, allowed [][]string, rec []string) {
	ok := false
	for _, a := range allowed {
		if strings.Join(a, ",") == strings.Join(sp.Path, ",") {
			ok = true
		}
	}
	if !ok {
		sp.Path = rec
	}
	sp.Line.Color = oneOf(sp.Line.Color, colors, "默认")
	sp.Line.Width = oneOf(sp.Line.Width, widths, "默认")
	sp.Line.Style = oneOf(sp.Line.Style, styles, "实线")
	sp.Line.ArrowShape = oneOf(sp.Line.ArrowShape, shapes, "")
	if sp.Line.ArrowShape != "" {
		sp.Line.Arrow = true
	}
	in := map[string]bool{}
	for _, id := range sp.Path {
		in[id] = true
	}
	var ms []Marker
	for _, m := range sp.Markers {
		m.Type = oneOf(m.Type, mtypes, "")
		m.At = strings.ToUpper(strings.TrimSpace(m.At))
		if m.Type == "" || g.by[m.At] == nil || len(ms) >= 8 {
			continue
		}
		m.Text = clip(m.Text, 8)
		m.Color = oneOf(m.Color, colors, "")
		if m.Color == "默认" {
			m.Color = ""
		}
		ms = append(ms, m)
	}
	sp.Markers = ms
	sp.Title = clip(sp.Title, 12)
	sp.Labels = oneOf(sp.Labels, []string{"全部", "只留关键"}, "全部")
	sp.Theme = oneOf(sp.Theme, themes, "默认")
	sp.Audience = oneOf(sp.Audience, audience, "未说明")
	sp.Explain = clip(sp.Explain, 60)
}

var reTitle = regexp.MustCompile(`标题[写是叫为：:]*[“"「]([^”"」]{1,12})[”"」]`)

// localSpec turns the requirement text into a spec with simple keyword rules (兜底 / 模拟模式).
// vary>0 adds the randomness used for “再试一次”/“问 5 次” without a model.
func (g *Graph) localSpec(req string, path []string, safety bool, rng *rand.Rand, vary bool) *Spec {
	sp := &Spec{Path: path, Line: Line{Color: "默认", Width: "默认", Style: "实线"}, Labels: "全部", Theme: "默认", Audience: "未说明"}
	has := func(ws ...string) bool {
		for _, w := range ws {
			if strings.Contains(req, w) {
				return true
			}
		}
		return false
	}
	for _, c := range colors[1:] {
		if has(strings.TrimSuffix(c, "色")+"色", "用"+strings.TrimSuffix(c, "色")) {
			sp.Line.Color = c
			break
		}
	}
	switch {
	case has("特粗", "超粗", "很粗"):
		sp.Line.Width = "特粗"
	case has("粗"):
		sp.Line.Width = "粗"
	case has("细"):
		sp.Line.Width = "细"
	}
	switch {
	case has("虚线"):
		sp.Line.Style = "虚线"
	case has("点线"):
		sp.Line.Style = "点线"
	case has("双线"):
		sp.Line.Style = "双线"
	case has("发光", "荧光"):
		sp.Line.Style = "发光"
	}
	switch {
	case has("脚印", "小脚丫"):
		sp.Line.ArrowShape = "脚印"
	case has("一串", ">>>", "燕尾"):
		sp.Line.ArrowShape = "燕尾"
	case has("箭头", "方向"):
		sp.Line.ArrowShape = "三角"
	}
	if has("一年级", "低年级", "小朋友", "二年级") {
		sp.Audience = "低年级"
		if sp.Line.Width == "默认" {
			sp.Line.Width = "粗"
		}
		if sp.Line.ArrowShape == "" {
			sp.Line.ArrowShape = "三角"
		}
		sp.Numbers = true
		sp.Labels = "只留关键"
	} else if has("家长") {
		sp.Audience = "家长"
	} else if has("消防员") {
		sp.Audience = "消防员"
	}
	sp.Line.Arrow = sp.Line.ArrowShape != ""
	if has("序号", "步骤", "第一步", "1、2、3") {
		sp.Numbers = true
	}
	switch {
	case has("卡通", "可爱"):
		sp.Theme = "卡通"
	case has("醒目", "显眼"):
		sp.Theme = "醒目"
	case has("简洁", "干净"):
		sp.Theme = "简洁"
	case has("夜间", "晚上", "黑夜"):
		sp.Theme = "夜间"
	case has("黑白"):
		sp.Theme = "黑白"
	}
	start, goal := path[0], path[len(path)-1]
	stair := ""
	for _, id := range path {
		if k := g.by[id].Kind; k == "stair" || k == "guess" {
			stair = id
			break
		}
	}
	if has("你在这里", "我们在这") {
		sp.Markers = append(sp.Markers, Marker{Type: "小人", At: start, Text: "你在这里"})
	}
	if has("安全出口") && stair != "" {
		sp.Markers = append(sp.Markers, Marker{Type: "安全出口", At: stair})
	}
	if has("红旗", "集合点", "旗") {
		sp.Markers = append(sp.Markers, Marker{Type: "红旗", At: goal, Text: "集合点"})
	}
	if has("五角星", "星星") {
		sp.Markers = append(sp.Markers, Marker{Type: "五角星", At: goal})
	}
	if has("靠右") && len(path) > 2 {
		sp.Markers = append(sp.Markers, Marker{Type: "文字", At: path[len(path)/2], Text: "靠右慢行"})
	}
	if has("灭火器") && stair != "" {
		sp.Markers = append(sp.Markers, Marker{Type: "灭火器", At: stair})
	}
	if m := reTitle.FindStringSubmatch(req); m != nil {
		sp.Title = m[1]
	}
	if vary && rng != nil { // 模拟 AI 的随机性：没指定的地方每次不一样
		if sp.Line.Color == "默认" {
			sp.Line.Color = colors[1+rng.IntN(len(colors)-1)]
		}
		if !strings.Contains(req, "线") && rng.IntN(2) == 0 {
			sp.Line.Style = styles[rng.IntN(len(styles))]
		}
		if sp.Line.ArrowShape == "" && rng.IntN(3) > 0 {
			sp.Line.ArrowShape = shapes[rng.IntN(len(shapes))]
			sp.Line.Arrow = true
		}
		if len(sp.Markers) < 2 {
			sp.Markers = append(sp.Markers, Marker{Type: []string{"红旗", "五角星", "对勾", "爱心"}[rng.IntN(4)], At: goal, Text: "集合点"})
		}
	}
	names := []string{}
	if stair != "" {
		names = append(names, "从"+g.by[stair].Name+"下楼")
	}
	names = append(names, "到"+g.by[goal].Name)
	sp.Explain = strings.Join(names, "，") + "。"
	if safety {
		sp.Explain += "记得靠右慢行，不推不挤。"
		if !has("靠右") && len(sp.Markers) < 8 && len(path) > 2 {
			sp.Markers = append(sp.Markers, Marker{Type: "文字", At: path[len(path)/2], Text: "靠右慢行"})
		}
	}
	sp.Explain = clip(sp.Explain, 60)
	return sp
}

func hasSafety(sp *Spec) bool {
	kw := []string{"靠右", "不推", "不挤", "捂", "低姿", "电梯", "有序", "慢行", "不奔跑", "不返回"}
	txt := sp.Explain
	for _, m := range sp.Markers {
		txt += m.Text
		if m.Type == "警告" || m.Type == "灭火器" {
			return true
		}
	}
	for _, k := range kw {
		if strings.Contains(txt, k) {
			return true
		}
	}
	return false
}
