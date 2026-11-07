#!/usr/bin/env python3
"""离线自检：用模拟读者检验推荐算法（《02_推荐算法与提示词》第 9 节）。不调用模型。

用法：python3 sim.py [读者数=200] [种子=7]
"""
import math, random, sys, json
from collections import Counter
from rec import BOOKS, BY_ID, TAGS, CATS, gen_round, profile, candidates, persona

TOPICS, TONES, FORMS = TAGS["topics"], TAGS["tones"], TAGS["forms"]


def make_reader(rng):
    likes = rng.sample(TOPICS, rng.choice([2, 3]))
    dislike = rng.choice([t for t in TOPICS if t not in likes])
    return {"likes": likes, "tone": rng.choice(TONES), "dislike": dislike,
            "form": rng.choice(FORMS), "len": rng.choice([-1, 0, 1])}


def utility(r, b):
    u = sum(1.0 for t in r["likes"] if t in b["topic"])
    u += 0.8 if r["tone"] in b["tone"] else 0
    u -= 1.0 if r["dislike"] in b["topic"] else 0
    u += 0.5 if b["form"] == r["form"] else 0
    u -= 0.3 * abs(b["_len"] - r["len"])
    return u


def choose(r, shown, rng, temp=0.5, noise=0.3):
    us = [utility(r, BY_ID[s["id"]]) + rng.gauss(0, noise) for s in shown]
    mx = max(us)
    ps = [math.exp((u - mx) / temp) for u in us]
    tot = sum(ps)
    x, acc = rng.random() * tot, 0
    k = len(us) - 1
    for i, p in enumerate(ps):
        acc += p
        if x <= acc:
            k = i
            break
    srt = sorted(us, reverse=True)
    gap = srt[0] - srt[1]
    ms = int(2500 + 14000 * math.exp(-2 * gap) + rng.random() * 2000)
    opened = [shown[i]["id"] for i in sorted(range(len(us)), key=lambda i: -us[i])[:2] if rng.random() < 0.4]
    return shown[k]["id"], ms, opened


def run(reader, rng, mode):
    rounds = []
    for n in range(1, 6):
        shown = gen_round(n, rounds, rng, mode=mode)
        c, ms, opened = choose(reader, shown, rng)
        rounds.append({"n": n, "shown": shown, "chosen": c, "ms": ms, "opened": opened})
    prof = profile(rounds)
    shown_ids = {s["id"] for r in rounds for s in r["shown"]}
    cand = candidates(prof, shown_ids)
    top3 = sorted(cand["ids"], key=lambda i: -cand["interest"][i])[:3]
    return rounds, prof, cand, top3, shown_ids


def percentile(reader, bid, shown_ids):
    us = sorted(utility(reader, b) for b in BOOKS if b["id"] not in shown_ids)
    u = utility(reader, BY_ID[bid])
    below = sum(1 for x in us if x < u)
    equal = sum(1 for x in us if x == u)
    return (below + 0.5 * equal) / len(us)


def evaluate(n_readers, seed, mode):
    rng = random.Random(seed)
    readers = [make_reader(rng) for _ in range(n_readers)]
    pick_top20 = reader_ok = prof_ok = 0
    cats3 = []
    rec_counter = Counter()
    pcts = []
    dup = 0
    for rd in readers:
        rounds, prof, cand, top3, shown = run(rd, rng, mode)
        ids = [s["id"] for r in rounds for s in r["shown"]]
        dup += len(ids) - len(set(ids))
        ps = [percentile(rd, b, shown) for b in top3]
        pcts += ps
        pick_top20 += sum(1 for p in ps if p >= 0.8)
        reader_ok += 1 if sum(ps) / 3 >= 0.8 else 0
        tp = [t for t in sorted(prof["p"], key=lambda t: -prof["p"][t]) if t in TOPICS][:2]
        prof_ok += 1 if set(tp) & set(rd["likes"]) else 0
        cats3.append(len({BY_ID[s["id"]]["category"] for r in rounds[:3] for s in r["shown"]}))
        rec_counter.update(top3)
    n = n_readers
    return {
        "mode": mode,
        "推荐书在前20%的比例": round(pick_top20 / (3 * n), 3),
        "3本平均进前20%的读者比例": round(reader_ok / n, 3),
        "推荐书的平均百分位": round(sum(pcts) / len(pcts), 3),
        "画像前2题材命中喜欢的题材": round(prof_ok / n, 3),
        "被推荐过的不同书占书库": round(len(rec_counter) / len(BOOKS), 3),
        "单本最高占比": round(max(rec_counter.values()) / (3 * n), 3),
        "最常被推荐的5本": [(BY_ID[i]["title"], c) for i, c in rec_counter.most_common(5)],
        "第3轮结束时大类数（最少/平均）": (min(cats3), round(sum(cats3) / n, 1)),
        "5轮内重复出书次数": dup,
    }


if __name__ == "__main__":
    N = int(sys.argv[1]) if len(sys.argv) > 1 else 200
    seed = int(sys.argv[2]) if len(sys.argv) > 2 else 7
    a = evaluate(N, seed, "adaptive")
    b = evaluate(N, seed, "random")
    print(json.dumps([a, b], ensure_ascii=False, indent=1))
