<script setup lang="ts">
/* 输出环节 · 学生平板（小组共享）：左边任务区（班级卡 / 信息卡 / 要求），右边元小智对话区。 */
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import YxzAvatar from '../common/components/YxzAvatar.vue'
import SeatPicker from '../common/components/SeatPicker.vue'
import ZoomViewer from '../common/components/ZoomViewer.vue'
import RetryButton from '../common/components/RetryButton.vue'
import LookAtScreen from '../common/components/LookAtScreen.vue'
import Toasts from '../common/components/Toasts.vue'
import Stamp from '../common/components/Stamp.vue'
import { ApiError, events, get, post, stream, token } from '../common/api'
import { ME, loadMe } from '../common/seat'
import { toast } from '../common/toast'
import { openZoom } from '../common/zoom'
import { ICON } from '../common/icons'
import { campusThumb, routeJpeg, routeSvg } from './map/render'
import './map/map.css'
import type { DrawSpec, RoadNode } from './types'

const G = reactive<any>({ loaded: false, group: 0, class: {}, cards: [], req: '', versions: [], chat: [], busy: 0, max_versions: 5 })
const nodes = ref<RoadNode[]>([])
const chips = ref<Record<string, string[]>>({})
const stage = ref<'loading' | 'seat' | 'work'>('loading')
const live = ref<{ version: number; spec: DrawSpec } | null>(null) // 正在“改图”的那一版
const sending = ref(false)
const vote = reactive({ open: false, mine: '', cls: '' })
const look = ref(false)
const moved = ref(false)
const verdicts = reactive<Record<number, string>>({})
const chatBox = ref<HTMLElement>()
let off: (() => void) | null = null
const reqDraft = ref('')
const notes = reactive<Record<number, string>>({})
let reqT = 0
const noteT: Record<number, number> = {}

function applyGroup(d: any) {
  Object.assign(G, d, { loaded: true })
  if (document.activeElement?.id !== 'req') reqDraft.value = d.req
  for (const c of d.cards) if (document.activeElement?.id !== 'note' + c.id) notes[c.id] = c.note
  if (d.vote) { vote.open = d.vote.open; vote.mine = d.vote.mine || '' }
  scrollChat()
}
async function load() { applyGroup(await get('/api/shuchu/group')) }
async function init() {
  await loadMe()
  if (!ME.seat) { stage.value = 'seat'; return }
  const g = await get('/api/shuchu/graph')
  nodes.value = g.nodes; chips.value = g.chips
  await load()
  stage.value = 'work'
  off = events('shuchu', 'student', {
    group_update: (d) => {
      if (d.by && d.by !== ME.seat && d.what) toast(`${d.by} 号平板${d.what}`)
      applyGroup(d)
    },
    vote: (d) => { vote.open = d.open; vote.cls = d.class || ''; if (d.open) vote.mine = '' },
    verdict: (d) => (verdicts[d.version] = d.verdict),
    look: (d) => (look.value = d.on),
    reset: () => location.reload(),
    seats_cleared: () => location.reload(),
    seat_moved: (d) => { if (d.keep !== token()) moved.value = true },
  })
}
onMounted(init)
onBeforeUnmount(() => off && off())
async function onSeat() { stage.value = 'loading'; await init() }

function scrollChat() { nextTick(() => { const el = chatBox.value; if (el) el.scrollTop = el.scrollHeight }) }

// ---------------- 信息卡 / 要求 ----------------
async function toggle(c: any) {
  c.on = !c.on
  try { await post('/api/shuchu/group/cards', { card: c.id, on: c.on }) } catch (e) { c.on = !c.on; toast((e as Error).message, 'warn') }
}
function noteInput(id: number) {
  clearTimeout(noteT[id])
  noteT[id] = window.setTimeout(() => post('/api/shuchu/group/cards', { card: id, note: notes[id] }), 600)
}
function reqInput() { clearTimeout(reqT); reqT = window.setTimeout(() => post('/api/shuchu/group/cards', { req: reqDraft.value }), 600) }
function addChip(t: string) {
  reqDraft.value = reqDraft.value ? reqDraft.value.replace(/[，。,\s]*$/, '') + '，' + t : t
  reqInput()
}
const checked = computed(() => G.cards.filter((c: any) => c.on).length)

