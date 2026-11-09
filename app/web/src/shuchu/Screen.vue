<script setup lang="ts">
/* 输出环节 · 教室大屏：V0 任务发布 / V1 提交状态 / V2 规划器慢放 / V3 作品墙 / V4 发布会验收 / V5 问 5 次 */
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import LockToggle from '../common/components/LockToggle.vue'
import ScreenStage from '../common/components/ScreenStage.vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import YxzAvatar from '../common/components/YxzAvatar.vue'
import Stamp from '../common/components/Stamp.vue'
import Toasts from '../common/components/Toasts.vue'
import PlanMap from './PlanMap.vue'
import { events, get, post } from '../common/api'
import { toast } from '../common/toast'
import { routeJpeg, routeSvg } from './map/render'
import './map/map.css'
import type { PlanTrace, RoadNode } from './types'

type V = 'V0' | 'V1' | 'V2' | 'V3' | 'V4' | 'V5'
const view = ref<V>('V0')
const S = reactive<any>({ groups: [], plan: null, five: null, tally: { same: 0, diff: 0 }, mode: 'live', phase: 1 })
async function setPhase(n: number) { await post('/api/shuchu/screen/control', { action: 'phase', phase: n }); S.phase = n }
const nodes = ref<RoadNode[]>([])
const edges = ref<any[]>([])
const nodeName = (id: string) => nodes.value.find((n) => n.id === id)?.name || id
const CARDS = ['位置人数', '楼梯', '道路', '拥堵', '集合点', '安全']
let off: (() => void) | null = null
async function load() { Object.assign(S, await get('/api/shuchu/screen/state')) }
onMounted(async () => {
  const g = await get('/api/shuchu/graph')
  nodes.value = g.nodes; edges.value = g.edges
  await load()
  off = events('shuchu', 'screen', {
    group_update: (d) => upd(d), gallery_update: (d) => upd(d),
    plan_trace: (d) => { if (view.value !== 'V2' || pm.step === 0) S.plan = d },
    verdict: () => load(), phase: (d) => (S.phase = d.phase), vote_update: (d) => (S.tally = d), mode: (d) => (S.mode = d.mode), reset: load,
    five_item: (d) => addFive(d), five_done: () => (five.running = false),
  })
})
onBeforeUnmount(() => off && off())
function upd(d: any) { const i = S.groups.findIndex((g: any) => g.group === d.group); if (i >= 0) S.groups[i] = d }
async function ctl(action: string, extra: any = {}) {
  try { return await post('/api/shuchu/screen/control', { action, ...extra }) } catch (e) { toast((e as Error).message, 'warn') }
}

