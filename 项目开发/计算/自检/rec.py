"""推荐算法原型（对应《02_推荐算法与提示词》第 2–5、10 节）。

只用 Python 标准库。正式开发时用 Go 重写，逻辑保持一致。
"""
import json, math, random
from collections import Counter, defaultdict
from pathlib import Path

LIB = Path(__file__).resolve().parent.parent / "书库"
BOOKS = json.loads((LIB / "books.json").read_text(encoding="utf-8"))
TAGS = json.loads((LIB / "tags.json").read_text(encoding="utf-8"))
BY_ID = {b["id"]: b for b in BOOKS}
CATS = [c["name"] for c in TAGS["categories"]]
LEN = {"短": -1, "中": 0, "长": 1}
for b in BOOKS:
    b["_tags"] = set(b["topic"]) | set(b["tone"])
    b["_len"] = LEN[b["length"]]
    b["_g0"] = int(b["grade"].split("-")[0])

TOPIC_W, TONE_W = 1.0, 0.8
PROFILE_MODE = "idf"   # heuristic | idf | logit
_DF = Counter(t for b in BOOKS for t in b["_tags"])
IDF = {t: math.log(len(BOOKS) / _DF[t]) for t in _DF}
_IDF_MEAN = sum(IDF.values()) / len(IDF)
IDF = {t: v / _IDF_MEAN for t, v in IDF.items()}   # 平均为 1：稀有标签 >1，常见标签 <1
FORMS = TAGS["forms"]


def features(b):
    x = {}
    for t in b["topic"]:
        x["t:" + t] = IDF[t]
    for t in b["tone"]:
        x["t:" + t] = 0.8 * IDF[t]
    x["f:" + b["form"]] = 1.0
    x["len"] = b["_len"]
    return x


def fit_logit(rounds, lam=0.6, iters=150, lr=0.25):
    """条件 logit：P(选 i | 本轮 5 本) = softmax(β·x_i)。用梯度上升 + L2 正则拟合 β。"""
    beta = defaultdict(float)
    data = [([features(BY_ID[s["id"]]) for s in r["shown"]], [s["id"] for s in r["shown"]].index(r["chosen"]), conf_of(r["ms"])) for r in rounds]
    for _ in range(iters):
        g = defaultdict(float)
        for xs, k, wgt in data:
            us = [sum(beta[f] * v for f, v in x.items()) for x in xs]
            m = max(us); es = [math.exp(u - m) for u in us]; z = sum(es)
            for i, x in enumerate(xs):
                coef = wgt * ((1 if i == k else 0) - es[i] / z)
                for f, v in x.items():
                    g[f] += coef * v
        for f in set(g) | set(beta):
            beta[f] += lr * (g[f] - lam * beta[f])
    return dict(beta)


def jaccard(a, b):
    return len(a["_tags"] & b["_tags"]) / max(1, len(a["_tags"] | b["_tags"]))


# ---------------- 偏好画像 ----------------
def conf_of(ms):
    return 1.2 if ms < 4000 else (1.0 if ms <= 12000 else 0.85)