// ---------------- 提交：规划 + 画图 ----------------
async function ask(retry = false) {
  if (sending.value || G.busy) return
  clearTimeout(reqT)
  if (!retry) await post('/api/shuchu/group/cards', { req: reqDraft.value })
  sending.value = true
  try {
    await stream(retry ? '/api/shuchu/group/retry' : '/api/shuchu/group/ask', {}, async (ev, d) => {
      if (ev === 'spec') {
        live.value = { version: d.version, spec: d.spec }
        scrollChat()
        // 先在底图上“改”给学生看，再栅格化成 JPEG 发回对话
        const [jpg] = await Promise.all([routeJpeg(d.spec, nodes.value).catch(() => ''), new Promise((r) => setTimeout(r, 2600))])
        if (jpg) { try { await post('/api/shuchu/group/image', { version: d.version, data: jpg }) } catch { /* 同组别的平板也会传 */ } }
        live.value = null
        await load()
      }
    })
  } catch (e) {
    toast((e as Error).message, 'warn')
    if (e instanceof ApiError) load()
  } finally { sending.value = false }
}

const vById = computed<Record<number, any>>(() => Object.fromEntries(G.versions.map((v: any) => [v.n, v])))
const svgCache = new Map<string, string>()
function inlineSvg(spec: DrawSpec, animate: boolean) {
  const k = JSON.stringify(spec) + animate
  if (!svgCache.has(k)) svgCache.set(k, routeSvg(spec, nodes.value, { animate }))
  return svgCache.get(k)!
}
function zoomVersion(v: any, e: MouseEvent) {
  if (v.image) openZoom(v.image, e)
  else openZoom(URL.createObjectURL(new Blob([routeSvg(v.spec, nodes.value)], { type: 'image/svg+xml' })), e)
}
const left = computed(() => G.max_versions - G.versions.length)
const lastIsMine = computed(() => G.versions.length > 0)
const compare = ref(false)
async function sendVote(c: string) {
  vote.mine = c
  try { await post('/api/shuchu/vote', { choice: c }); toast('投票成功', 'ok') } catch (e) { toast((e as Error).message, 'warn') }
}
const thumb = computed(() => (stage.value === 'work' ? campusThumb() : ''))
</script>

