<script setup lang="ts">
/* 计算环节 · 教室大屏：V0 书库墙 / V1 全班进度 / V2 投屏回放 / V3 全班汇总 */
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import ScreenStage from '../common/components/ScreenStage.vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import BookCover from '../common/components/BookCover.vue'
import RollingNumber from '../common/components/RollingNumber.vue'
import Analysis from './Analysis.vue'
import { events, get, post } from '../common/api'
import { toast } from '../common/toast'
import Toasts from '../common/components/Toasts.vue'
import { META, catOf, newFeed, type Feed } from './types'

const view = ref<'V0' | 'V1' | 'V2' | 'V3'>('V0')
const S = reactive<any>({ seats: [], all_seats: [], taste: {}, picks: 0, ops: 0, compute_ms: 0, interests: [], stars: [0, 0, 0, 0, 0, 0], top_books: [], books: {}, paused: false, mode: 'live', effort: 'low', show_draft: true, warm: '' })
const seatMap = computed(() => Object.fromEntries((S.seats as any[]).map((s) => [s.seat, s])))
const analyzing = reactive<Record<number, boolean>>({})
let off: (() => void) | null = null
let tmr = 0

async function load() { Object.assign(S, await get('/api/jisuan/screen/state')) }
function soon() { clearTimeout(tmr); tmr = window.setTimeout(load, 400) }
onMounted(async () => {
  const m = await get('/api/jisuan/meta')
  META.cats = m.categories; META.spines = m.spines; META.books = m.books; META.tags = m.tags; META.loaded = true
  await load()
  off = events('jisuan', 'screen', {
    seat_progress: (d) => { analyzing[d.seat] = d.stage === 'analyzing' || d.stage === 'again'; soon() },
    taste_update: (d) => { for (const t of d.tags) S.taste[t] = (S.taste[t] || 0) + 1; S.picks++ },
    analysis_done: (d) => { analyzing[d.seat] = false; soon() },
    stars: soon, mode: (d) => (S.mode = d.mode), reset: load,
    draft: (d) => { if (view.value === 'V2' && d.seat === v2.seat && v2.live) v2.draft += d.text },
  })
  requestAnimationFrame(drawWall)
})
onBeforeUnmount(() => { off && off(); cancelAnimationFrame(wallRaf) })

// ---------------- V0 书库墙（Canvas） ----------------
const wall = ref<HTMLCanvasElement>()
let wallRaf = 0
function drawWall(t: number) {
  const c = wall.value
  if (c && view.value === 'V0') {
    const g = c.getContext('2d')!
    const W = c.width, H = c.height, n = META.spines.length || 301
    const cols = 43, rows = Math.ceil(n / cols), sw = W / cols, sh = H / rows
    g.clearRect(0, 0, W, H)
    for (let i = 0; i < n; i++) {
      const col = i % cols, row = Math.floor(i / cols)
      const cat = META.cats[META.spines[i]]
      const breathe = 0.72 + 0.28 * Math.sin(t / 900 + col * 0.35 + row * 0.8)
      const h = sh * (0.7 + ((i * 37) % 23) / 100)
      g.globalAlpha = breathe
      g.fillStyle = cat ? cat.color : '#888'
      g.fillRect(col * sw + 3, row * sh + (sh - h) - 10, sw - 6, h)
      g.globalAlpha = 1
      g.fillStyle = 'rgba(255,255,255,.25)'
      g.fillRect(col * sw + 3, row * sh + (sh - h) - 10, 4, h)
      if (col === 0) { g.fillStyle = '#6b5a45'; g.fillRect(0, row * sh + sh - 10, W, 8) }
    }
  }
  wallRaf = requestAnimationFrame(drawWall)
}

// ---------------- V1 ----------------
const tasteList = computed(() => {
  const e = Object.entries(S.taste as Record<string, number>).sort((a, b) => b[1] - a[1]).slice(0, 18)
  const max = Math.max(1, ...e.map((x) => x[1]))
  return e.map(([t, n], i) => ({ t, n, r: 50 + 80 * Math.sqrt(n / max), c: ['#e0442f', '#2a73c9', '#149a58', '#e89414', '#8a55d6', '#11a5b5', '#ec5a9e', '#8a5a3c', '#5b6b73', '#c2a600', '#3b4cc0', '#2e8b57'][i % 12] }))
})
const doneCount = computed(() => (S.seats as any[]).filter((s) => s.top).length)