def profile(rounds):
    """rounds: [{n, shown:[{id,purpose,pair}], chosen, ms, opened:[ids]}]
    返回 p（标签 -1..1）、pf（体裁 -1..1）、Lbar、per_round 变化明细、原始权重。"""
    w = defaultdict(float)
    f = defaultdict(float)
    lens = []
    per = []
    for r in rounds:
        c = BY_ID[r["chosen"]]
        k = conf_of(r["ms"])
        d = []
        idf = (lambda t: IDF[t]) if PROFILE_MODE != "heuristic" else (lambda t: 1.0)
        for t in c["topic"]:
            w[t] += TOPIC_W * k * idf(t); d.append((t, round(TOPIC_W * k * idf(t), 2), "选中"))
        for t in c["tone"]:
            w[t] += TONE_W * k * idf(t); d.append((t, round(TONE_W * k * idf(t), 2), "选中"))
        f[c["form"]] += k
        lens.append(c["_len"])
        for s in r["shown"]:
            if s["id"] == c["id"]:
                continue
            u = BY_ID[s["id"]]
            pen = 0.12 if s["id"] in r.get("opened", []) else 0.25
            for t in u["_tags"] - c["_tags"]:
                w[t] -= pen * idf(t); d.append((t, round(-pen * idf(t), 2), "看了没选" if pen < 0.2 else "没选"))
            if u["form"] != c["form"]:
                f[u["form"]] -= 0.2
        # 对照题
        for s in r["shown"]:
            if s.get("pair") and c["id"] in (s["id"], s["pair"]):
                other = BY_ID[s["pair"] if c["id"] == s["id"] else s["id"]]
                for t in set(c["tone"]) - set(other["tone"]):
                    w[t] += 0.5; d.append((t, 0.5, "对照题选中"))
                for t in set(other["tone"]) - set(c["tone"]):
                    w[t] -= 0.3; d.append((t, -0.3, "对照题没选"))
                if c["form"] != other["form"]:
                    f[c["form"]] += 0.5; f[other["form"]] -= 0.3
                break
        per.append({"round": r["n"], "chosen": c["id"], "conf": k, "delta": d})
    m = max([abs(v) for v in w.values()] + [1e-9])
    p = {t: v / m for t, v in w.items()}
    mf = max([abs(v) for v in f.values()] + [1e-9])
    pf = {t: v / mf for t, v in f.items()}
    Lbar = sum(lens) / len(lens) if lens else 0
    prof = {"p": p, "pf": pf, "L": Lbar, "per_round": per, "raw": dict(w)}
    if PROFILE_MODE == "logit" and rounds:
        beta = fit_logit(rounds)
        prof["beta"] = beta
        mb = max([abs(v) for f, v in beta.items() if f.startswith("t:")] + [1e-9])
        prof["p"] = {f[2:]: v / mb for f, v in beta.items() if f.startswith("t:")}
        mfb = max([abs(v) for f, v in beta.items() if f.startswith("f:")] + [1e-9])
        prof["pf"] = {f[2:]: v / mfb for f, v in beta.items() if f.startswith("f:")}
    return prof


def score(b, prof):
    if "beta" in prof:
        beta = prof["beta"]
        return sum(beta.get(f, 0) * v for f, v in features(b).items()) + 0.03 * b["q"]
    p, pf = prof["p"], prof["pf"]
    s = (sum(p.get(t, 0) for t in b["topic"]) + 0.7 * sum(p.get(t, 0) for t in b["tone"])) / math.sqrt(len(b["topic"]) + len(b["tone"]))
    s += 0.35 * pf.get(b["form"], 0) - 0.15 * abs(b["_len"] - prof["L"]) + 0.03 * b["q"]
    return s


def score_all(prof, exclude):
    ops = 0
    out = {}
    for b in BOOKS:
        ops += len(TAGS["topics"]) + len(TAGS["tones"])  # 每本书和 32 个标签维度比较一次
        if b["id"] in exclude:
            continue
        out[b["id"]] = score(b, prof)
    return out, ops


