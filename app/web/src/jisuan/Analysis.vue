<script setup lang="ts">
/* 平板版分析动画（《计算 03》）：只用 transform / opacity；数据全部来自真实轨迹和模型输出。
 * 同一个组件也用于“再看一遍”和“再想想”短版。 */
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import BookCover from '../common/components/BookCover.vue'
import YxzAvatar from '../common/components/YxzAvatar.vue'
import RollingNumber from '../common/components/RollingNumber.vue'
import Stamp from '../common/components/Stamp.vue'
import { ICON } from '../common/icons'
import { FX, dur } from '../common/fx'
import { META, catOf, KIND_ICON, type Feed, type Pick } from './types'

const props = defineProps<{ feed: Feed; replay?: boolean }>()
const emit = defineEmits<{ (e: 'finish'): void }>()

const S = reactive({
  seg: 'wait' as 'wait' | 'rewind' | 'scan' | 'funnel' | 'think' | 'reveal' | 'strike' | 'bars',
  round: 0, // rewind: index into rounds (4..0)
  bars: {} as Record<string, number>,
  title: '正在提取你的阅读画像……',
  persona: '',
  stampPersona: false,
  scanned: false,
  counters: { books: 0, tags: 0, ops: 0, ms: 0 },
  funnelN: 0,
  showCands: false,
  focusCand: -1,
  thoughts: [] as { kind: string; text: string; shown: string }[],
  revealed: [] as Pick[],
  stamp: false,
  flyTags: [] as { id: number; tag: string; v: number; neg: boolean }[],
  quip: '',
  elapsed: 0,
  skipOK: false,
  struck: false,
})
let aborted = false
let flySeq = 0
const t0 = Date.now()
const timer = window.setInterval(() => { S.elapsed = Math.floor((Date.now() - t0) / 1000); if (S.elapsed >= 5) S.skipOK = true }, 500)
onBeforeUnmount(() => { aborted = true; clearInterval(timer) })

const tr = computed(() => props.feed.trace)
const isAgain = computed(() => tr.value?.kind === 'again')
const book = (id: string) => tr.value?.books[id]

function wait(ms: number) { return new Promise<void>((r) => { if (aborted) return r(); setTimeout(r, dur(ms)) }) }
function until(pred: () => boolean, maxMs: number) {
  return new Promise<boolean>((res) => {
    if (pred()) return res(true)
    const stop = watch(() => pred(), (v) => { if (v) { stop(); clearTimeout(t); res(true) } }, { deep: false })
    const t = setTimeout(() => { stop(); res(false) }, maxMs)
  })
}

// ---------- 阅读画像（右侧条形） ----------
const barTags = computed(() => {
  const all = tr.value?.profile.all || []
  const pick = [...all].sort((a, b) => Math.abs(b.v) - Math.abs(a.v)).slice(0, 8).map((t) => t.tag)
  return pick
})
const barRows = computed(() => {
  const rows = barTags.value.map((t) => ({ tag: t, v: S.bars[t] || 0 }))
  const order = [...rows].sort((a, b) => b.v - a.v)
  return rows.map((r) => ({ ...r, rank: order.indexOf(r) }))
})
const changeOf = (tag: string) => tr.value?.changes?.find((c) => c.tag === tag)?.v

// ---------- 记忆带 ----------
const rounds = computed(() => tr.value?.rounds || [])
const timecodes = computed(() => { let s = 0; return rounds.value.map((r) => (s += r.ms)) })
const tc = computed(() => {
  const r = S.round, all = timecodes.value
  const ms = r > 0 ? all[r - 1] : 0
  const sec = Math.round(ms / 1000)
  return `${String(Math.floor(sec / 60)).padStart(2, '0')}:${String(sec % 60).padStart(2, '0')}`
})
const curRound = computed(() => rounds.value[S.round])
const curDelta = computed(() => tr.value?.per_round?.[S.round])

