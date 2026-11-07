#!/usr/bin/env python3
"""带模型的推荐测试（《02_推荐算法与提示词》第 6–7 节、《公共/03》第 7 节 V1–V4）。

密钥只从环境变量读取：YXZ_TEXT_BASE、YXZ_TEXT_KEY。不要把密钥写进任何文件。
用法：
  python3 llm_test.py main  [读者数=10] [并发=5] [effort=medium]
  python3 llm_test.py again [读者数=3]
  python3 llm_test.py effort            # 同一批读者比较 off / low / medium
结果写到 results/*.json，并打印汇总。
"""
import json, os, random, re, sys, time, threading, urllib.request
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import rec, sim
from rec import BY_ID, BOOKS, TAGS, gen_round, profile, candidates, persona

HERE = Path(__file__).resolve().parent
BASE, KEY = os.environ["YXZ_TEXT_BASE"], os.environ["YXZ_TEXT_KEY"]
MODEL = os.environ.get("YXZ_TEXT_MODEL", "YM ·DeepSeek-V4.1")
INDEX = (rec.LIB / "books_index.txt").read_text(encoding="utf-8")
TOPICS, TONES = TAGS["topics"], TAGS["tones"]
ALLTAGS = set(TOPICS) | set(TONES)


# ---------------- 组装提示词 ----------------
def sec(ms):
    s = ms / 1000
    return f"{s:.1f} 秒" + ("（很果断）" if ms < 4000 else ("（犹豫了一会儿）" if ms > 12000 else ""))


def bk(i):
    b = BY_ID[i]
    return f"{i}《{b['title']}》[{','.join(b['topic'])};{','.join(b['tone'])}|{b['form']}|{b['length']}]"


PURPOSE = {"anchor": "摸底", "exploit": "试探喜好", "contrast": "对照", "explore": "探索", "wildcard": "彩蛋", "random": "随机"}


def user_msg(rounds, prof, cand):
    L = ["【5 轮选书记录】"]
    for r in rounds:
        items = " | ".join(f"{bk(s['id'])} 目的={PURPOSE.get(s['purpose'], s['purpose'])}" for s in r["shown"])
        opened = "、".join(f"《{BY_ID[i]['title']}》" for i in r["opened"]) or "无"
        L.append(f"第{r['n']}轮：{items}")
        L.append(f"  选了 {bk(r['chosen'])}，用时 {sec(r['ms'])}；看过简介：{opened}")
    p = prof["p"]
    pos = sorted([t for t in p if p[t] > 0], key=lambda t: -p[t])[:6]
    neg = sorted([t for t in p if p[t] < 0], key=lambda t: p[t])[:3]
    L.append("【阅读画像】喜欢：" + "，".join(f"{t} {p[t]:.2f}" for t in pos) + "；不太感兴趣：" + ("，".join(f"{t} {p[t]:.2f}" for t in neg) or "无"))
    pf = prof["pf"]
    fav = max(pf, key=pf.get) if pf else "小说"
    Ls = prof["L"]
    L.append(f"喜欢的体裁：{fav}；篇幅：{'偏短' if Ls < -0.2 else ('偏长' if Ls > 0.2 else '适中')}（{Ls:+.1f}）")
    L.append("【候选书】")
    for i in cand["ids"]:
        b = BY_ID[i]
        L.append(f"{i}《{b['title']}》{b['author']}｜{','.join(b['topic'])};{','.join(b['tone'])}｜{b['form']}｜{b['length']}｜参考感兴趣程度 {cand['interest'][i]}｜简介：{b['summary']}")
    return "\n".join(L)