// ---------------- V2 投屏回放 ----------------
const v2 = reactive({ seat: 0, attempt: 0, attempts: 1, feed: null as Feed | null, key: 0, draft: '', shownDraft: '', live: false, fallback: false, which: 'first' as 'first' | 'again', hasAgain: false })
let draftT = 0
async function openSeat(seat: number, attempt = 0, which: 'first' | 'again' = 'first') {
  if (!seatMap.value[seat]) { toast(`${seat} 号平板还没有记录`, 'warn'); return }
  const d = await get(`/api/jisuan/screen/trace?seat=${seat}${attempt ? '&attempt=' + attempt : ''}`)
  if (!d.trace) { toast(`${seat} 号平板还没分析完`, 'warn'); return }
  const tr = which === 'again' && d.again_trace ? d.again_trace : d.trace
  const rs = which === 'again' && d.again ? d.again : d.result
  const f = newFeed()
  f.trace = tr
  for (const t of tr.llm?.thoughts || []) f.thoughts.push(t)
  if (rs) { f.persona = { name: rs.persona, tags: rs.top_tags }; f.picks.push(...rs.picks); f.final = rs }
  f.done = true
  Object.assign(v2, { seat, attempt: d.attempt, attempts: d.attempts, feed: f, draft: tr.llm?.draft || '', shownDraft: '', fallback: !!d.fallback, which, hasAgain: !!d.again_trace })
  v2.key++
  view.value = 'V2'
  post('/api/jisuan/screen/control', { action: 'project', seat })
  clearInterval(draftT)
  let i = 0
  draftT = window.setInterval(() => {
    if (i >= v2.draft.length) return
    i = Math.min(v2.draft.length, i + 18)
    v2.shownDraft = v2.draft.slice(Math.max(0, i - 1600), i)
    nextTick(() => { const el = document.querySelector('.draft pre'); if (el) el.scrollTop = el.scrollHeight })
  }, 60)
}
onBeforeUnmount(() => clearInterval(draftT))

// ---------------- V3 ----------------
const avgStars = computed(() => { const s = S.stars as number[]; let n = 0, t = 0; s.forEach((c, k) => { n += c; t += c * k }); return n ? t / n : 0 })
const starN = computed(() => (S.stars as number[]).reduce((a, b) => a + b, 0))
const bins = computed(() => {
  const b = [0, 0, 0, 0, 0, 0] // 50-59 60-69 70-79 80-89 90-97 100
  for (const v of S.interests as number[]) b[v >= 100 ? 5 : Math.min(4, Math.floor((v - 50) / 10))]++
  return b
})
const binMax = computed(() => Math.max(1, ...bins.value))

// ---------------- 控制 ----------------
async function ctl(action: string, extra: any = {}) {
  const d = await post('/api/jisuan/screen/control', { action, ...extra })
  S.paused = d.paused; S.effort = d.effort; S.show_draft = d.show_draft
  if (action === 'start') { toast('已开始，正在预热', 'ok'); setTimeout(load, 3000) }
  if (action === 'reset') { await load(); view.value = 'V1' }
}
function resetConfirm() { if (confirm('确定重置计算环节？所有平板的选书记录都会清空。')) ctl('reset') }
const order: typeof view.value[] = ['V0', 'V1', 'V2', 'V3']
function onKey(k: string) {
  if (k === 'next') view.value = order[Math.min(3, order.indexOf(view.value) + 1)]
  else if (k === 'prev') view.value = order[Math.max(0, order.indexOf(view.value) - 1)]
  else if (k === 'home') view.value = 'V1'
  else if (k === 'replay' && view.value === 'V2') openSeat(v2.seat, v2.attempt, v2.which)
  else if (k.startsWith('seat:')) openSeat(Number(k.slice(5)))
}
watch(view, (v) => { if (v === 'V0') requestAnimationFrame(() => {}) })
</script>