// ---------- 扫描书库：120 根代表书脊 ----------
const spines = computed(() => {
  const sc = tr.value?.scores || []
  const n = sc.length, k = Math.max(1, Math.floor(n / 120))
  const out: { cat: number; s: number }[] = []
  for (let i = 0; i < n && out.length < 120; i += k) out.push({ cat: META.spines[i] ?? 0, s: sc[i] })
  return out
})
const heat = (s: number) => (s > 0.35 ? 'hot' : s > 0.12 ? 'warm' : s > -0.02 ? 'zero' : 'neg')

// ---------- 时间轴 ----------
async function rewind() {
  S.seg = 'rewind'
  const per = tr.value?.per_round || []
  for (let k = rounds.value.length - 1; k >= 0 && !aborted; k--) {
    S.round = k
    S.flyTags = []
    await wait(FX.low ? 200 : 500)
    const d = per[k]
    if (d) {
      const chips = d.deltas.slice(0, FX.low ? 3 : 6)
      S.flyTags = chips.map((c) => ({ id: ++flySeq, tag: c.tag, v: c.v, neg: c.v < 0 }))
      await wait(450)
      S.bars = { ...d.bars }
    }
    await wait(FX.low ? 400 : 700)
  }
  S.flyTags = []
  S.title = '你的阅读画像（初步）'
}
async function scan() {
  S.seg = 'scan'
  await wait(150)
  S.scanned = true
  S.counters = { books: META.books, tags: META.tags, ops: tr.value!.ops, ms: tr.value!.compute_ms }
  await wait(2900)
}
async function funnel() {
  S.seg = 'funnel'
  const f = tr.value!.funnel
  for (let i = 1; i <= f.length - 1 && !aborted; i++) { S.funnelN = i; await wait(380) }
  S.showCands = true
  await wait(800)
}
async function typeLine(line: { kind: string; text: string; shown: string }) {
  if (FX.low) { line.shown = line.text; return }
  const cps = 16
  for (let i = 1; i <= line.text.length && !aborted; i++) {
    line.shown = line.text.slice(0, i)
    await new Promise((r) => setTimeout(r, 1000 / cps))
  }
  line.shown = line.text
}
const quips = ['这本也不错……', '再比一比……', '嗯，有意思……', '看看简介……', '对一对你的口味……']
async function think() {
  S.seg = 'think'
  const tStart = Date.now()
  let i = 0, q = 0
  for (;;) {
    if (aborted) return
    if (i < props.feed.thoughts.length) {
      const l = { ...props.feed.thoughts[i], shown: '' }
      S.thoughts.push(l)
      if (S.thoughts.length > 6) S.thoughts.shift()
      S.focusCand = -1
      await typeLine(l)
      await wait(600)
      i++
      continue
    }
    if (props.feed.final || props.feed.done) {
      if (Date.now() - tStart > 6000 || props.replay) break
    }
    if (props.feed.persona && !S.persona) { S.persona = props.feed.persona.name; S.stampPersona = true }
    // 等待策略：细读候选（真实的命中标签和得分）
    const cs = tr.value!.candidates
    S.focusCand = (S.focusCand + 1) % Math.max(1, cs.length)
    if (S.focusCand === 0) S.quip = quips[q++ % quips.length]
    await until(() => props.feed.thoughts.length > i || !!props.feed.final, 1200)
  }
  if (props.feed.persona) { S.persona = props.feed.persona.name; S.stampPersona = true; S.title = '元小智猜你是' }
  await wait(500)
}
async function reveal() {
  await until(() => !!props.feed.final || props.feed.picks.length >= 3, 60000)
  S.seg = 'reveal'
  S.focusCand = -1
  const picks = props.feed.final?.picks || props.feed.picks
  for (let k = picks.length - 1; k >= 0 && !aborted; k--) {
    S.revealed.unshift(picks[k])
    await wait(k === 0 ? 900 : 800)
  }
  S.stamp = true
  await wait(1400)
}
async function againIntro() {
  S.seg = 'strike'
  const prev = tr.value!.per_round ? {} : null
  void prev
  S.bars = Object.fromEntries((tr.value!.profile.all || []).map((t) => [t.tag, t.v - (changeOf(t.tag) || 0)]))
  await wait(400)
  S.struck = true
  await wait(1000)
  S.seg = 'bars'
  S.title = '根据你的反馈调整画像'
  await wait(300)
  S.bars = Object.fromEntries((tr.value!.profile.all || []).map((t) => [t.tag, t.v]))
  await wait(1800)
  S.seg = 'funnel'
  S.funnelN = 1
  S.showCands = true
  await wait(1000)
}
async function run() {
  await until(() => !!props.feed.trace, 30000)
  if (!tr.value || aborted) return
  if (isAgain.value) await againIntro()
  else { await rewind(); await scan(); await funnel() }
  await think()
  await reveal()
  if (!aborted) emit('finish')
}
onMounted(run)
function skip() { aborted = true; emit('finish') }