def local_output(rounds, prof, cand):
    """本地兜底（也用来生成提示词里的示例）。只用真实轨迹里的数据。"""
    name, tags = persona(prof)
    top3 = sorted(cand["ids"], key=lambda i: -cand["interest"][i])[:3]
    lines = []
    tp = [t for t in sorted(prof["p"], key=lambda t: -prof["p"][t]) if t in TOPICS]
    t0 = tp[0]
    rs = [r["n"] for r in rounds if t0 in BY_ID[r["chosen"]]["topic"]]
    if len(rs) >= 2:
        lines.append(f"想|证据|第{rs[0]}轮和第{rs[-1]}轮，你都选了{t0}的书。")
    fast = min(rounds, key=lambda r: r["ms"])
    word = "很果断" if fast["ms"] < 4000 else "是你选得最快的一次"
    lines.append(f"想|证据|第{fast['n']}轮你{fast['ms']/1000:.0f}秒就选了《{BY_ID[fast['chosen']]['title']}》，{word}。")
    slow = max(rounds, key=lambda r: r["ms"])
    if slow["ms"] > 12000:
        lines.append(f"想|推断|第{slow['n']}轮你犹豫了{slow['ms']/1000:.0f}秒，几本书你都有点喜欢。")
    neg = [t for t in sorted(prof["p"], key=lambda t: prof["p"][t]) if prof["p"][t] < 0]
    for t in neg[:1]:
        rn = [r["n"] for r in rounds if any(t in BY_ID[x["id"]]["_tags"] for x in r["shown"] if x["id"] != r["chosen"])]
        if rn:
            lines.append(f"想|排除|第{rn[0]}轮有{t}的书，你没有选它。")
    lines.append(f"想|结论|所以我重点找：{'、'.join(tags)}。")
    lines.append(f"画像|{name}|{','.join(tags)}")
    used = set()
    for i in top3:
        b = BY_ID[i]
        ranked = sorted(rounds, key=lambda r: (-len(set(BY_ID[r["chosen"]]["topic"]) & set(b["topic"])), -len(BY_ID[r["chosen"]]["_tags"] & b["_tags"]), r["n"] in used))
        ev = [r for r in ranked if BY_ID[r["chosen"]]["_tags"] & b["_tags"]][:2]
        if ev:
            used.add(ev[0]["n"])
            common = "、".join(sorted(BY_ID[ev[0]["chosen"]]["_tags"] & b["_tags"], key=lambda t: t not in TOPICS)[:2])
            reason = f"你第{ev[0]['n']}轮选了《{BY_ID[ev[0]['chosen']]['title']}》，这本也是{common}的书。"
        else:
            reason = b["hook"]
        if len(reason) > 45:
            reason = f"你第{ev[0]['n']}轮选的书也有{common}。" if ev else b["hook"]
        lines.append(f"荐|{i}|{cand['interest'][i]}|{reason}|{','.join(str(r['n']) for r in sorted(ev, key=lambda r: r['n'])) or '1'}")
    lines.append("完")
    return "\n".join(lines)


def simulate(seed, temp=0.3):
    rng = random.Random(seed)
    rd = sim.make_reader(rng)
    rounds = []
    for n in range(1, 6):
        shown = gen_round(n, rounds, rng)
        c, ms, op = sim.choose(rd, shown, rng, temp=temp)
        rounds.append({"n": n, "shown": shown, "chosen": c, "ms": ms, "opened": op})
    prof = profile(rounds)
    shown_ids = {s["id"] for r in rounds for s in r["shown"]}
    cand = candidates(prof, shown_ids)
    return rd, rounds, prof, cand, shown_ids


CATALOG_IN_PREFIX = os.environ.get("YXZ_CATALOG_IN_PREFIX", "0") == "1"
PLACE = os.environ.get("YXZ_PREFIX_PLACE", "user")   # system | user（课堂网关的 YM 模型会不定期丢掉 system 消息）


def build_prefix():
    _, r, p, c, _ = simulate(9999)
    example = "【示例输入】\n" + user_msg(r, p, c) + "\n【示例输出】\n" + local_output(r, p, c)
    rules = (HERE / "prompts/recommend_rules.md").read_text(encoding="utf-8").strip()
    if not CATALOG_IN_PREFIX:
        rules = rules.replace("最多可以有 1 本不在候选里，但必须在下面的书库索引里，且不在学生已经看过的 25 本里。", "只能从候选书里选。")
        rules = rules.replace("想|发现|……（只有推荐了候选之外的书时才写）\n", "")
    parts = [(HERE / "prompts/persona.md").read_text(encoding="utf-8").strip(), rules,
             "【示例】（示例只演示格式和写法；真实回答要更具体，引用更多细节）\n" + example]
    if CATALOG_IN_PREFIX:
        parts.append("【书库索引】每行：编号|书名|作者|大类|题材标签;风格标签|体裁|篇幅|一句话\n" + INDEX.strip())
    return "\n\n".join(parts)


def make_messages(um, history=()):
    if PLACE == "system":
        return [{"role": "system", "content": PREFIX}, {"role": "user", "content": um}, *history]
    return [{"role": "user", "content": PREFIX + "\n\n===== 下面是这位学生的数据 =====\n" + um}, *history]


PREFIX = build_prefix()