// ---------------- V2 规划器慢放 ----------------
const pm = reactive({ step: 0, round: 0, flash: 0, paused: false, msg: '', playing: false })
let resume: (() => void) | null = null
const wait = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))
const holdUntilContinue = () => new Promise<void>((r) => { pm.paused = true; resume = () => { pm.paused = false; r() } })
async function runPlan() {
  if (pm.playing) return
  const tr: PlanTrace = await ctl('run_plan')
  if (!tr) return
  S.plan = tr
  view.value = 'V2'
  pm.playing = true
  pm.step = 0; pm.round = 0; pm.msg = '这是校园平面图'
  await wait(1200)
  pm.step = 1; pm.msg = '①平面图变成道路网：节点和道路'
  await wait(2200)
  pm.step = 2
  for (const g of tr.groups) { pm.flash = g.group; pm.msg = `②${g.class}班的候选路线`; await wait(1300) }
  pm.step = 3; pm.round = 0; pm.msg = '③6 个班一起放进去，楼梯口按人数变色'
  await wait(2200)
  if (tr.first_red === 0) { pm.msg = `这里要堵了！${nodeName(tr.red_stair)}变红。你是元小智，会让哪个班换楼梯？`; await holdUntilContinue() }
  pm.step = 4
  for (let k = 1; k < tr.rounds.length; k++) {
    pm.round = k
    pm.msg = '⑤调整：' + (tr.rounds[k].moved?.why || '')
    await wait(2600)
    if (tr.first_red === k) { pm.msg = `这里要堵了！${nodeName(tr.red_stair)}变红。你会怎么调整？`; await holdUntilContinue() }
  }
  pm.step = 5; pm.msg = '⑥定稿：每个班的推荐路线'
  pm.playing = false
}
function cont() { resume && resume(); resume = null }
const planRows = computed(() => {
  const tr: PlanTrace | null = S.plan
  if (!tr) return []
  const r = tr.rounds[Math.min(pm.round, tr.rounds.length - 1)]
  return tr.groups.map((g) => {
    const st = r.assign[g.group]
    const l = r.loads[st]
    const notes: string[] = []
    if (!g.known) notes.push('没给位置：按真实情况算人数')
    else {
      if (!g.info.Crowd) notes.push('没给④：没考虑拥堵')
      if (!g.info.Roads) notes.push('没给③：直线穿楼')
      if (!g.info.Stairs) notes.push('没给②：猜楼梯')
      if (!g.info.Goal) notes.push('没给⑤：终点不对')
    }
    return { g: g.group, cls: g.class, stair: nodeName(st), color: l?.color, total: r.times[g.group], notes }
  })
})

// ---------------- V3 / V4 ----------------
const wallVer = reactive<Record<number, number>>({})
const verOf = (g: any) => { const n = wallVer[g.group] || g.versions.length; return g.versions[n - 1] }
const sel = reactive({ group: 1, version: 0, showSpec: false })
const selG = computed(() => S.groups.find((g: any) => g.group === sel.group))
const selV = computed(() => selG.value?.versions[(sel.version || selG.value.versions.length) - 1])
const selSvg = computed(() => (selV.value?.spec ? routeSvg(selV.value.spec, nodes.value, { issues: selV.value.issues?.filter((i: any) => !i.warn) }) : ''))
function openCheck(g: number, v = 0) { sel.group = g; sel.version = v; view.value = 'V4' }
async function verdict(ok: boolean) {
  await ctl(ok ? 'approve' : 'reject', { group: sel.group, version: selV.value.n, seat: selV.value.seat })
  await load()
}

// ---------------- V5 问 5 次 ----------------
const five = reactive({ group: 1, items: [] as any[], imgs: {} as Record<number, string>, running: false, voting: false })
async function openVote() { five.voting = true; five.items = []; five.imgs = {}; await ctl('vote_open', { group: five.group }) }
async function startFive() {
  five.voting = false; five.items = []; five.imgs = {}; five.running = true
  await ctl('five_start', { group: five.group })
}
async function addFive(d: any) {
  five.items.push(d)
  five.imgs[d.i] = await routeJpeg(d.spec, nodes.value, 900).catch(() => '')
}
const stats = computed(() => {
  const st: Record<string, number> = {}, col = new Set<string>(), mk = new Set<string>()
  let fail = 0
  for (const it of five.items) {
    st[it.stair || '—'] = (st[it.stair || '—'] || 0) + 1
    col.add(it.spec.line.color + '/' + it.spec.line.style)
    for (const m of it.spec.markers) mk.add(m.type)
    if (it.issues.some((i: any) => !i.warn)) fail++
  }
  return { st, colors: col.size, markers: mk.size, fail }
})
const allSame = computed(() => five.items.length === 5 && new Set(five.items.map((i) => JSON.stringify(i.spec))).size === 1)