def candidates(prof, exclude, k=12, pool=40):
    scores, ops = score_all(prof, exclude)
    ranked = sorted(scores, key=lambda i: -scores[i])
    top = ranked[:pool]
    positive = sum(1 for v in scores.values() if v > 0)
    chosen = []
    authors, series, cats = Counter(), set(), Counter()
    rest = list(top)
    while rest and len(chosen) < k:
        best, bv = None, -1e9
        for i in rest:
            b = BY_ID[i]
            if authors[b["author"]] >= 1 and b["author"] != "佚名":
                continue
            if b["series"] and b["series"] in series:
                continue
            sim = max([jaccard(b, BY_ID[c]) for c in chosen] + [0])
            v = 0.7 * scores[i] - 0.3 * sim
            # 至少覆盖 3 个大类：最后 3 个名额优先给新大类
            if len(chosen) >= k - 3 and len(cats) < 3 and b["category"] in cats:
                v -= 1
            if v > bv:
                best, bv = i, v
        if best is None:
            break
        rest.remove(best)
        b = BY_ID[best]
        chosen.append(best); authors[b["author"]] += 1; cats[b["category"]] += 1
        if b["series"]:
            series.add(b["series"])
    s1, s40 = scores[top[0]], scores[top[-1]]
    interest = {i: round(58 + 36 * (scores[i] - s40) / max(1e-9, s1 - s40)) for i in chosen}
    funnel = [("书库里的全部书", len(BOOKS)), ("去掉你已经看过的", len(BOOKS) - len(exclude)),
              ("和你的口味有关的", positive), ("最接近你口味的", len(top)), ("交给元小智细读", len(chosen))]
    return {"ids": chosen, "interest": interest, "scores": scores, "ops": ops, "funnel": funnel}


# ---------------- 出题 ----------------
def eligible(shown_ids, authors):
    return [b for b in BOOKS if b["id"] not in shown_ids and b["_g0"] <= 6 and (authors[b["author"]] < 2 or b["author"] == "佚名")]


def gen_round(n, rounds, rng, mode="adaptive"):
    shown_ids = {s["id"] for r in rounds for s in r["shown"]}
    authors = Counter(BY_ID[i]["author"] for i in shown_ids)
    seen_cats = {BY_ID[i]["category"] for i in shown_ids}
    slots = []

    def pool():
        return [b for b in eligible(shown_ids | {s["id"] for s in slots}, authors + Counter(BY_ID[s["id"]]["author"] for s in slots))]

    def add(b, purpose, note="", pair=None):
        if b is None:
            return False
        slots.append({"id": b["id"], "purpose": purpose, "note": note, "pair": pair})
        return True

    def pick(cands, key=None, top=3):
        if not cands:
            return None
        if key:
            cands = sorted(cands, key=key)[:top]
        return rng.choice(cands)

    if mode == "random":
        while len(slots) < 5:
            add(rng.choice(pool()), "random")
        return slots

    prof = profile(rounds) if rounds else None
    sc = (lambda b: -score(b, prof)) if prof else None
    if n == 1:
        cats = rng.sample(CATS, 5)
        for c in cats:
            add(pick([b for b in pool() if b["category"] == c and b["anchor"]] or [b for b in pool() if b["category"] == c]), "anchor", "摸底")
        if all(BY_ID[s["id"]]["form"] == "小说" for s in slots):
            slots.pop()
            add(pick([b for b in pool() if b["anchor"] and b["form"] != "小说" and b["category"] not in cats]), "anchor", "摸底")
    else:
        last = BY_ID[rounds[-1]["chosen"]]
        unseen = lambda: [c for c in CATS if c not in seen_cats and c not in {BY_ID[s["id"]]["category"] for s in slots}]

        def explore(k):
            for _ in range(k):
                us = unseen()
                c = rng.choice(us) if us else None
                cand = [b for b in pool() if (c is None or b["category"] == c)]
                add(pick([b for b in cand if b["anchor"]] or cand), "explore", f"探索：你还没见过的{c or '书'}")

        def exploit(k):
            for _ in range(k):
                add(pick(pool(), key=sc, top=4), "exploit", "试探：和你选过的书口味相近")

        def contrast_tone(base):
            cand = [b for b in pool() if (set(b["topic"]) & set(base["topic"])) and not (set(b["tone"]) & set(base["tone"]))]
            b = pick(cand, key=sc, top=4)
            if b and add(b, "contrast", f"对照题：同样是{('、'.join(set(b['topic']) & set(base['topic'])))}，风格不同", pair=base["id"]):
                return True
            return False

        if n == 2:
            add(pick([b for b in pool() if len(b["_tags"] & last["_tags"]) >= 2], key=sc, top=4), "exploit", f"试探：上一轮你选了《{last['title']}》，这本口味相近")
            cand = [b for b in pool() if b["category"] == last["category"] and not (set(b["tone"]) & set(last["tone"]))]
            b = pick(cand)
            if b:
                add(b, "contrast", f"对照题：和《{last['title']}》同类，风格不同", pair=None)
            explore(5 - len(slots))
        elif n == 3:
            exploit(2)
            base = BY_ID[slots[0]["id"]]
            if not contrast_tone(base):
                explore(1)
            explore(5 - len(slots))
        elif n == 4:
            exploit(2)
            top_topic = max(prof["p"], key=lambda t: prof["p"][t] if t in TAGS["topics"] else -9)
            fav_form = max(prof["pf"], key=prof["pf"].get) if prof["pf"] else "小说"
            base = BY_ID[slots[0]["id"]]
            cand = [b for b in pool() if top_topic in b["topic"] and b["form"] != base["form"]]
            b = pick(cand, key=sc, top=4)
            if b:
                add(b, "contrast", f"对照题：都是{top_topic}，体裁不同", pair=base["id"] if top_topic in base["topic"] else None)
            explore(1)
            cand = sorted(pool(), key=lambda b: -b["q"])[:60]
            add(rng.choice(cand), "wildcard", "彩蛋：书库里口碑很好的一本")
        else:
            tp = sorted([t for t in prof["p"] if t in TAGS["topics"]], key=lambda t: -prof["p"][t])
            h1 = tp[0] if tp else None
            h2 = next((t for t in tp[1:] if t != h1), None)
            for h, other in ((h1, h2), (h2, h1)):
                cand = [b for b in pool() if h in b["topic"] and other not in b["topic"]]
                for _ in range(2):
                    b = pick(cand, key=sc, top=5)
                    if b:
                        add(b, "exploit", f"二选一：你更喜欢{h}吗？")
                        cand = [x for x in cand if x["id"] != b["id"]]
            Lbar = prof["L"]
            cand = [b for b in pool() if (b["_len"] > 0) == (Lbar <= 0)]
            add(pick(cand, key=sc, top=4), "contrast", "篇幅对照：一本" + ("长" if Lbar <= 0 else "短") + "一点的书")
    while len(slots) < 5:
        add(rng.choice(pool()), "explore", "探索")
    rng.shuffle(slots)
    return slots[:5]


