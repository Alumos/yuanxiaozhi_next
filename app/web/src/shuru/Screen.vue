<script setup lang="ts">
/* 输入环节 · 大屏：全班的“输入墙”（每台平板给了元小智什么）+ 偏好统计；点一台看“输入 → 输出”。 */
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import ScreenStage from '../common/components/ScreenStage.vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import RollingNumber from '../common/components/RollingNumber.vue'
import Toasts from '../common/components/Toasts.vue'
import { events, get, post } from '../common/api'
import { toast } from '../common/toast'

const S = reactive<any>({ students: [], all_seats: [], mode: 'live' })
const by = computed<Record<number, any>>(() => Object.fromEntries(S.students.map((s: any) => [s.seat, s])))
const pop = reactive<Record<number, boolean>>({})
const detail = ref<any>(null)
const dv = ref(0)
let off: (() => void) | null = null
async function load() { Object.assign(S, await get('/api/shuru/screen/state')) }
function upsert(d: any) {
  const i = S.students.findIndex((s: any) => s.seat === d.seat)
  if (i >= 0) S.students[i] = d; else S.students.push(d)
  pop[d.seat] = true; setTimeout(() => delete pop[d.seat], 2500)
}
onMounted(async () => {
  await load()
  off = events('shuru', 'screen', { seat_input: upsert, seat_guide: upsert, reset: load, mode: (d) => (S.mode = d.mode) })
})
onBeforeUnmount(() => off && off())

const counts = computed(() => {
  const c: Record<string, number> = {}
  for (const s of S.students) for (const k of ['scene', 'play']) for (const v of s.prefs?.[k] || []) c[v] = (c[v] || 0) + 1
  return Object.entries(c).sort((a, b) => b[1] - a[1]).slice(0, 14)
})
const cmax = computed(() => Math.max(1, ...counts.value.map((x) => x[1])))
const places = computed(() => {
  const c: Record<string, number> = {}
  for (const s of S.students) if (s.top) c[s.top] = (c[s.top] || 0) + 1
  return Object.entries(c).sort((a, b) => b[1] - a[1]).slice(0, 6)
})
const stats = computed(() => {
  const st = S.students.filter((s: any) => s.sent)
  return { n: st.length, avg: st.length ? st.reduce((a: number, s: any) => a + (s.n_info || 0), 0) / st.length : 0, text: st.filter((s: any) => s.text).length, voice: st.filter((s: any) => s.voice).length, guides: S.students.reduce((a: number, s: any) => a + (s.guides || 0), 0) }
})
async function open(seat: number) {
  if (!by.value[seat]?.guides) { toast(`${seat} 号平板还没有攻略`, 'warn'); return }
  detail.value = await get(`/api/shuru/screen/seat?seat=${seat}`)
  dv.value = detail.value.guides.length - 1
}
const g = computed(() => detail.value?.guides[dv.value])
async function ctl(action: string) { await post('/api/shuru/screen/control', { action }); if (action === 'reset') { detail.value = null; load() } }
function resetAll() { if (window.confirm('清空输入环节的全部攻略？')) ctl('reset') }
function onKey(k: string) {
  if (k === 'home') detail.value = null
  else if (k.startsWith('seat:')) open(Number(k.slice(5)))
  else if (detail.value && (k === 'next' || k === 'prev')) dv.value = Math.max(0, Math.min(detail.value.guides.length - 1, dv.value + (k === 'next' ? 1 : -1)))
}
const short = (s: string) => (s || '').replace(/\n.*$/s, '').split('；').map((x) => x.replace(/^[^：]*：/, '')).join(' · ')
</script>