<template>
  <ScreenStage :mode="S.mode" lock="jisuan" @key="onKey">
    <Toasts />
    <YxzHeader title="元小智 · 计算" link="compute" big :state="Object.values(analyzing).some(Boolean) ? 'think' : 'idle'">
      <span v-if="view === 'V2'">{{ v2.seat }} 号平板的分析过程</span>
      <span v-else>智能图书管理员</span>
    </YxzHeader>

    <!-- V0 -->
    <section v-show="view === 'V0'" class="v0">
      <h1>元小智的书库：<b class="num">{{ META.books }}</b> 本书 · <b class="num">{{ META.tags }}</b> 种标签</h1>
      <canvas ref="wall" width="1760" height="620" />
      <div class="legend"><span v-for="c in META.cats" :key="c.name"><i :style="{ background: c.color }" />{{ c.name }}</span></div>
    </section>

    <!-- V1 -->
    <section v-if="view === 'V1'" class="v1">
      <div class="counters">
        <div>全班已做出 <b><RollingNumber :value="S.picks" /></b> 次选择</div>
        <div>元小智已比对 <b class="hot"><RollingNumber :value="S.ops" :ms="1500" /></b> 次</div>
        <div>已完成 <b><RollingNumber :value="doneCount" /></b> 台</div>
      </div>
      <div class="v1b">
        <div class="grid">
          <button v-for="a in S.all_seats" :key="a.seat" class="cell" :class="{ on: seatMap[a.seat], spin: analyzing[a.seat], done: seatMap[a.seat]?.top }" @click="openSeat(a.seat)">
            <b class="sn num">{{ a.seat }}</b>
            <span v-if="seatMap[a.seat]?.attempt > 1" class="tries">第 {{ seatMap[a.seat].attempt }} 次</span>
            <span v-if="seatMap[a.seat]?.differ" class="diff">两次不一样</span>
            <div v-if="seatMap[a.seat]?.top" class="top">
              <BookCover :id="seatMap[a.seat].top.id" :title="seatMap[a.seat].top.title" :cat="catOf(seatMap[a.seat].top)" :w="44" />
              <small>{{ seatMap[a.seat].persona }}</small>
            </div>
            <div v-else class="pips"><i v-for="k in 5" :key="k" :class="{ lit: k <= (seatMap[a.seat]?.picked || 0) }" /></div>
            <span v-if="seatMap[a.seat]?.stars" class="stars num">★ {{ seatMap[a.seat].stars }}</span>
          </button>
        </div>
        <div class="taste">
          <h2>全班阅读口味</h2>
          <TransitionGroup name="bub" tag="div" class="bubs">
            <span v-for="b in tasteList" :key="b.t" class="bub" :style="{ width: b.r + 'px', height: b.r + 'px', background: b.c, fontSize: Math.max(20, b.r / 4.2) + 'px' }">{{ b.t }}<small class="num">{{ b.n }}</small></span>
          </TransitionGroup>
          <p v-if="!tasteList.length" class="muted">等同学们开始选书……</p>
        </div>
      </div>
    </section>

    <!-- V2 -->
    <section v-if="view === 'V2'" class="v2">
      <div v-if="v2.feed" class="v2s">
        <div class="v2in"><Analysis :key="v2.key" :feed="v2.feed" replay @finish="() => {}" /></div>
        <div v-if="S.show_draft" class="draft">
          <h4>元小智的草稿纸</h4>
          <pre v-if="v2.draft">{{ v2.shownDraft }}</pre>
          <p v-else>这一次元小智没写草稿，直接作答</p>
        </div>
        <div class="v2bar">
          <button v-for="k in v2.attempts" :key="k" :class="{ on: v2.attempt === k && v2.which === 'first' }" @click="openSeat(v2.seat, k)">第 {{ k }} 次</button>
          <button v-if="v2.hasAgain" :class="{ on: v2.which === 'again' }" @click="openSeat(v2.seat, v2.attempt, 'again')">再想想</button>
          <button @click="openSeat(v2.seat, v2.attempt, v2.which)">重播</button>
          <i v-if="v2.fallback" class="fb" title="本次用了本地算法" />
        </div>
      </div>
      <p v-else class="pick-hint">在全班进度里点一个格子，或按平板号数字键</p>
    </section>

    <!-- V3 -->
    <section v-if="view === 'V3'" class="v3">
      <div class="panel">
        <h3>猜得准吗？</h3>
        <p class="big num">{{ avgStars.toFixed(1) }} <small>/ 5 星 · {{ starN }} 人评分</small></p>
        <div class="hbar" v-for="k in [5, 4, 3, 2, 1]" :key="k"><span>{{ k }} 星</span><i :style="{ width: (S.stars[k] / Math.max(1, starN)) * 100 + '%' }" /><b class="num">{{ S.stars[k] }}</b></div>
      </div>
      <div class="panel">
        <h3>感兴趣程度的分布</h3>
        <div class="hist">
          <div v-for="(n, i) in bins" :key="i" class="col"><b class="num">{{ n }}</b><i :style="{ height: (n / binMax) * 260 + 'px' }" :class="{ zero: i === 5 }" /><span>{{ ['50+', '60+', '70+', '80+', '90+', '100%'][i] }}</span></div>
        </div>
        <p class="note">没有一本是 100%：元小智是在“猜”</p>
      </div>
      <div class="panel">
        <h3>全班被推荐最多的书</h3>
        <div class="tb" v-for="(t, i) in S.top_books" :key="t.id"><b>{{ i + 1 }}</b><BookCover :id="t.id" :title="S.books[t.id]?.title" :cat="catOf(S.books[t.id])" :w="50" /><span>{{ S.books[t.id]?.title }}</span><em class="num">{{ t.n }} 次</em></div>
      </div>
      <div class="sum">
        <div class="sc"><h4>数据</h4><p>全班 <b class="num">{{ S.picks }}</b> 次选择</p><small>你的每次选择、用时、看没看简介</small></div>
        <div class="sc"><h4>算法</h4><p>标签打分 → 筛选 → 推理</p><small>{{ META.books }} 本 → 12 本 → 3 本</small></div>
        <div class="sc"><h4>算力</h4><p>共比对 <b class="num">{{ (S.ops / 10000).toFixed(1) }}</b> 万次</p><small>本地计算只用了 {{ (S.compute_ms / 1000).toFixed(3) }} 秒</small></div>
      </div>
    </section>

    <template #controls>
      <button @click="ctl('start')">开始本环节</button>
      <span class="sep" />
      <button v-for="v in order" :key="v" :class="{ on: view === v }" @click="view = v">{{ { V0: '书库墙', V1: '全班进度', V2: '投屏回放', V3: '全班汇总' }[v] }}</button>
      <span class="sep" />
      <button :class="{ on: S.paused }" @click="ctl(S.paused ? 'resume' : 'pause')">{{ S.paused ? '继续学生操作' : '暂停学生操作' }}</button>
      <button :class="{ on: S.effort === 'medium' }" @click="ctl('effort', { effort: S.effort === 'medium' ? 'low' : 'medium' })">思考：{{ S.effort === 'medium' ? '深度' : '快速' }}</button>
      <button :class="{ on: S.show_draft }" @click="ctl('draft')">草稿纸</button>
      <button class="warn" @click="resetConfirm">重置本环节</button>
      <small style="opacity:.6;font-size:18px">{{ S.warm }}</small>
    </template>
  </ScreenStage>