# ---------------- 本地兜底 ----------------
ADJ = {"搞笑": "快乐的", "紧张刺激": "勇敢的", "温暖感人": "温柔的", "烧脑": "爱动脑的", "想象力": "爱幻想的", "知识多": "好奇的", "励志": "不服输的", "安静优美": "安静的"}
NOUN = {"冒险": "冒险家", "动物": "动物朋友", "科学": "小科学家", "侦探推理": "小侦探", "太空宇宙": "星空旅人", "历史": "历史迷",
        "魔法奇幻": "魔法师", "童话寓言": "童话迷", "科幻": "未来探险家", "机器人科技": "小工程师", "自然植物": "自然观察家", "恐龙古生物": "恐龙迷",
        "海洋": "海洋探险家", "校园生活": "校园达人", "友谊": "好伙伴", "家庭亲情": "暖心读者", "成长": "追梦少年", "神话传说": "神话迷",
        "传统文化": "国风少年", "数学": "数学迷", "人体健康": "健康小卫士", "地理旅行": "小旅行家", "艺术体育": "小艺术家", "名人故事": "榜样追随者"}


def persona(prof):
    tp = [t for t in sorted(prof["p"], key=lambda t: -prof["p"][t]) if t in TAGS["topics"]]
    tn = [t for t in sorted(prof["p"], key=lambda t: -prof["p"][t]) if t in TAGS["tones"]]
    name = ADJ.get(tn[0], "") + NOUN.get(tp[0], "读书人") if tp and tn else "爱读书的人"
    return name, (tp[:2] + tn[:1])