const funnelSteps = computed(() => tr.value?.funnel || [])
const candBooks = computed(() => (tr.value?.candidates || []).map((c) => ({ c, b: book(c.id)! })).filter((x) => x.b))
</script>

<template>
  <div class="an">
    <div class="top">
      <div class="who"><YxzAvatar state="think" :size="46" /><b>元小智正在分析</b></div>
      <div class="tape" v-if="!isAgain">
        <span v-for="(r, i) in rounds" :key="r.n" class="cell" :class="{ on: S.seg === 'rewind' && S.round === i, done: S.seg !== 'rewind' || i > S.round }">
          <BookCover v-if="book(r.chosen)" :id="r.chosen" :title="book(r.chosen)!.title" :cat="catOf(book(r.chosen))" :w="30" />
        </span>
        <span v-if="S.seg === 'rewind'" class="rw"><span v-html="ICON.rewind" />倒带 · 第 {{ curRound?.n }} 轮 <span class="num">{{ tc }}</span></span>
      </div>
      <div class="live"><i />计算中 <span class="num">{{ String(Math.floor(S.elapsed / 60)).padStart(2, '0') }}:{{ String(S.elapsed % 60).padStart(2, '0') }}</span></div>
      <button v-if="S.skipOK" class="skip" @click="skip"><span v-html="ICON.skip" />跳过动画</button>
    </div>

    <div class="body">
      <section class="main">
        <!-- ① 倒带 -->
        <div v-if="S.seg === 'rewind' && curRound" :key="'r' + S.round" class="rewind">
          <div class="cards">
            <div v-for="s in curRound.shown" :key="s.book_id" class="mini" :class="{ chosen: s.book_id === curRound.chosen }">
              <BookCover :id="s.book_id" :title="book(s.book_id)?.title || ''" :cat="catOf(book(s.book_id))" :w="92" />
              <small>{{ book(s.book_id)?.title }}</small>
            </div>
          </div>
          <div class="obs"><span v-for="o in curDelta?.obs" :key="o" class="ob">{{ o }}</span></div>
          <div class="fly">
            <span v-for="f in S.flyTags" :key="f.id" class="ft" :class="{ neg: f.neg }">{{ f.tag }} {{ f.v > 0 ? '+' : '' }}{{ f.v.toFixed(2) }}</span>
          </div>
        </div>

        <!-- ② 扫描书库 -->
        <div v-else-if="S.seg === 'scan'" class="scan">
          <div class="shelf" :class="{ go: S.scanned }">
            <span v-for="(sp, i) in spines" :key="i" class="sp" :class="heat(sp.s)" :style="{ '--d': (i % 30) * 55 + 'ms', '--c': META.cats[sp.cat]?.color }" />
            <i class="band" />
          </div>
          <div class="ctr">
            <div><b><RollingNumber :value="S.counters.books" /></b>本书</div><span>×</span>
            <div><b><RollingNumber :value="S.counters.tags" /></b>个标签</div><span>=</span>
            <div class="hot"><b><RollingNumber :value="S.counters.ops" :ms="2200" /></b>次比较</div>
            <div class="dim">用时 <RollingNumber :value="S.counters.ms / 1000" :decimals="4" :sep="false" /> 秒</div>
          </div>
        </div>

        <!-- ③ 漏斗 / ④ 思考链 / ⑤ 揭晓 -->
        <div v-else-if="S.seg === 'funnel' || S.seg === 'think' || S.seg === 'reveal' || S.seg === 'strike' || S.seg === 'bars'" class="flow">
          <div v-if="S.seg === 'strike' || S.seg === 'bars'" class="struck">
            <div v-for="id in tr?.old_picks" :key="id" class="mini" :class="{ x: S.struck && tr?.struck?.includes(id), like: tr?.liked?.includes(id) }">
              <BookCover :id="id" :title="book(id)?.title || ''" :cat="catOf(book(id))" :w="80" /><small>{{ book(id)?.title }}</small>
            </div>
            <p v-if="tr?.text" class="said">你说：“{{ tr.text }}”</p>
          </div>
          <div v-if="S.seg === 'funnel'" class="funnel">
            <div v-for="(f, i) in funnelSteps.slice(0, -1)" :key="i" class="fl" :class="{ on: i < S.funnelN || isAgain }" :style="{ '--w': 100 - i * 15 + '%' }">
              <b class="num">{{ f.n }}</b><span>{{ f.text }}</span>
            </div>
          </div>
          <div v-if="(S.showCands && S.seg !== 'reveal')" class="cands">
            <div v-for="(x, i) in candBooks" :key="x.c.id" class="cd" :class="{ focus: S.focusCand === i }">
              <BookCover :id="x.c.id" :title="x.b.title" :cat="catOf(x.b)" :w="56" />
              <div v-if="S.focusCand === i" class="peek"><b>{{ x.b.title }}</b><span>{{ x.c.hit.slice(0, 3).join(' · ') || x.b.tags.join(' · ') }}</span><span class="num">口味相似 {{ x.c.score.toFixed(2) }}</span></div>
            </div>
          </div>
          <div v-if="S.seg === 'think' || S.seg === 'reveal'" class="thoughts">
            <p v-if="S.thoughts.length === 0" class="quip">{{ S.quip || '我来仔细读一读这 12 本书……' }}</p>
            <TransitionGroup name="fade">
              <div v-for="(t, i) in S.thoughts" :key="t.text" class="th" :class="t.kind">
                <span class="ic" v-html="ICON[KIND_ICON[t.kind] || 'bulb']" /><em>{{ t.kind }}</em><span>{{ t.shown }}<i v-if="t.shown.length < t.text.length && i === S.thoughts.length - 1" class="caret" /></span>
              </div>
            </TransitionGroup>
          </div>
          <div v-if="S.seg === 'reveal'" class="reveal">
            <TransitionGroup name="pop">
              <div v-for="(p, i) in S.revealed" :key="p.book_id" class="rv">
                <b class="rank">{{ i + 1 }}</b>
                <BookCover :id="p.book_id" :title="book(p.book_id)?.title || ''" :cat="catOf(book(p.book_id))" :w="64" />
                <div class="rt"><b>《{{ book(p.book_id)?.title }}》</b><small>{{ p.reason }}</small></div>
                <div class="pct"><div class="bar"><i :style="{ transform: `scaleX(${p.interest / 100})` }" /></div><b class="num"><RollingNumber :value="p.interest" :ms="900" />%</b></div>
                <span v-if="i === 0 && S.stamp" class="st"><Stamp text="为你推荐" /></span>
              </div>
            </TransitionGroup>
          </div>
        </div>
        <div v-else class="waiting"><YxzAvatar state="think" :size="90" /><p>正在调出你的选书记录……</p></div>
      </section>

      <aside class="prof">
        <h3>{{ S.persona ? '元小智猜你是' : S.title }}</h3>
        <div v-if="S.persona" class="persona">“{{ S.persona }}”<span v-if="S.stampPersona" class="ps"><Stamp text="画像" color="var(--calc-hot)" /></span></div>
        <div class="bars" :style="{ height: barRows.length * 44 + 'px' }">
          <div v-for="r in barRows" :key="r.tag" class="br" :style="{ transform: `translateY(${r.rank * 44}px)` }">
            <span class="bt">{{ r.tag }}</span>
            <span class="track"><i :class="{ neg: r.v < 0 }" :style="{ transform: `scaleX(${Math.min(1, Math.abs(r.v))})` }" /></span>
            <span class="bv num">{{ r.v.toFixed(2) }}</span>
            <em v-if="isAgain && S.seg === 'bars' && changeOf(r.tag)" class="chg" :class="{ up: changeOf(r.tag)! > 0 }">{{ changeOf(r.tag)! > 0 ? '+' : '' }}{{ changeOf(r.tag)!.toFixed(2) }}</em>
          </div>
        </div>
        <div class="meta" v-if="tr"><span>体裁：{{ tr.profile.form }}</span><span>篇幅：{{ tr.profile.len_word }}</span></div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.an { position: fixed; inset: 0; z-index: 40; background: var(--calc-bg); color: var(--calc-ink); display: flex; flex-direction: column; }