</template>

<style scoped>
section { position: absolute; left: 0; right: 0; top: 104px; bottom: 0; padding: 30px 60px; }
.v0 { display: flex; flex-direction: column; align-items: center; gap: 26px; }
.v0 h1 { font-size: 60px; }
.v0 h1 b { color: var(--accent); }
.v0 canvas { width: 1760px; height: 620px; background: linear-gradient(#f2ebdf, #e9dfcf); border-radius: 18px; }
.legend { display: flex; gap: 26px; flex-wrap: wrap; justify-content: center; font-size: 26px; }
.legend i { display: inline-block; width: 22px; height: 22px; border-radius: 5px; margin-right: 8px; vertical-align: -3px; }
.counters { display: flex; gap: 60px; font-size: 34px; margin-bottom: 12px; }
.counters b { font-size: 64px; color: var(--accent); margin: 0 6px; }
.counters .hot { color: #e2702c; }
.v1b { display: flex; gap: 40px; }
.grid { display: grid; grid-template-columns: repeat(6, 178px); grid-auto-rows: 136px; gap: 12px; }
.cell { position: relative; border-radius: 18px; background: #ece8e0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; color: #9aa4a6; }
.cell.on { background: #fff; color: var(--ink); box-shadow: var(--shadow); }
.cell.spin::before { content: ""; position: absolute; inset: -4px; border-radius: 20px; border: 4px solid transparent; border-top-color: var(--accent); border-right-color: var(--accent); animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.cell.done { background: var(--accent-soft); }
.sn { position: absolute; left: 10px; top: 4px; font-size: 26px; }
.tries { position: absolute; right: 8px; top: 6px; font-size: 17px; color: var(--accent); font-weight: 700; }
.diff { position: absolute; right: 6px; bottom: 6px; font-size: 15px; background: var(--amber); color: #fff; border-radius: 8px; padding: 0 8px; }
.pips { display: flex; gap: 6px; margin-top: 16px; }
.pips i { width: 22px; height: 30px; border-radius: 5px; background: #ddd6ca; }
.pips i.lit { background: var(--accent); }
.top { display: flex; align-items: center; gap: 8px; margin-top: 14px; }
.top small { font-size: 19px; font-weight: 700; color: var(--accent); max-width: 100px; line-height: 1.2; }
.stars { position: absolute; left: 10px; bottom: 4px; font-size: 18px; color: var(--amber); }
.taste { flex: 1; }
.taste h2 { font-size: 40px; margin-bottom: 16px; }
.bubs { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; align-content: flex-start; }
.bub { border-radius: 50%; color: #fff; font-weight: 800; display: flex; flex-direction: column; align-items: center; justify-content: center; line-height: 1.1; transition: width .6s var(--ease), height .6s var(--ease); text-align: center; }
.bub small { font-size: 20px; opacity: .85; }
.bub-move { transition: transform .6s var(--ease); }
.v2 { padding: 0; }
.v2s { position: absolute; inset: 0; }
.v2in { position: absolute; left: 0; top: 0; width: 1280px; height: 651px; transform: scale(1.5); transform-origin: 0 0; overflow: hidden; }
.draft { position: absolute; left: 330px; bottom: 20px; width: 700px; height: 230px; background: rgba(10, 14, 28, .82); border: 1px solid rgba(155, 123, 255, .5); border-radius: 14px; padding: 12px 16px; color: #b7c0e6; z-index: 5; display: flex; flex-direction: column; }
.draft h4 { font-size: 24px; color: #fff; margin: 0 0 6px; }
.draft pre { flex: 1; margin: 0; overflow: hidden; white-space: pre-wrap; font-size: 18px; line-height: 1.4; opacity: .75; font-family: var(--font); }
.draft p { font-size: 22px; }
.v2bar { position: absolute; left: 30px; bottom: 24px; display: flex; gap: 12px; z-index: 5; }
.v2bar button { min-height: 56px; padding: 0 22px; border-radius: 12px; background: rgba(255, 255, 255, .14); color: #fff; font-size: 24px; font-weight: 700; }
.v2bar button.on { background: var(--calc-glow); }
.fb { width: 12px; height: 12px; border-radius: 50%; background: #f0b400; align-self: center; opacity: .7; }
.pick-hint { font-size: 40px; text-align: center; margin-top: 300px; color: var(--muted); }
.v3 { display: grid; grid-template-columns: 1fr 1.2fr 1fr; grid-template-rows: 1fr auto; gap: 26px; }
.panel { background: #fff; border-radius: 22px; padding: 26px 30px; box-shadow: var(--shadow); }
.panel h3 { font-size: 36px; margin-bottom: 14px; }
.big { font-size: 96px; font-weight: 800; color: var(--accent); line-height: 1.1; }
.big small { font-size: 26px; color: var(--muted); font-weight: 500; }
.hbar { display: flex; align-items: center; gap: 12px; font-size: 26px; margin-top: 10px; }
.hbar span { width: 64px; } .hbar i { height: 26px; background: var(--amber); border-radius: 6px; min-width: 4px; } .hbar b { width: 40px; }
.hist { display: flex; align-items: flex-end; gap: 22px; height: 330px; justify-content: center; }
.col { display: flex; flex-direction: column; align-items: center; gap: 8px; font-size: 24px; }
.col i { width: 74px; background: var(--accent); border-radius: 8px 8px 0 0; min-height: 4px; }
.col i.zero { background: #ccc; }
.note { text-align: center; font-size: 28px; color: var(--accent); margin-top: 10px; font-weight: 700; }
.tb { display: flex; align-items: center; gap: 16px; font-size: 28px; margin-bottom: 12px; }
.tb b { width: 30px; color: var(--accent); } .tb span { flex: 1; } .tb em { font-style: normal; color: var(--muted); font-size: 24px; }
.sum { grid-column: 1 / 4; display: flex; gap: 26px; }
.sc { flex: 1; background: var(--calc-bg); color: var(--calc-ink); border-radius: 22px; padding: 22px 30px; }
.sc h4 { font-size: 30px; color: var(--calc-cyan); margin: 0; }
.sc p { font-size: 38px; font-weight: 800; }
.sc b { color: var(--calc-hot); }
.sc small { font-size: 24px; color: var(--calc-dim); }
</style>