# ---------------- 调用模型（流式） ----------------
def call(messages, effort="medium", temperature=0.7, max_tokens=6000):
    body = {"model": MODEL, "stream": True, "stream_options": {"include_usage": True}, "messages": messages,
            "temperature": temperature, "max_tokens": max_tokens}
    if effort == "off":
        body["thinking"] = {"type": "disabled"}
    else:
        body["reasoning_effort"] = effort
    req = urllib.request.Request(BASE + "/chat/completions", data=json.dumps(body).encode(),
                                 headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
    t0 = time.time()
    out = {"t_first_reason": None, "t_first_content": None, "t_first_line": None, "content": "", "reasoning": "", "usage": None, "line_times": []}
    buf = ""
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            for raw in r:
                line = raw.decode("utf-8", "ignore").strip()
                if not line.startswith("data:"):
                    continue
                d = line[5:].strip()
                if d == "[DONE]":
                    break
                j = json.loads(d)
                if j.get("usage"):
                    out["usage"] = j["usage"]
                for ch in j.get("choices", []):
                    dl = ch.get("delta", {})
                    rc = dl.get("reasoning_content") or dl.get("reasoning") or ""
                    if rc:
                        if out["t_first_reason"] is None:
                            out["t_first_reason"] = time.time() - t0
                        out["reasoning"] += rc
                    c = dl.get("content") or ""
                    if c:
                        if out["t_first_content"] is None:
                            out["t_first_content"] = time.time() - t0
                        out["content"] += c
                        buf += c
                        while "\n" in buf:
                            ln, buf = buf.split("\n", 1)
                            if ln.strip():
                                out["line_times"].append((round(time.time() - t0, 2), ln.strip()[:12]))
                                if out["t_first_line"] is None:
                                    out["t_first_line"] = time.time() - t0
    except Exception as e:  # noqa
        out["error"] = repr(e)[:300]
    out["t_total"] = time.time() - t0
    return out


# ---------------- 解析与校验（第 7 节） ----------------
def match_title(t, titles):
    """模型常用简称，如《戴小桥》《哈利·波特》：前 3 个字以上的前缀匹配就算对。"""
    if t in titles:
        return t
    for full in titles:
        if len(t) >= 3 and (full.startswith(t) or t in full):
            return full
    return None


def validate(text, rounds, cand):
    shown_ids = {s["id"] for r in rounds for s in r["shown"]}
    shown_titles = {BY_ID[i]["title"]: i for i in shown_ids}
    round_of = {}
    for r in rounds:
        for s in r["shown"]:
            round_of[BY_ID[s["id"]]["title"]] = r["n"]
    res = {"thoughts": [], "dropped_thoughts": [], "persona": None, "picks": [], "dropped_picks": [], "format_ok": True, "end": False, "bad_lines": []}
    for ln in [l.strip() for l in text.splitlines() if l.strip()]:
        c = ln.split("|")
        if c[0] == "想" and len(c) >= 3:
            t = "|".join(c[2:])
            ok = c[1] in ("证据", "推断", "排除", "发现", "结论") and len(t) <= 40
            for m in re.finditer(r"《(.+?)》", t):
                rec_titles = {BY_ID[p['id']]['title'] for p in res["picks"]} | {BY_ID[i]["title"] for i in cand["ids"]}
                title = match_title(m.group(1), set(shown_titles) | rec_titles)
                if title is None:
                    ok = False
                if title in round_of:
                    near = re.findall(r"第(\d)轮", t)
                    if near and len(near) == 1 and str(round_of[title]) not in near:
                        ok = False
            (res["thoughts"] if ok else res["dropped_thoughts"]).append((c[1], t))
        elif c[0] == "画像" and len(c) >= 3:
            tags = [x for x in re.split(r"[,，、]", c[2]) if x]
            res["persona"] = (c[1], tags, len(c[1]) <= 8 and all(x in ALLTAGS for x in tags))
        elif c[0] == "荐" and len(c) >= 4:
            bid = c[1].strip()
            try:
                it = int(c[2])
            except ValueError:
                it = None
            reason = c[3] if len(c) > 3 else ""
            evid = [int(x) for x in re.findall(r"\d", c[4])] if len(c) > 4 else []
            prob = []
            if bid not in BY_ID: prob.append("编号不存在")
            elif bid in shown_ids: prob.append("推荐了看过的书")
            if bid in [p["id"] for p in res["picks"]]: prob.append("重复")
            if it is None: prob.append("感兴趣程度不是整数")
            if len(reason) > 45: prob.append(f"理由{len(reason)}字")
            for m in re.finditer(r"《(.+?)》", reason):
                if match_title(m.group(1), set(shown_titles) | ({BY_ID[bid]["title"]} if bid in BY_ID else set())) is None:
                    prob.append("理由提到没出现过的书")
            good_ev = [n for n in evid if 1 <= n <= 5 and bid in BY_ID and BY_ID[rounds[n-1]["chosen"]]["_tags"] & BY_ID[bid]["_tags"]]
            outside = bid in BY_ID and bid not in cand["ids"]
            item = {"id": bid, "interest": it, "reason": reason, "evidence": evid, "good_evidence": good_ev, "outside": outside, "problems": prob}
            (res["picks"] if not [p for p in prob if p != f"理由{len(reason)}字"] else res["dropped_picks"]).append(item)
        elif ln == "完":
            res["end"] = True
        else:
            res["format_ok"] = False
            res["bad_lines"].append(ln[:40])
    outside_n = sum(1 for p in res["picks"] if p["outside"])
    res["outside_n"] = outside_n
    # 最终感兴趣程度：夹在本地参考值 ±8 内，50–97，严格递减
    final = []
    for p in res["picks"][:3]:
        loc = cand["interest"].get(p["id"])
        v = p["interest"] if p["interest"] is not None else (loc or 60)
        if loc is not None:
            v = min(max(v, loc - 8), loc + 8)
        final.append([p["id"], max(50, min(97, v))])
    final.sort(key=lambda x: -x[1])
    for k in range(1, len(final)):
        if final[k][1] >= final[k-1][1]:
            final[k][1] = final[k-1][1] - 1
    res["final"] = final
    return res


def one(seed, effort):
    rd, rounds, prof, cand, shown = simulate(seed)
    um = user_msg(rounds, prof, cand)
    msgs = make_messages(um)
    o = call(msgs, effort=effort)
    v = validate(o["content"], rounds, cand)
    local3 = sorted(cand["ids"], key=lambda i: -cand["interest"][i])[:3]
    pct = lambda ids: [round(sim.percentile(rd, i, shown), 3) for i in ids if i in BY_ID]
    return {"seed": seed, "effort": effort, "reader": rd, "t_first_reason": o["t_first_reason"], "t_first_content": o["t_first_content"],
            "t_first_line": o["t_first_line"], "t_total": o["t_total"], "usage": o["usage"], "reasoning_chars": len(o["reasoning"]),
            "error": o.get("error"), "content": o["content"], "reasoning_head": o["reasoning"][:600], "line_times": o["line_times"],
            "valid": v, "llm_pct": pct([p[0] for p in v["final"]]), "local_pct": pct(local3),
            "messages_user": um, "assistant": o["content"]}


def summarize(rows, title):
    ok = [r for r in rows if not r["error"]]
    def avg(xs):
        xs = [x for x in xs if x is not None]
        return round(sum(xs) / len(xs), 2) if xs else None
    u = [r["usage"] or {} for r in ok]
    cached = [((x.get("prompt_tokens_details") or {}).get("cached_tokens") or x.get("prompt_cache_hit_tokens") or 0) for x in u]
    prompt = [x.get("prompt_tokens", 0) for x in u]
    s = {
        "场景": title, "调用数": len(rows), "失败": len(rows) - len(ok),
        "平均首个推理token秒": avg([r["t_first_reason"] for r in ok]),
        "平均首个正文秒": avg([r["t_first_content"] for r in ok]),
        "平均首行思考秒": avg([r["t_first_line"] for r in ok]),
        "平均总耗时秒": avg([r["t_total"] for r in ok]),
        "最长总耗时秒": round(max([r["t_total"] for r in ok] or [0]), 1),
        "平均输入token": avg(prompt), "平均缓存命中token": avg(cached),
        "缓存命中率": round(sum(cached) / max(1, sum(prompt)), 3),
        "平均输出token": avg([x.get("completion_tokens") for x in u]),
        "平均推理字数": avg([r["reasoning_chars"] for r in ok]),
        "格式完全正确": sum(1 for r in ok if r["valid"]["format_ok"] and r["valid"]["end"] and len(r["valid"]["picks"]) >= 3),
        "思考行通过/丢弃": (sum(len(r["valid"]["thoughts"]) for r in ok), sum(len(r["valid"]["dropped_thoughts"]) for r in ok)),
        "推荐通过/丢弃": (sum(len(r["valid"]["picks"]) for r in ok), sum(len(r["valid"]["dropped_picks"]) for r in ok)),
        "理由超长": sum(1 for r in ok for p in r["valid"]["picks"] if len(p["reason"]) > 45),
        "候选外推荐数": sum(r["valid"]["outside_n"] for r in ok),
        "依据轮次全部合格的推荐比例": round(sum(1 for r in ok for p in r["valid"]["picks"] if p["evidence"] and len(p["good_evidence"]) == len(p["evidence"])) / max(1, sum(len(r["valid"]["picks"]) for r in ok)), 3),
        "模型推荐的平均百分位": avg([sum(r["llm_pct"]) / len(r["llm_pct"]) for r in ok if r["llm_pct"]]),
        "本地前3的平均百分位": avg([sum(r["local_pct"]) / len(r["local_pct"]) for r in ok if r["local_pct"]]),
    }
    return s


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "main"
    (HERE / "results").mkdir(exist_ok=True)
    print("前缀字数", len(PREFIX))
    if mode == "main":
        n = int(sys.argv[2]) if len(sys.argv) > 2 else 10
        conc = int(sys.argv[3]) if len(sys.argv) > 3 else 5
        effort = sys.argv[4] if len(sys.argv) > 4 else "medium"
        # 先预热一次（max_tokens=1），再并发
        w = call(make_messages("（预热，请只回复：好）"), effort="off", max_tokens=1)
        print("预热", round(w["t_total"], 1), w["usage"])
        with ThreadPoolExecutor(conc) as ex:
            rows = list(ex.map(lambda s: one(s, effort), range(100, 100 + n)))
        (HERE / f"results/main_{effort}_{n}x{conc}.json").write_text(json.dumps(rows, ensure_ascii=False, indent=1), encoding="utf-8")
        print(json.dumps(summarize(rows, f"首次推荐 effort={effort} 并发{conc}"), ensure_ascii=False, indent=1))
    elif mode == "effort":
        rows = {}
        for eff in ["off", "low", "medium"]:
            rows[eff] = [one(s, eff) for s in range(200, 204)]
            print(json.dumps(summarize(rows[eff], f"effort={eff}"), ensure_ascii=False, indent=1))
        (HERE / "results/effort.json").write_text(json.dumps(rows, ensure_ascii=False, indent=1), encoding="utf-8")
    elif mode == "again":
        n = int(sys.argv[2]) if len(sys.argv) > 2 else 3
        out = []
        for s in range(100, 100 + n):
            first = one(s, os.environ.get("YXZ_EFFORT", "low"))
            rd, rounds, prof, cand, shown = simulate(s)
            picks = [p[0] for p in first["valid"]["final"]]
            # 模拟读者按隐藏口味给反馈：效用最高的那本“想读”，其余“不想读”
            likes = {i: (i == max(picks, key=lambda i: sim.utility(rd, BY_ID[i]))) for i in picks}
            prof2 = profile(rounds)
            for i, lk in likes.items():
                for t in BY_ID[i]["_tags"]:
                    prof2["p"][t] = prof2["p"].get(t, 0) + (0.4 if lk else -0.6) * 0.5
            excl = shown | set(picks)
            cand2 = candidates(prof2, excl)
            fb = "【学生的反馈】想读：" + "、".join(i for i, v in likes.items() if v) + "；不想读：" + "、".join(i for i, v in likes.items() if not v)
            fb += "\n【学生补充】“想要更好玩一点的”\n【新的候选书】\n" + "\n".join(
                f"{i}《{BY_ID[i]['title']}》｜{','.join(BY_ID[i]['topic'])};{','.join(BY_ID[i]['tone'])}｜参考感兴趣程度 {cand2['interest'][i]}｜简介：{BY_ID[i]['summary']}" for i in cand2["ids"])
            fb += "\n请按同样的格式重新推荐 3 本，并在第一行“想|证据|”里说明你根据反馈改了什么。"
            msgs = make_messages(first["messages_user"], ({"role": "assistant", "content": first["assistant"]}, {"role": "user", "content": fb}))
            o = call(msgs, effort=os.environ.get("YXZ_EFFORT", "low"))
            v = validate(o["content"], rounds, cand2)
            again_ids = [p[0] for p in v["final"]]
            out.append({"seed": s, "first_usage": first["usage"], "usage": o["usage"], "t_total": o["t_total"], "t_first_line": o["t_first_line"],
                        "content": o["content"], "repeat": len(set(again_ids) & set(picks)), "first_pct": first["llm_pct"],
                        "again_pct": [round(sim.percentile(rd, i, shown | set(picks)), 3) for i in again_ids]})
            print(s, "耗时", round(o["t_total"], 1), "用量", o["usage"], "重复", out[-1]["repeat"])
        (HERE / "results/again.json").write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")


if __name__ == "__main__":
    main()