function resetAll() { if (window.confirm('重置输出环节？')) ctl('reset').then(load) }
const order: V[] = ['V0', 'V1', 'V2', 'V3', 'V4', 'V5']
const NAMES: Record<V, string> = { V0: '任务发布', V1: '提交状态', V2: '规划器', V3: '作品墙', V4: '验收', V5: '问 5 次' }
function onKey(k: string) {
  if (k === 'pause' && view.value === 'V2') cont()
  else if (k === 'next') view.value = order[Math.min(5, order.indexOf(view.value) + 1)]
  else if (k === 'prev') view.value = order[Math.max(0, order.indexOf(view.value) - 1)]
  else if (k === 'home') view.value = 'V1'
  else if (k === 'replay' && view.value === 'V2') runPlan()
  else if (k.startsWith('seat:')) { const g = Number(k.slice(5)); if (g >= 1 && g <= 6) openCheck(g) }
}
</script>

<template>
  <ScreenStage :mode="S.mode" @key="onKey">
    <Toasts />
    <YxzHeader title="元小智 · 输出" link="output" big :state="pm.playing || five.running ? 'think' : 'idle'"><template #after><LockToggle mod="shuchu" /></template>疏散路线图发布会</YxzHeader>

    <section v-if="view === 'V0'" class="v0">
      <div class="task card">
        <span class="badge">消防队长的任务</span>
        <h1>11 月 9 日 · 全国消防日</h1>
        <p>每个班门口都要贴一张<b>疏散路线图</b>。</p>
        <p>每组负责六年级的一个班，拿到一张能<b>通过消防队长验收</b>的路线图。</p>
        <p class="std">验收标准：沿道路走 · 楼梯走得对 · 到达集合点 · 方向一看就懂 · 符合“给谁看”</p>
      </div>
      <div class="demo">
        <div class="q">元小智，请为六年级规划疏散路线图！</div>
        <div class="a"><YxzAvatar state="speak" :size="90" /><p>好的！不过我需要知道：各班在哪栋楼、几楼？楼梯在哪里？集合点在哪里？每个班多少人？</p></div>
      </div>
    </section>

    <section v-if="view === 'V1'" class="v1">
      <div v-for="g in S.groups" :key="g.group" class="gc card" :class="{ busy: g.busy }">
        <h2>第 {{ g.group }} 组 · {{ g.class }}班</h2>
        <div class="cs"><span v-for="(n, i) in g.counts" :key="i" class="cp" :class="{ on: n > 0 }">{{ '①②③④⑤⑥'[i] }} {{ CARDS[i] }}<em v-if="n" class="num"> {{ n }}</em></span></div>
        <div class="tabs"><span v-for="t in g.tablets" :key="t.seat" class="tb" :class="{ got: t.versions, busy: t.busy }">{{ t.seat }}</span></div>
        <p class="st">{{ g.busy ? `${g.busy} 台正在请元小智画图……` : g.versions.length ? `${g.versions.length} 台已拿到路线图` : g.tablets.length ? `${g.tablets.length} 台在填写` : '还没开始' }}</p>
      </div>
    </section>

    <section v-if="view === 'V2'" class="v2">
      <div class="mapw"><PlanMap :nodes="nodes" :edges="edges" :trace="S.plan" :step="pm.step" :round="pm.round" :flash="pm.flash" /></div>
      <aside>
        <p class="msg" :class="{ red: pm.paused }">{{ pm.msg || '点“开始计算”' }}</p>
        <button v-if="pm.paused" class="btn big" @click="cont">继续（P）</button>
        <table v-if="S.plan && pm.step >= 3">
          <tr v-for="r in planRows" :key="r.g"><td>{{ r.cls }}</td><td>{{ r.stair }}</td><td><i class="cd" :class="r.color" /></td><td class="num">{{ Math.round(r.total || 0) }} 秒</td></tr>
        </table>
        <ul v-if="pm.step >= 3" class="notes"><template v-for="r in planRows" :key="r.g"><li v-for="n in r.notes" :key="n">{{ r.cls }}：{{ n }}</li></template></ul>
        <p v-if="S.plan" class="ops muted">规划器比较了 {{ S.plan.ops }} 次</p>
      </aside>
    </section>

    <section v-if="view === 'V3'" class="v3">
      <div v-for="g in S.groups" :key="g.group" class="wt card" @click="g.versions.length && openCheck(g.group, wallVer[g.group] || 0)">
        <h3>{{ g.class }}班 <small v-if="g.versions.length > 1"><button v-for="v in g.versions" :key="v.n" :class="{ on: verOf(g)?.n === v.n }" @click.stop="wallVer[g.group] = v.n">{{ v.seat }}号</button></small></h3>
        <div class="im">
          <img v-if="verOf(g)?.image" :src="verOf(g).image" />
          <p v-else class="muted">{{ g.busy ? '正在画……' : '等待中' }}</p>
          <span v-if="verOf(g)?.verdict === 'pass'" class="stp"><Stamp text="验收通过" /></span>
        </div>
      </div>
    </section>

    <section v-if="view === 'V4'" class="v4">
      <div class="big">
        <div v-if="selSvg" class="svgw" v-html="selSvg" />
        <p v-else class="muted">这一组还没有路线图</p>
        <span v-if="selV?.verdict === 'pass'" class="stp"><Stamp text="验收通过" /></span>
        <span v-if="selV?.verdict === 'fail'" class="stp"><Stamp text="请修改" color="#7d8c90" /></span>
      </div>
      <aside>
        <div class="gs"><button v-for="g in S.groups" :key="g.group" :class="{ on: sel.group === g.group }" @click="sel.group = g.group; sel.version = 0">{{ g.class }}</button></div>
        <div v-if="selV" class="chk">
          <div v-if="selG && selG.versions.length > 1" class="gs"><button v-for="v in selG.versions" :key="v.n" :class="{ on: selV.n === v.n }" @click="sel.version = v.n">{{ v.seat }}号</button></div>
          <h3>{{ selV.seat }} 号平板 · 系统核验</h3>
          <p v-for="(i, k) in selV.issues" :key="k" :class="{ warn: i.warn }">{{ i.warn ? '提醒' : '不对' }}：{{ i.label }}</p>
          <p v-if="!selV.issues?.length" class="ok">路线沿道路、楼梯正确、终点是集合点</p>
          <div class="vb"><button class="btn big" @click="verdict(true)">通过</button><button class="btn big ghost" @click="verdict(false)">不通过</button></div>
          <button class="lnk" @click="sel.showSpec = !sel.showSpec">{{ sel.showSpec ? '隐藏' : '显示' }}说明书</button>
          <pre v-if="sel.showSpec">{{ JSON.stringify(selV.spec, null, 1) }}</pre>
        </div>
      </aside>
    </section>

    <section v-if="view === 'V5'" class="v5">
      <div class="top">
        <span>选一组：</span>
        <button v-for="g in S.groups" :key="g.group" :class="{ on: five.group === g.group }" :disabled="!g.versions.length" @click="five.group = g.group">{{ g.class }}</button>
        <span class="sp" />
        <button class="btn" @click="openVote">① 让平板投票</button>
        <button class="btn" :disabled="five.running" @click="startFive">② 开始问 5 次</button>
      </div>
      <div v-if="five.voting || (!five.items.length && (S.tally.same + S.tally.diff))" class="votes">
        <h2>同样的要求问 5 次，会一样吗？</h2>
        <div class="vbar"><span>会一样</span><i :style="{ width: S.tally.same * 30 + 'px' }" /><b class="num">{{ S.tally.same }}</b></div>
        <div class="vbar diff"><span>会不一样</span><i :style="{ width: S.tally.diff * 30 + 'px' }" /><b class="num">{{ S.tally.diff }}</b></div>
      </div>
      <div v-if="five.items.length || five.running" class="tiles">
        <div v-for="k in 5" :key="k" class="tl card">
          <b>第 {{ k }} 次</b>
          <img v-if="five.imgs[k - 1] !== undefined && five.items[k - 1]" :src="five.imgs[five.items[k - 1].i]" />
          <p v-else class="muted">生成中……</p>
          <small v-if="five.items[k - 1]">{{ five.items[k - 1].stair }} · {{ five.items[k - 1].spec.line.color }}{{ five.items[k - 1].spec.line.style }}</small>
        </div>
      </div>
      <div v-if="five.items.length === 5" class="fst">
        <span v-for="(n, s) in stats.st" :key="s">{{ s }}：{{ n }} 次</span>
        <span>用了 {{ stats.colors }} 种线条</span><span>{{ stats.markers }} 种标志</span><span>核验没通过 {{ stats.fail }} 次</span>
        <span v-if="allSame" class="warn">5 次完全一样：换一个信息较少的组试试</span>
      </div>
    </section>

    <template #controls>
      <button v-for="v in order" :key="v" :class="{ on: view === v }" @click="view = v">{{ NAMES[v] }}</button>
      <span class="sep" />
      <button @click="runPlan">开始计算</button>
      <button v-if="pm.paused" class="on" @click="cont">继续</button>
      <button class="task2" :class="{ on: S.phase === 2 }" @click="setPhase(S.phase === 2 ? 1 : 2)">{{ S.phase === 2 ? '平板：回到任务一' : '平板：进入任务二' }}</button>
      <button @click="ctl('vote_close')">关闭投票</button>
      <button class="warn" @click="resetAll">重置本环节</button>
    </template>
  </ScreenStage>
