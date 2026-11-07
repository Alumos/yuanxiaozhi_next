#!/usr/bin/env python3
"""把 草稿/*.txt 整理成 books.json，并做自检（对应《04_图书数据采集规范》第 7 节）。

草稿每行一本书，用 | 分隔，共 13 列：
书名|作者|中/外|题材(、分隔)|风格(、分隔)|体裁|篇幅|年级段|摸底书(1/0)|系列|来源(M/K/A/C 可多个)|一句话|简介
以 # 开头的行是注释；“# 大类: xxx” 设定后面各行的大类。

用法：python3 build_books.py        生成 books.json、books_review.csv、books_index.txt、check_report.md
"""
import csv, json, math, re, sys
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent
TAGS = json.loads((ROOT / "tags.json").read_text(encoding="utf-8"))
CATS = [c["name"] for c in TAGS["categories"]]
TOPICS, TONES, FORMS, LENS = TAGS["topics"], TAGS["tones"], TAGS["forms"], TAGS["lengths"]
SRC = TAGS["sources"]
IDMAP_PATH = ROOT / "ids.json"   # 书名|作者 → id，保证 id 一旦分配就不再变

errors, warns = [], []


def parse():
    books = []
    for f in sorted((ROOT / "草稿").glob("*.txt")):
        cat = None
        for ln, raw in enumerate(f.read_text(encoding="utf-8").splitlines(), 1):
            line = raw.strip()
            if not line:
                continue
            if line.startswith("#"):
                m = re.match(r"#\s*大类[:：]\s*(\S+)", line)
                if m:
                    cat = m.group(1)
                    if cat not in CATS:
                        errors.append(f"{f.name}:{ln} 未知大类 {cat}")
                continue
            cols = [c.strip() for c in line.split("|")]
            where = f"{f.name}:{ln}"
            if len(cols) != 13:
                errors.append(f"{where} 应为 13 列，实际 {len(cols)} 列")
                continue
            title, author, origin, topic, tone, form, length, grade, anchor, series, src, hook, summary = cols
            b = {
                "title": title, "author": author,
                "origin": {"中": "中国", "外": "外国"}.get(origin, origin),
                "category": cat,
                "topic": [t for t in topic.split("、") if t],
                "tone": [t for t in tone.split("、") if t],
                "form": form, "length": length, "grade": grade,
                "hook": hook, "summary": summary,
                "anchor": anchor == "1", "series": series,
                "q": max((SRC[s]["q"] for s in src if s in SRC), default=0),
                "source": [SRC[s]["name"] for s in src if s in SRC],
                "in_library": None, "shelf": "",
                "_where": where,
            }
            check_one(b, src)
            books.append(b)
    return books


def check_one(b, src):
    w = b["_where"] + f"《{b['title']}》"
    if b["category"] is None:
        errors.append(f"{w} 没有大类")
    if b["origin"] not in ("中国", "外国"):
        errors.append(f"{w} 国别应为 中/外")
    for t in b["topic"]:
        if t not in TOPICS:
            errors.append(f"{w} 未知题材 {t}")
    for t in b["tone"]:
        if t not in TONES:
            errors.append(f"{w} 未知风格 {t}")
    if not 2 <= len(b["topic"]) <= 4:
        errors.append(f"{w} 题材应 2–4 个")
    if not 1 <= len(b["tone"]) <= 2:
        errors.append(f"{w} 风格应 1–2 个")
    if b["form"] not in FORMS:
        errors.append(f"{w} 未知体裁 {b['form']}")
    if b["length"] not in LENS:
        errors.append(f"{w} 篇幅应为 短/中/长")
    if not re.fullmatch(r"[3-9]-[3-9]", b["grade"]):
        errors.append(f"{w} 年级段格式应如 4-6")
    if not src or any(s not in SRC for s in src):
        errors.append(f"{w} 来源代码只能是 M/K/A/C")
    if len(b["hook"]) > 18:
        errors.append(f"{w} 一句话 {len(b['hook'])} 字，超过 18 字")
    n = len(b["summary"])
    if not 60 <= n <= 100:
        errors.append(f"{w} 简介 {n} 字，应为 60–100 字")


def assign_ids(books):
    idmap = json.loads(IDMAP_PATH.read_text(encoding="utf-8")) if IDMAP_PATH.exists() else {}
    used = set(idmap.values())
    nxt = max([int(v[1:]) for v in used] + [0]) + 1
    for b in books:
        key = f"{b['title']}|{b['author']}"
        if key not in idmap:
            idmap[key] = f"b{nxt:04d}"
            nxt += 1
        b["id"] = idmap[key]
    IDMAP_PATH.write_text(json.dumps(idmap, ensure_ascii=False, indent=1), encoding="utf-8")