.an::before { content: ""; position: absolute; inset: 0; pointer-events: none; background: repeating-linear-gradient(0deg, rgba(155, 123, 255, .05) 0 2px, transparent 2px 6px); }
.top { height: 72px; display: flex; align-items: center; gap: 18px; padding: 0 20px; border-bottom: 1px solid rgba(155, 123, 255, .25); position: relative; }
.who { display: flex; align-items: center; gap: 10px; font-size: 20px; --accent: var(--calc-glow); }
.tape { display: flex; gap: 6px; align-items: center; flex: 1; justify-content: center; }
.cell { width: 40px; height: 52px; border-radius: 6px; border: 2px solid rgba(155, 123, 255, .3); display: flex; align-items: center; justify-content: center; opacity: .35; transition: opacity var(--t-mid), transform var(--t-mid); }
.cell.done { opacity: .8; } .cell.on { opacity: 1; border-color: var(--calc-glow); transform: scale(1.15); }
.rw { display: flex; align-items: center; gap: 6px; margin-left: 12px; color: var(--calc-cyan); font-size: 18px; font-weight: 700; }
.rw :deep(svg) { width: 20px; height: 20px; }
.live { font-size: 17px; color: var(--calc-dim); display: flex; align-items: center; gap: 6px; }
.live i { width: 10px; height: 10px; border-radius: 50%; background: var(--calc-hot); animation: blink 1s infinite; }
@keyframes blink { 50% { opacity: .2; } }
.skip { display: flex; align-items: center; gap: 6px; height: 48px; padding: 0 16px; border-radius: 12px; background: rgba(255, 255, 255, .1); color: #fff; font-size: 17px; font-weight: 700; }
.skip :deep(svg) { width: 18px; height: 18px; }
.body { flex: 1; display: flex; min-height: 0; position: relative; }
.main { flex: 1; min-width: 0; padding: 22px; display: flex; flex-direction: column; justify-content: center; overflow: hidden; }
.prof { width: 34%; max-width: 420px; background: var(--calc-panel); padding: 20px 22px; border-left: 1px solid rgba(155, 123, 255, .2); overflow: hidden; }
.prof h3 { font-size: 20px; color: var(--calc-dim); margin-bottom: 8px; }
.persona { font-size: 30px; font-weight: 800; color: #fff; margin-bottom: 12px; position: relative; }
.ps { position: absolute; right: 0; top: -6px; font-size: 14px; }
.bars { position: relative; }
.br { position: absolute; left: 0; right: 0; height: 40px; display: flex; align-items: center; gap: 10px; transition: transform var(--t-heavy) var(--ease); }
.bt { width: 92px; font-size: 18px; font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.track { flex: 1; height: 14px; background: rgba(255, 255, 255, .08); border-radius: 7px; overflow: hidden; }
.track i { display: block; height: 100%; background: linear-gradient(90deg, var(--calc-glow), var(--calc-cyan)); transform-origin: left; transition: transform var(--t-heavy) var(--ease); border-radius: 7px; }
.track i.neg { background: #5a6385; }
.bv { width: 50px; text-align: right; font-size: 16px; color: var(--calc-dim); }
.chg { position: absolute; right: -4px; top: -10px; font-size: 14px; font-style: normal; color: #ff7a7a; font-weight: 800; }
.chg.up { color: #6cf0a2; }
.meta { display: flex; gap: 14px; margin-top: 16px; color: var(--calc-dim); font-size: 17px; }

.rewind { display: flex; flex-direction: column; align-items: center; gap: 20px; animation: slidein .3s var(--ease); }
@keyframes slidein { from { opacity: 0; transform: translateX(60px); } }
.cards { display: flex; gap: 14px; }
.mini { display: flex; flex-direction: column; align-items: center; gap: 6px; width: 104px; opacity: .35; transform: scale(.9); transition: opacity .3s, transform .3s var(--ease); }
.mini small { font-size: 14px; text-align: center; line-height: 1.25; max-height: 36px; overflow: hidden; }
.mini.chosen { opacity: 1; transform: scale(1.1); }
.mini.chosen :deep(.cv) { box-shadow: 0 0 0 4px var(--calc-glow); }
.obs { display: flex; gap: 12px; min-height: 40px; flex-wrap: wrap; justify-content: center; }
.ob { background: rgba(63, 215, 232, .14); color: var(--calc-cyan); padding: 6px 14px; border-radius: 10px; font-size: 18px; font-weight: 700; animation: slidein .3s var(--ease) both; }
.fly { position: relative; display: flex; gap: 8px; flex-wrap: wrap; justify-content: center; min-height: 40px; }
.ft { padding: 4px 12px; border-radius: 9px; background: rgba(155, 123, 255, .25); color: #fff; font-weight: 700; font-size: 17px; animation: fly 1.1s var(--ease) .1s both; }
.ft.neg { background: transparent; color: var(--calc-dim); font-size: 15px; }
@keyframes fly { 0% { opacity: 0; transform: translateY(10px); } 30% { opacity: 1; transform: none; } 100% { opacity: 0; transform: translate(260px, -120px) scale(.7); } }

.scan { display: flex; flex-direction: column; gap: 26px; align-items: center; }
.shelf { position: relative; display: grid; grid-template-columns: repeat(30, 1fr); gap: 4px 3px; width: 100%; max-width: 760px; overflow: hidden; padding: 6px 0; }
.sp { height: 44px; border-radius: 2px; background: var(--c); opacity: .55; transition: opacity .25s var(--d), transform .25s var(--d), background-color .25s var(--d); }
.go .sp.hot { background: var(--calc-hot); opacity: 1; transform: translateY(-5px); }
.go .sp.warm { background: #e7b46a; opacity: .9; }
.go .sp.zero { background: #59607c; opacity: .6; }
.go .sp.neg { background: #2a3150; opacity: .5; }
.band { position: absolute; top: 0; bottom: 0; left: 0; width: 70px; background: linear-gradient(90deg, transparent, rgba(155, 123, 255, .7), transparent); transform: translateX(-80px); }
.go .band { animation: band 1.8s linear forwards; }
@keyframes band { to { transform: translateX(800px); } }
.ctr { display: flex; align-items: baseline; gap: 14px; font-size: 20px; color: var(--calc-dim); flex-wrap: wrap; justify-content: center; }
.ctr b { font-size: 40px; color: var(--calc-cyan); margin-right: 4px; }
.ctr .hot b { color: var(--calc-hot); }
.ctr .dim { width: 100%; text-align: center; font-size: 17px; }

.flow { display: flex; flex-direction: column; gap: 16px; height: 100%; justify-content: flex-start; padding-top: 4px; }
.funnel { display: flex; flex-direction: column; align-items: center; gap: 8px; }
.fl { width: var(--w); min-height: 48px; display: flex; align-items: center; gap: 16px; padding: 0 18px; border-radius: 10px; background: rgba(155, 123, 255, .16); opacity: 0; transform: scaleX(.6); transition: opacity .3s, transform .35s var(--ease); }
.fl.on { opacity: 1; transform: none; }
.fl b { font-size: 30px; color: var(--calc-cyan); width: 70px; text-align: right; }
.fl span { font-size: 18px; }
.cands { display: flex; flex-wrap: wrap; gap: 10px; justify-content: center; position: relative; }
.cd { position: relative; transition: transform .3s var(--ease), opacity .3s; opacity: .7; }
.cd.focus { opacity: 1; transform: translateY(-8px) scale(1.15); z-index: 2; }
.peek { position: absolute; left: 50%; top: 100%; transform: translateX(-50%); margin-top: 10px; width: 210px; background: var(--calc-panel); border: 1px solid var(--calc-glow); border-radius: 10px; padding: 8px 10px; display: flex; flex-direction: column; font-size: 15px; gap: 2px; }
.peek b { font-size: 17px; } .peek span { color: var(--calc-cyan); }
.thoughts { display: flex; flex-direction: column; gap: 10px; margin-top: 8px; }
.quip { color: var(--calc-dim); font-size: 20px; text-align: center; }
.th { display: flex; align-items: flex-start; gap: 10px; font-size: 21px; line-height: 1.45; background: rgba(255, 255, 255, .04); padding: 8px 14px; border-radius: 10px; }
.th .ic { width: 26px; height: 26px; flex: none; margin-top: 2px; }
.th .ic :deep(svg) { width: 100%; height: 100%; }
.th em { font-style: normal; font-size: 15px; font-weight: 800; padding: 1px 8px; border-radius: 6px; margin-top: 4px; flex: none; }
.th.证据 .ic, .th.证据 em { color: var(--calc-cyan); } .th.推断 .ic, .th.推断 em { color: var(--calc-glow); }
.th.排除 .ic, .th.排除 em { color: #98a0bb; } .th.发现 .ic, .th.发现 em { color: var(--calc-hot); } .th.结论 .ic, .th.结论 em { color: #6cf0a2; }
.th em { background: rgba(255, 255, 255, .08); }
.caret { display: inline-block; width: 2px; height: 1em; background: var(--calc-cyan); vertical-align: -2px; margin-left: 2px; animation: blink .6s infinite; }
.reveal { display: flex; flex-direction: column; gap: 10px; margin-top: 6px; }
.rv { display: flex; align-items: center; gap: 14px; background: rgba(255, 255, 255, .06); border-radius: 12px; padding: 10px 14px; position: relative; }
.rank { font-size: 30px; color: var(--calc-hot); width: 30px; }
.rt { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.rt b { font-size: 21px; } .rt small { font-size: 16px; color: var(--calc-dim); }
.pct { width: 150px; display: flex; align-items: center; gap: 8px; }
.pct .bar { flex: 1; height: 12px; background: rgba(255, 255, 255, .1); border-radius: 6px; overflow: hidden; }
.pct .bar i { display: block; height: 100%; background: linear-gradient(90deg, var(--calc-glow), var(--calc-hot)); transform-origin: left; animation: grow .9s var(--ease) both; }
@keyframes grow { from { transform: scaleX(0); } }
.pct b { font-size: 24px; color: var(--calc-hot); width: 62px; text-align: right; }
.st { position: absolute; right: 170px; top: 4px; font-size: 16px; }
.pop-enter-active { transition: transform .5s var(--ease), opacity .5s; }
.pop-enter-from { opacity: 0; transform: rotateX(80deg) translateY(-10px); }
.struck { display: flex; gap: 18px; justify-content: center; align-items: flex-end; flex-wrap: wrap; }
.struck .mini { opacity: 1; transform: none; position: relative; }
.struck .mini.x { opacity: .3; transform: scale(.85); }
.struck .mini.x::after { content: ""; position: absolute; left: -6px; right: -6px; top: 45%; height: 4px; background: #ff6b6b; transform: rotate(-20deg); }
.struck .mini.like :deep(.cv) { box-shadow: 0 0 0 4px #6cf0a2; }
.said { width: 100%; text-align: center; color: var(--calc-cyan); font-size: 20px; }
.waiting { text-align: center; color: var(--calc-dim); font-size: 20px; --accent: var(--calc-glow); }
@media (max-width: 900px) {
  .prof { display: none; }
  .tape { display: none; }
}
@media (orientation: portrait) {
  .body { flex-direction: column; }
  .prof { display: block; width: auto; max-width: none; border-left: 0; border-top: 1px solid rgba(155, 123, 255, .2); max-height: 38%; }
}
</style>