<template>
  <ScreenStage :mode="S.mode" @key="onKey">
    <Toasts />
    <YxzHeader title="元小智 · 输入" link="input" big>秋假攻略</YxzHeader>
    <section class="wall">
      <div class="left">
        <div class="ctr">
          <div><b><RollingNumber :value="stats.n" /></b>台平板发来了输入</div>
          <div>平均每台 <b><RollingNumber :value="stats.avg" :decimals="1" /></b> 条信息</div>
          <div><b><RollingNumber :value="stats.text" /></b>台写了补充 · <b><RollingNumber :value="stats.voice" /></b>台用了语音</div>
        </div>
        <div class="grid">
          <button v-for="a in S.all_seats" :key="a.seat" class="cell" :class="{ on: by[a.seat]?.sent, busy: by[a.seat]?.busy, pop: pop[a.seat] }" @click="open(a.seat)">
            <b class="sn">{{ a.seat }}</b>
            <em v-if="by[a.seat]?.n_info" class="ni">{{ by[a.seat].n_info }} 条</em>
            <p v-if="by[a.seat]?.sent" class="sent">{{ short(by[a.seat].sent) }}</p>
            <span v-if="by[a.seat]?.top" class="top">→ {{ by[a.seat].top }}</span>
            <span v-else-if="by[a.seat]?.busy" class="top w">元小智正在写……</span>
          </button>
        </div>
      </div>
      <aside>
        <h2>大家想要的</h2>
        <div v-for="[k, n] in counts" :key="k" class="hb"><span>{{ k }}</span><i :style="{ width: (n / cmax) * 100 + '%' }" /><b class="num">{{ n }}</b></div>
        <p v-if="!counts.length" class="muted">等同学们发来输入……</p>
        <h2 class="mt">元小智推荐最多的地方</h2>
        <div v-for="([k, n], i) in places" :key="k" class="pr"><b>{{ i + 1 }}</b><span>{{ k }}</span><em class="num">{{ n }} 台</em></div>
      </aside>
    </section>

    <div v-if="detail && g" class="mask" @click.self="detail = null">
      <div class="dt">
        <div class="dh">
          <h2>{{ detail.seat }} 号平板</h2>
          <div class="tabs"><button v-for="(x, i) in detail.guides" :key="x.n" :class="{ on: dv === i }" @click="dv = i">第 {{ x.n }} 份{{ x.kind === 'retry' ? ' · 再试' : x.kind === 'focus' ? ' · 换地方' : '' }}</button></div>
          <button class="x" @click="detail = null">关闭</button>
        </div>
        <div class="io">
          <div class="inp">
            <h3>输入 <small>{{ g.n_info }} 条信息{{ g.voice ? ' · 用了语音' : '' }}</small></h3>
            <p v-for="(l, i) in g.sent.split(/[；\n]/).filter(Boolean)" :key="i">{{ l }}</p>
          </div>
          <span class="arrow">→</span>
          <div class="outp">
            <h3>输出 <small>用时 {{ (g.ms / 1000).toFixed(1) }} 秒</small></h3>
            <div v-for="(p, i) in g.places" :key="p.name" class="pp"><b>{{ i + 1 }}. {{ p.name }}</b><small>{{ p.prov }}</small><p>{{ p.why }}</p></div>
            <div class="days"><p v-for="d in g.days" :key="d.n"><b>第 {{ d.n }} 天</b>{{ d.text }}</p></div>
          </div>
        </div>
      </div>
    </div>

    <template #controls>
      <button @click="ctl('look')">平板：请看大屏</button>
      <button @click="ctl('unlook')">平板：继续操作</button>
      <button class="warn" @click="resetAll">清空本环节</button>
    </template>
  </ScreenStage>
</template>