</template>

<style scoped>
section { position: absolute; left: 0; right: 0; top: 104px; bottom: 0; padding: 30px 50px; }
.v0 { display: flex; gap: 50px; align-items: center; }
.task { flex: 1.1; padding: 50px; border-top: 14px solid var(--red); font-size: 36px; display: flex; flex-direction: column; gap: 18px; }
.badge { align-self: flex-start; background: var(--red); color: #fff; border-radius: 10px; padding: 4px 18px; font-size: 28px; font-weight: 800; }
.task h1 { font-size: 64px; }
.task b { color: var(--red); }
.task .std { font-size: 28px; color: var(--ink-2); }
.demo { flex: 1; display: flex; flex-direction: column; gap: 30px; font-size: 34px; }
.q { align-self: flex-end; background: var(--accent); color: #fff; padding: 20px 28px; border-radius: 24px 24px 4px 24px; }
.a { display: flex; gap: 18px; align-items: flex-start; }
.a p { background: #fff; padding: 20px 28px; border-radius: 4px 24px 24px 24px; box-shadow: var(--shadow); }
.v1 { display: grid; grid-template-columns: repeat(3, 1fr); gap: 26px; align-content: start; }
.gc { padding: 26px; display: flex; flex-direction: column; gap: 14px; }
.cp em { font-style: normal; font-weight: 800; }
.tabs { display: flex; gap: 6px; margin: 8px 0 2px; }
.tb { min-width: 34px; height: 30px; border-radius: 8px; display: inline-flex; align-items: center; justify-content: center; font-size: 17px; font-weight: 700; background: #eee9df; color: var(--muted); }
.tb.got { background: var(--output); color: #fff; }
.tb.busy { box-shadow: 0 0 0 3px var(--amber); }
.gc.busy { box-shadow: 0 0 0 5px var(--accent); }
.gc h2 { font-size: 38px; }
.cs { display: flex; flex-wrap: wrap; gap: 8px; }
.cp { font-size: 24px; padding: 4px 12px; border-radius: 10px; background: #eee9e0; color: #a8a49b; }
.cp.on { background: var(--accent-soft); color: var(--accent); font-weight: 800; }
.gc p { font-size: 28px; } .gc .st { color: var(--accent); font-weight: 700; }
.v2 { display: flex; gap: 30px; padding: 20px 40px; background: var(--calc-bg); }
.mapw { flex: none; width: 1130px; height: 920px; background: #fffdf8; border-radius: 16px; overflow: hidden; }
.v2 aside { flex: 1; color: var(--calc-ink); display: flex; flex-direction: column; gap: 18px; }
.msg { font-size: 36px; font-weight: 800; line-height: 1.4; }
.msg.red { color: #ff7a6b; animation: blink 1s infinite; }
@keyframes blink { 50% { opacity: .6; } }
table { font-size: 28px; border-collapse: collapse; }
td { padding: 6px 12px; border-bottom: 1px solid rgba(255, 255, 255, .12); }
.cd { display: inline-block; width: 22px; height: 22px; border-radius: 50%; }
.cd.green { background: #2bc06b; } .cd.yellow { background: #f2b705; } .cd.red { background: #e0442f; }
.notes { font-size: 24px; color: var(--calc-dim); margin: 0; padding-left: 26px; }
.ops { font-size: 22px; }
.v3 { display: grid; grid-template-columns: repeat(3, 1fr); grid-template-rows: 1fr 1fr; gap: 22px; }
.wt { padding: 14px 18px; display: flex; flex-direction: column; gap: 8px; overflow: hidden; cursor: pointer; }
.wt h3 { font-size: 30px; display: flex; justify-content: space-between; }
.wt h3 button { width: 44px; height: 40px; border-radius: 8px; background: #eee; margin-left: 6px; font-size: 22px; }
.wt h3 button.on { background: var(--accent); color: #fff; }
.im { flex: 1; position: relative; display: flex; align-items: center; justify-content: center; min-height: 0; }
.im img { max-width: 100%; max-height: 100%; border-radius: 8px; }
.stp { position: absolute; right: 20px; top: 20px; font-size: 40px; }
.v4 { display: flex; gap: 30px; }
.v4 .big { flex: none; width: 1100px; position: relative; }
.v4 .svgw :deep(svg) { width: 100%; height: auto; border-radius: 14px; }
.v4 .big .stp { font-size: 70px; right: 60px; top: 60px; }
.v4 aside { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 16px; }
.gs { display: flex; flex-wrap: wrap; gap: 10px; }
.gs button, .v5 .top > button:not(.btn) { min-height: 58px; padding: 0 20px; border-radius: 12px; background: #fff; font-size: 26px; font-weight: 700; box-shadow: var(--shadow); }
.gs button.on, .v5 .top > button.on { background: var(--accent); color: #fff; }
.chk h3 { font-size: 36px; margin-bottom: 8px; }
.chk p { font-size: 30px; color: var(--red); font-weight: 700; }
.chk p.warn { color: var(--amber); } .chk p.ok { color: var(--output); }
.vb { display: flex; gap: 16px; margin: 20px 0; flex-wrap: wrap; }
.lnk { font-size: 22px; color: var(--muted); text-decoration: underline; }
pre { font-size: 18px; background: #1d2540; color: #cfd6ff; padding: 14px; border-radius: 10px; max-height: 360px; overflow: auto; }
.v5 { display: flex; flex-direction: column; gap: 22px; }
.v5 .top { display: flex; gap: 12px; align-items: center; font-size: 28px; }
.v5 .sp { flex: 1; }
.votes h2 { font-size: 44px; margin-bottom: 14px; }
.vbar { display: flex; align-items: center; gap: 16px; font-size: 34px; margin: 10px 0; }
.vbar span { width: 170px; } .vbar i { height: 48px; background: var(--accent); border-radius: 10px; min-width: 6px; transition: width .4s; } .vbar.diff i { background: var(--amber); }
.tiles { display: grid; grid-template-columns: repeat(5, 1fr); gap: 16px; }
.tl { padding: 12px; display: flex; flex-direction: column; gap: 8px; font-size: 26px; animation: up .5s var(--ease) both; }
.tl img { width: 100%; border-radius: 8px; }
.tl small { font-size: 22px; color: var(--ink-2); }
@keyframes up { from { opacity: 0; transform: translateY(30px); } }
.fst { display: flex; gap: 30px; font-size: 32px; font-weight: 700; flex-wrap: wrap; }
.fst .warn { color: var(--amber); }
</style>