def distribution(books):
    lines = []
    total = len(books)
    lines.append(f"# 书库自检报告\n\n共 **{total}** 本。\n")
    if errors:
        lines.append(f"## 错误（{len(errors)}）\n")
        lines += [f"- {e}" for e in errors]
        lines.append("")

    def table(title, counter, order, lo=None, hi=None):
        lines.append(f"## {title}\n\n| 项 | 数量 | 判定 |\n|---|---|---|")
        for k in order:
            v = counter.get(k, 0)
            ok = (lo is None or v >= lo) and (hi is None or v <= hi)
            if not ok:
                warns.append(f"{title}「{k}」{v} 本（要求 {lo if lo is not None else ''}–{hi if hi is not None else ''}）")
            lines.append(f"| {k} | {v} | {'✓' if ok else '✗'} |")
        lines.append("")

    table("大类", Counter(b["category"] for b in books), CATS, 20, 45)
    table("题材", Counter(t for b in books for t in b["topic"]), TOPICS, 8)
    table("风格", Counter(t for b in books for t in b["tone"]), TONES, 20)
    fc = Counter(b["form"] for b in books)
    lines.append("## 体裁\n\n| 体裁 | 数量 | 占比 |\n|---|---|---|")
    lines += [f"| {k} | {fc.get(k,0)} | {fc.get(k,0)/total:.0%} |" for k in FORMS]
    lines.append("")
    for title, key in (("国别", "origin"), ("篇幅", "length"), ("年级段", "grade")):
        c = Counter(b[key] for b in books)
        lines.append(f"## {title}\n\n" + "，".join(f"{k} {v}（{v/total:.0%}）" for k, v in sorted(c.items())) + "\n")

    # 摸底书：每个大类 ≥ 4
    ac = Counter(b["category"] for b in books if b["anchor"])
    table("摸底书（anchor）", ac, CATS, 4)

    # 风格对照：每个大类至少 2 种风格各 ≥ 3 本
    lines.append("## 风格对照可出题性（每个大类至少 2 种风格各 ≥3 本）\n")
    for c in CATS:
        tc = Counter(t for b in books if b["category"] == c for t in b["tone"])
        good = [t for t, v in tc.items() if v >= 3]
        ok = len(good) >= 2
        if not ok:
            warns.append(f"大类「{c}」能出风格对照题的风格不足：{dict(tc)}")
        lines.append(f"- {c}：{'✓' if ok else '✗'} " + "、".join(f"{t}{v}" for t, v in tc.most_common()))
    lines.append("")

    # 体裁对照：热门题材至少 2 种体裁各 ≥ 3 本
    lines.append("## 体裁对照可出题性（热门题材至少 2 种体裁各 ≥3 本）\n")
    for t in ["冒险", "动物", "科学", "魔法奇幻", "侦探推理", "校园生活"]:
        fc2 = Counter(b["form"] for b in books if t in b["topic"])
        good = [f for f, v in fc2.items() if v >= 3]
        ok = len(good) >= 2
        if not ok:
            warns.append(f"题材「{t}」能出体裁对照题的体裁不足：{dict(fc2)}")
        lines.append(f"- {t}：{'✓' if ok else '✗'} " + "、".join(f"{f}{v}" for f, v in fc2.most_common()))
    lines.append("")

    # 作者 ≤ 6；重复
    au = Counter(b["author"] for b in books)
    for a, v in au.items():
        if v > 6 and a != "佚名":
            warns.append(f"作者「{a}」有 {v} 本，超过 6 本")
    dup = [k for k, v in Counter(b["title"] for b in books).items() if v > 1]
    for d in dup:
        errors.append(f"书名重复：{d}")
    lines.append("## 作者（≥3 本）\n\n" + "，".join(f"{a} {v}" for a, v in au.most_common() if v >= 3) + "\n")

    # 来源
    sc = Counter(s for b in books for s in b["source"])
    lines.append("## 来源\n\n" + "\n".join(f"- {k}：{v}" for k, v in sc.items()) + "\n")

    if warns:
        lines.append(f"## 警告（{len(warns)}）\n")
        lines += [f"- {w}" for w in warns]
    return "\n".join(lines) + "\n"


def index_line(b):
    return f"{b['id']}|{b['title']}|{b['author']}|{b['category']}|{','.join(b['topic'])};{','.join(b['tone'])}|{b['form']}|{b['length']}|{b['hook']}"


def main():
    books = parse()
    assign_ids(books)
    books.sort(key=lambda b: b["id"])
    report = distribution(books)
    out = [{k: v for k, v in b.items() if not k.startswith("_")} for b in books]
    order = ["id", "title", "author", "origin", "category", "topic", "tone", "form", "length", "grade",
             "hook", "summary", "anchor", "series", "q", "source", "in_library", "shelf"]
    out = [{k: b[k] for k in order} for b in out]
    (ROOT / "books.json").write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")
    with open(ROOT / "books_review.csv", "w", encoding="utf-8-sig", newline="") as f:
        w = csv.writer(f)
        w.writerow(["编号", "书名", "作者", "国别", "大类", "题材", "风格", "体裁", "篇幅", "年级段", "一句话", "简介", "摸底书", "来源", "审核意见"])
        for b in out:
            w.writerow([b["id"], b["title"], b["author"], b["origin"], b["category"], "、".join(b["topic"]), "、".join(b["tone"]),
                        b["form"], b["length"], b["grade"], b["hook"], b["summary"], "是" if b["anchor"] else "", "；".join(b["source"]), ""])
    idx = "\n".join(index_line(b) for b in out)
    (ROOT / "books_index.txt").write_text(idx + "\n", encoding="utf-8")
    report += f"\n## 书库索引（提示词前缀）\n\n共 {len(idx)} 字，约 {math.ceil(len(idx) * 0.75)} token（粗估）。\n"
    (ROOT / "check_report.md").write_text(report, encoding="utf-8")
    print(f"{len(out)} 本；错误 {len(errors)}；警告 {len(warns)}")
    for e in errors[:80]:
        print("E", e)
    for w_ in warns[:80]:
        print("W", w_)
    sys.exit(1 if errors else 0)


if __name__ == "__main__":
    main()