<style scoped>
.wall { position: absolute; left: 0; right: 0; top: 104px; bottom: 0; display: flex; gap: 30px; padding: 22px 40px; }
.left { flex: 1; display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.ctr { display: flex; gap: 46px; font-size: 28px; color: var(--ink-2); }
.ctr b { font-size: 50px; color: var(--accent); margin: 0 6px; }
.grid { flex: 1; display: grid; grid-template-columns: repeat(5, 1fr); grid-template-rows: repeat(5, 1fr); gap: 12px; }
.cell { position: relative; border-radius: 16px; background: #ece8e0; color: #a1a8aa; text-align: left; padding: 10px 12px; display: flex; flex-direction: column; gap: 4px; overflow: hidden; transition: transform .3s var(--ease); }
.cell.on { background: #fff; color: var(--ink); box-shadow: var(--shadow); }
.cell.busy { box-shadow: 0 0 0 4px var(--accent); }
.cell.pop { transform: scale(1.06); z-index: 2; }
.sn { font-size: 30px; line-height: 1; }
.ni { position: absolute; right: 10px; top: 10px; font-style: normal; font-size: 20px; background: var(--accent); color: #fff; border-radius: 8px; padding: 0 8px; }
.sent { font-size: 18px; line-height: 1.35; max-height: 3.9em; overflow: hidden; color: var(--ink-2); }
.top { margin-top: auto; font-size: 22px; font-weight: 800; color: var(--accent); }
.top.w { font-weight: 600; font-size: 19px; animation: blink 1s infinite; }
@keyframes blink { 50% { opacity: .4; } }
aside { width: 520px; background: #fff; border-radius: 22px; padding: 24px 28px; box-shadow: var(--shadow); }
aside h2 { font-size: 34px; margin-bottom: 12px; }
aside .mt { margin-top: 26px; }
.hb { display: flex; align-items: center; gap: 12px; font-size: 24px; margin: 6px 0; }
.hb span { width: 120px; } .hb i { height: 24px; background: var(--accent); border-radius: 6px; min-width: 6px; transition: width .5s var(--ease); flex: none; max-width: 280px; } .hb b { color: var(--accent); }
.pr { display: flex; gap: 14px; align-items: center; font-size: 28px; margin: 8px 0; }
.pr b { color: var(--accent); width: 28px; } .pr span { flex: 1; font-weight: 700; } .pr em { font-style: normal; color: var(--muted); font-size: 24px; }
.mask { position: absolute; inset: 0; z-index: 50; background: rgba(20, 28, 40, .55); display: flex; align-items: center; justify-content: center; }
.dt { width: 1760px; height: 900px; background: var(--paper); border-radius: 26px; padding: 26px 34px; display: flex; flex-direction: column; gap: 16px; animation: up .4s var(--ease); }
@keyframes up { from { opacity: 0; transform: translateY(30px); } }
.dh { display: flex; align-items: center; gap: 20px; }
.dh h2 { font-size: 44px; }
.tabs { display: flex; gap: 10px; flex: 1; }
.tabs button, .x { min-height: 56px; padding: 0 22px; border-radius: 12px; background: #fff; font-size: 24px; font-weight: 700; box-shadow: inset 0 0 0 2px var(--line); }
.tabs button.on { background: var(--accent); color: #fff; box-shadow: none; }
.io { flex: 1; display: flex; gap: 20px; min-height: 0; }
.inp { width: 560px; background: var(--accent-soft); border-radius: 20px; padding: 22px 26px; }
.inp h3, .outp h3 { font-size: 38px; color: var(--accent); margin-bottom: 10px; }
.inp small, .outp small { font-size: 22px; color: var(--muted); font-weight: 500; }
.inp p { font-size: 28px; line-height: 1.5; margin-bottom: 8px; }
.arrow { align-self: center; font-size: 70px; color: var(--accent); font-weight: 900; }
.outp { flex: 1; background: #fff; border-radius: 20px; padding: 22px 26px; overflow: hidden; }
.outp h3 { color: var(--output); }
.pp { margin-bottom: 10px; }
.pp b { font-size: 30px; } .pp small { font-size: 22px; margin-left: 10px; color: var(--muted); } .pp p { font-size: 24px; color: var(--ink-2); }
.days p { font-size: 22px; line-height: 1.5; padding: 6px 0; border-top: 1px dashed var(--line); }
.days b { color: var(--accent); margin-right: 10px; }
</style>