<template>
  <YxzHeader title="疏散路线图" link="output" :seat="ME.seat" :state="sending || G.busy ? 'think' : 'idle'">
    <span v-if="G.group">第 {{ G.group }} 组 · {{ ME.seat }} 号平板</span>
  </YxzHeader>
  <Toasts />
  <SeatPicker v-if="stage === 'seat'" @done="onSeat" hint="同组的平板会一起看到同一份任务" />
  <main v-else-if="stage === 'work'" class="wk">
    <!-- 任务区 -->
    <section class="task">
      <div class="cls card">
        <div class="ct"><span class="lbl">我们负责</span><b>{{ G.class.class }}班</b></div>
        <p>位置：{{ G.class.building }} {{ G.class.floor }} 楼 {{ G.class.position }} · 人数：<b class="num">{{ G.class.people }}</b></p>
        <p class="std"><b>验收标准</b>沿道路走 · 楼梯走得对 · 到达集合点 · 楼梯不拥堵 · 标出安全提醒 · 符合“给谁看”</p>
      </div>
      <h3>① 勾选要告诉元小智的信息 <small class="muted">已选 {{ checked }} / 6</small></h3>
      <div class="cards">
        <div v-for="c in G.cards" :key="c.id" class="ic card" :class="{ on: c.on }">
          <button class="chk" @click="toggle(c)">
            <span class="box"><span v-if="c.on" v-html="ICON.check" /></span>
            <span class="tt"><b>{{ '①②③④⑤⑥'[c.id - 1] }} {{ c.title }}</b><small>{{ c.on ? c.text : '没给：' + c.miss }}</small></span>
          </button>
          <input v-if="c.on" :id="'note' + c.id" v-model="notes[c.id]" class="note" maxlength="40" placeholder="补充一句（可选）" @input="noteInput(c.id)" />
        </div>
      </div>
      <h3>② 对路线图的要求</h3>
      <textarea id="req" v-model="reqDraft" rows="3" maxlength="120" placeholder="例如：一年级小朋友也能看懂，用绿色大箭头，标出“你在这里”和集合点（可以用输入法的语音）" @input="reqInput" />
      <div class="chips">
        <div v-for="(arr, k) in chips" :key="k" class="cg"><span class="ck">{{ k }}</span><button v-for="t in arr" :key="t" class="chip" @click="addChip(t)">{{ t }}</button></div>
      </div>
    </section>

    <!-- 对话区 -->
    <section class="chat">
      <div ref="chatBox" class="msgs">
        <div class="m ai"><YxzAvatar :size="40" /><div class="bb">我是元小智。把信息卡勾好、写下要求，再连同校园平面图一起发给我，我来在图上画逃生路线。</div></div>
        <template v-for="(m, i) in G.chat" :key="i">
          <div v-if="m.role === 'user'" class="m me">
            <div class="bb">
              <small class="who">{{ m.seat }} 号平板</small>
              <img v-if="m.thumb" :src="thumb" class="thumb" alt="校园平面图" />
              <span>{{ m.text }}</span>
            </div>
          </div>
          <div v-else class="m ai">
            <YxzAvatar :size="40" :state="m.pending ? 'think' : 'speak'" />
            <div class="bb" :class="{ pend: m.pending }">
              <template v-if="m.version && vById[m.version]">
                <div class="pic" @click="zoomVersion(vById[m.version], $event)">
                  <img v-if="vById[m.version].image" :src="vById[m.version].image" alt="改好的路线图" />
                  <div v-else class="svgw" v-html="inlineSvg(vById[m.version].spec, false)" />
                  <span class="zoom">点图放大</span>
                  <span v-if="verdicts[m.version] === 'pass'" class="vd"><Stamp text="验收通过" /></span>
                </div>
              </template>
              <span>{{ m.text }}</span>
              <span v-if="m.pending" class="dots"><i /><i /><i /></span>
            </div>
          </div>
        </template>
        <div v-if="live" class="m ai">
          <YxzAvatar :size="40" state="think" />
          <div class="bb"><div class="pic editing"><div class="svgw" v-html="inlineSvg(live.spec, true)" /><span class="tag">正在修改图片……</span></div></div>
        </div>
      </div>
      <div v-if="G.versions.length > 1" class="vtabs">
        <span>对比：</span>
        <button v-for="v in G.versions" :key="v.n" class="chip" @click="zoomVersion(v, $event)">第 {{ v.n }} 版{{ v.kind === 'retry' ? ' · 再试' : '' }}</button>
        <button class="chip" @click="compare = !compare">{{ compare ? '收起并排' : '并排看' }}</button>
      </div>
      <div v-if="compare" class="side">
        <div v-for="v in G.versions" :key="v.n" class="sv" @click="zoomVersion(v, $event)"><img v-if="v.image" :src="v.image" /><b>第 {{ v.n }} 版</b></div>
      </div>
      <footer class="send">
        <p v-if="G.busy && G.busy !== ME.seat" class="busy">{{ G.busy }} 号平板正在请元小智画图……</p>
        <div class="row">
          <RetryButton v-if="lastIsMine" :left="left" :busy="sending || !!G.busy" confirm-text="用完全相同的要求再画一次，看看会不会不一样？" @go="ask(true)" />
          <span class="sp" />
          <button class="btn big" :disabled="sending || !!G.busy || left <= 0" @click="ask(false)">
            <span v-html="ICON.send" />{{ G.versions.length ? '改好了，再发给元小智' : '发给元小智' }}
          </button>
        </div>
      </footer>
    </section>
  </main>
  <ZoomViewer />

  <div v-if="vote.open" class="mask">
    <div class="vote card">
      <YxzAvatar state="speak" :size="70" />
      <h2>同样的要求，让元小智画 5 次</h2>
      <p>5 张图会一模一样吗？</p>
      <div class="row">
        <button class="btn big" :class="{ ghost: vote.mine !== 'same' }" @click="sendVote('same')">会一样</button>
        <button class="btn big" :class="{ ghost: vote.mine !== 'diff' }" @click="sendVote('diff')">会不一样</button>
      </div>
      <small v-if="vote.mine" class="muted">已投票，可以改，请看大屏</small>
    </div>
  </div>
  <LookAtScreen v-if="look" />
  <LookAtScreen v-if="moved" text="已在别的平板上继续" />
</template>

<style scoped>
.wk { height: calc(100vh - 64px - var(--icp-h)); height: calc(100dvh - 64px - var(--icp-h)); display: flex; }
.task { width: 55%; overflow: auto; -webkit-overflow-scrolling: touch; padding: 16px 18px 30px; display: flex; flex-direction: column; gap: 12px; }
.task > * { flex-shrink: 0; }
.cls { padding: 16px 20px; border-left: 8px solid var(--accent); }
.ct { display: flex; align-items: center; gap: 12px; }
.lbl { background: var(--accent); color: #fff; border-radius: 8px; padding: 2px 10px; font-size: 16px; font-weight: 700; }
.ct b { font-size: 30px; color: var(--accent); }
.cls p { font-size: 19px; margin-top: 4px; }
.std { font-size: 17px !important; color: var(--ink-2); }
.std b { margin-right: 8px; color: var(--ink); }
h3 { font-size: 21px; margin-top: 4px; }
h3 small { font-size: 16px; font-weight: 500; }
.cards { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.ic { padding: 4px; transition: box-shadow var(--t-light); }
.ic.on { box-shadow: inset 0 0 0 3px var(--accent), var(--shadow); background: var(--accent-soft); }
.chk { display: flex; gap: 12px; align-items: flex-start; width: 100%; text-align: left; padding: 10px 10px 8px; min-height: 80px; }
.box { width: 34px; height: 34px; border-radius: 9px; border: 3px solid #c9c2b6; flex: none; display: flex; align-items: center; justify-content: center; color: #fff; background: #fff; }
.ic.on .box { background: var(--accent); border-color: var(--accent); }
.box :deep(svg) { width: 22px; height: 22px; }
.tt { display: flex; flex-direction: column; }
.tt b { font-size: 19px; }
.tt small { font-size: 15px; color: var(--ink-2); line-height: 1.45; }
.note { width: calc(100% - 16px); margin: 0 8px 8px; min-height: 46px; border-radius: 10px; border: 2px solid var(--line); padding: 0 12px; font-size: 17px; }
textarea { width: 100%; min-height: 112px; border-radius: 14px; border: 2px solid var(--line); padding: 12px 14px; font-size: 20px; line-height: 1.5; resize: none; outline: none; background: #fff; }
textarea:focus, .note:focus { border-color: var(--accent); outline: none; }
.chips { display: flex; flex-direction: column; gap: 8px; }
.cg { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.ck { font-size: 15px; color: var(--muted); width: 72px; }
.cg .chip { min-height: 42px; font-size: 16px; padding: 0 14px; }
.chat { width: 45%; display: flex; flex-direction: column; background: var(--paper); border-left: 1px solid var(--line); }
.msgs { flex: 1; overflow: auto; -webkit-overflow-scrolling: touch; padding: 16px; display: flex; flex-direction: column; gap: 14px; }
.m { display: flex; gap: 10px; align-items: flex-start; }
.m.me { justify-content: flex-end; }
.bb { max-width: 86%; background: #fff; border-radius: 16px; padding: 12px 14px; box-shadow: var(--shadow); font-size: 18px; display: flex; flex-direction: column; gap: 8px; }
.me .bb { background: var(--accent); color: #fff; }
.who { font-size: 14px; opacity: .8; }
.thumb { width: 160px; border-radius: 8px; background: #fff; }
.pic { position: relative; width: 100%; min-width: 260px; border-radius: 10px; overflow: hidden; cursor: zoom-in; background: #fffdf8; }
.pic img { width: 100%; display: block; }
.svgw { width: 100%; }
.zoom { position: absolute; right: 8px; bottom: 8px; background: rgba(20, 28, 32, .7); color: #fff; font-size: 14px; padding: 2px 10px; border-radius: 8px; }
.editing .tag { position: absolute; left: 8px; top: 8px; background: var(--accent); color: #fff; font-size: 15px; padding: 2px 10px; border-radius: 8px; animation: blink 1s infinite; }
@keyframes blink { 50% { opacity: .5; } }
.vd { position: absolute; right: 12px; top: 12px; }
.pend { color: var(--ink-2); }
.dots { display: inline-flex; gap: 4px; }
.dots i { width: 8px; height: 8px; border-radius: 50%; background: var(--accent); animation: blink 1s infinite; }
.dots i:nth-child(2) { animation-delay: .2s; } .dots i:nth-child(3) { animation-delay: .4s; }
.vtabs { display: flex; gap: 8px; align-items: center; padding: 8px 14px; flex-wrap: wrap; border-top: 1px solid var(--line); font-size: 16px; }
.side { display: flex; gap: 8px; overflow-x: auto; padding: 8px 14px; }
.sv { width: 200px; flex: none; text-align: center; font-size: 15px; }
.sv img { width: 100%; border-radius: 8px; }
.send { padding: 10px 14px 12px; border-top: 1px solid var(--line); background: #fff; }
.busy { color: var(--amber); font-weight: 700; margin-bottom: 6px; text-align: center; }
.send .row { display: flex; align-items: center; gap: 12px; }
.send .sp { flex: 1; }
.mask { position: fixed; inset: 0; background: rgba(20, 28, 32, .55); display: flex; align-items: center; justify-content: center; z-index: 70; }
.vote { padding: 32px 40px; text-align: center; display: flex; flex-direction: column; align-items: center; gap: 12px; }
.vote h2 { font-size: 30px; } .vote p { font-size: 24px; }
.vote .row { display: flex; gap: 18px; margin-top: 10px; }
@media (orientation: portrait), (max-width: 900px) {
  .wk { flex-direction: column; height: auto; }
  .task, .chat { width: 100%; }
  .chat { min-height: 70vh; border-left: 0; border-top: 1px solid var(--line); }
  .cards { grid-template-columns: 1fr; }
}
</style>
