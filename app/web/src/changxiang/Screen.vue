<script setup lang="ts">
/* 畅想 · 教室大屏：同学们的新本领做成卷轴，三排循环滚动传递；点一卷展开看详情。 */
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import LockToggle from '../common/components/LockToggle.vue'
import ScreenStage from '../common/components/ScreenStage.vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import RollingNumber from '../common/components/RollingNumber.vue'
import Toasts from '../common/components/Toasts.vue'
import { events, get, post } from '../common/api'
import { toast } from '../common/toast'

interface Idea { id: string; seat: number; group: number; name: string; input: string; compute: string; output: string; at: string; hidden?: boolean }
const S = reactive({ ideas: [] as Idea[], seats: 0, mode: 'live' })
const fresh = reactive<Record<string, boolean>>({})
const open = ref<Idea | null>(null)
const paused = ref(false)
let off: (() => void) | null = null

async function load() { Object.assign(S, await get('/api/changxiang/screen/state')) }
onMounted(async () => {
  await load()
  off = events('changxiang', 'screen', {
    idea: (d: Idea) => {
      S.ideas.push(d)
      S.seats = new Set(S.ideas.map((i) => i.seat)).size
      fresh[d.id] = true
      setTimeout(() => delete fresh[d.id], 9000)
    },
    ideas_changed: load, reset: load, mode: (d) => (S.mode = d.mode),
  })
})
onBeforeUnmount(() => off && off())

const shown = computed(() => S.ideas.filter((i) => !i.hidden))
const PAPER = ['#fbf3df', '#f6ead6', '#fdf6e8', '#f3e6cc']
// 三排：每排至少铺满一屏宽（卷轴宽 230 + 间距 40），再复制一遍做无缝循环
const rows = computed(() => {
  const all = shown.value
  const r: Idea[][] = [[], [], []]
  all.forEach((it, i) => r[i % 3].push(it))
  return r.map((row, k) => {
    if (!row.length) return { items: [] as Idea[], dur: 0, dir: k % 2 ? 'r' : 'l' }
    let base = [...row]
    while (base.length * 270 < 2000) base = base.concat(row)
    return { items: base.concat(base), dur: Math.max(30, base.length * 5.5), dir: k % 2 ? 'r' : 'l' }
  })
})
async function ctl(action: string, id = '') {
  await post('/api/changxiang/screen/control', { action, id })
  if (action === 'hide' || action === 'delete') { open.value = null; toast(action === 'hide' ? '已从大屏隐藏' : '已删除', 'ok') }
  if (action === 'reset') { open.value = null; await load() }
}
function resetAll() { if (window.confirm('清空所有畅想？')) ctl('reset') }
function onKey(k: string) {
  if (k === 'home') open.value = null
  else if (k === 'pause') paused.value = !paused.value
}
const latest = computed(() => shown.value.slice(-1)[0])
</script>

<template>
  <ScreenStage :mode="S.mode" @key="onKey">
    <Toasts />
    <YxzHeader title="元小智 · 畅想" link="imagine" big><template #after><LockToggle mod="changxiang" /></template>新本领征集</YxzHeader>
    <section class="wall" :class="{ paused: paused || !!open }">
      <div class="top">
        <h1>同学们希望元小智学会的新本领</h1>
        <p>已收到 <b><RollingNumber :value="shown.length" /></b> 个畅想 · 来自 <b><RollingNumber :value="S.seats" /></b> 台平板</p>
      </div>
      <p v-if="!shown.length" class="empty">等待同学们的畅想……<br /><small>平板打开顶栏的“畅想”就能提交</small></p>
      <div v-for="(row, k) in rows" :key="k" class="row">
        <div v-if="row.items.length" class="track" :class="row.dir" :style="{ animationDuration: row.dur + 's' }">
          <button v-for="(it, i) in row.items" :key="it.id + '-' + i" class="sc" :class="{ fresh: fresh[it.id] }" :style="{ '--p': PAPER[(it.seat + i) % 4] }" @click="open = it">
            <i class="rod" />
            <span class="pp">
              <b class="nm">{{ it.name }}</b>
              <span class="mini"><em class="i">输入</em><em class="c">计算</em><em class="o">输出</em></span>
              <small>{{ it.seat }} 号平板</small>
            </span>
            <i class="rod" />
            <span v-if="fresh[it.id]" class="new">新</span>
          </button>
        </div>
      </div>
      <div v-if="latest && fresh[latest.id]" class="ticker">刚刚收到：{{ latest.seat }} 号平板的「{{ latest.name }}」</div>
    </section>

    <!-- 展开一卷 -->
    <div v-if="open" class="mask" @click.self="open = null">
      <div class="unroll">
        <i class="rod l" />
        <div class="paper">
          <small>{{ open.seat }} 号平板 · 第 {{ open.group }} 组的畅想</small>
          <h2>{{ open.name }}</h2>
          <div class="trio">
            <div class="part i"><h3>输入</h3><p>{{ open.input }}</p></div>
            <span class="ar">→</span>
            <div class="part c"><h3>计算</h3><p>{{ open.compute }}</p></div>
            <span class="ar">→</span>
            <div class="part o"><h3>输出</h3><p>{{ open.output }}</p></div>
          </div>
          <div class="ops">
            <button class="close" @click="open = null">收起卷轴</button>
            <button class="hide" @click="ctl('hide', open.id)">从大屏隐藏</button>
          </div>
        </div>
        <i class="rod r" />
      </div>
    </div>

    <template #controls>
      <button :class="{ on: paused }" @click="paused = !paused">{{ paused ? '继续滚动' : '暂停滚动' }}</button>
      <button class="warn" @click="resetAll">清空畅想</button>
    </template>
  </ScreenStage>
</template>

<style scoped>
.wall { position: absolute; left: 0; right: 0; top: 104px; bottom: 0; overflow: hidden; background: radial-gradient(ellipse at 50% 0%, #fff7ea, var(--bg) 70%); }
.top { text-align: center; padding: 14px 0 0; }
.top h1 { font-size: 54px; color: var(--accent); }
.top p { font-size: 30px; color: var(--ink-2); }
.top b { font-size: 44px; color: var(--accent); margin: 0 6px; }
.empty { text-align: center; font-size: 44px; color: var(--muted); margin-top: 220px; line-height: 1.6; }
.empty small { font-size: 30px; }
.row { height: 246px; margin-top: 8px; overflow: hidden; }
.track { display: flex; gap: 40px; width: max-content; padding: 6px 20px; animation: slide linear infinite; }
.track.r { animation-name: slide-r; }
.paused .track { animation-play-state: paused; }
@keyframes slide { from { transform: translateX(0); } to { transform: translateX(-50%); } }
@keyframes slide-r { from { transform: translateX(-50%); } to { transform: translateX(0); } }
.sc { position: relative; width: 230px; height: 232px; display: flex; flex-direction: column; align-items: stretch; transition: transform .25s var(--ease); }
.sc:hover { transform: translateY(-8px) scale(1.04); }
.rod { flex: none; height: 18px; border-radius: 9px; margin: 0 -12px; background: linear-gradient(#b07a43, #6f4421); box-shadow: 0 3px 5px rgba(0, 0, 0, .25); }
.pp { flex: 1; background: var(--p); display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 12px; padding: 10px; box-shadow: inset 0 0 0 1px rgba(140, 100, 50, .15), 0 6px 14px rgba(80, 50, 20, .15); }
.nm { font-size: 34px; line-height: 1.2; text-align: center; color: #4a2f17; font-family: "STKaiti", "KaiTi", var(--font); }
.mini { display: flex; gap: 6px; }
.mini em { font-style: normal; font-size: 18px; color: #fff; border-radius: 6px; padding: 1px 8px; }
.mini .i { background: var(--input); } .mini .c { background: var(--compute); } .mini .o { background: var(--output); }
.pp small { font-size: 20px; color: #8a6a48; }
.sc.fresh .pp { box-shadow: 0 0 0 5px var(--accent), 0 0 30px rgba(224, 112, 42, .6); animation: glow 1.2s ease-in-out infinite alternate; }
@keyframes glow { to { box-shadow: 0 0 0 5px var(--accent), 0 0 6px rgba(224, 112, 42, .2); } }
.new { position: absolute; right: -8px; top: 2px; background: var(--red); color: #fff; font-weight: 900; font-size: 22px; border-radius: 50%; width: 42px; height: 42px; display: flex; align-items: center; justify-content: center; }
.ticker { position: absolute; left: 50%; bottom: 24px; transform: translateX(-50%); background: var(--accent); color: #fff; font-size: 30px; font-weight: 800; padding: 10px 30px; border-radius: 999px; animation: up .4s var(--ease); }
@keyframes up { from { opacity: 0; transform: translate(-50%, 20px); } }
.mask { position: absolute; inset: 0; z-index: 50; background: rgba(40, 26, 12, .55); display: flex; align-items: center; justify-content: center; }
.unroll { display: flex; align-items: stretch; height: 760px; animation: open .8s var(--ease) both; transform-origin: center; }
@keyframes open { from { transform: scaleX(.08); } 40% { transform: scaleX(.08); } }
.unroll .rod { width: 34px; height: auto; margin: -24px 0; border-radius: 17px; background: linear-gradient(90deg, #6f4421, #b07a43, #6f4421); }
.paper { width: 1500px; background: linear-gradient(#fcf5e3, #f2e2bf); padding: 40px 60px; display: flex; flex-direction: column; overflow: hidden; }
.paper > * { animation: fadein .5s .55s both; }
@keyframes fadein { from { opacity: 0; } }
.paper small { font-size: 28px; color: #8a6a48; text-align: center; }
.paper h2 { font-size: 80px; text-align: center; color: #4a2f17; margin: 6px 0 26px; font-family: "STKaiti", "KaiTi", var(--font); }
.trio { flex: 1; display: flex; gap: 16px; align-items: stretch; }
.ar { align-self: center; font-size: 56px; color: #b39a7a; font-weight: 800; }
.part { flex: 1; background: rgba(255, 255, 255, .7); border-radius: 22px; padding: 22px 26px; border-top: 10px solid var(--c); }
.part.i { --c: var(--input); } .part.c { --c: var(--compute); } .part.o { --c: var(--output); }
.part h3 { font-size: 42px; color: var(--c); margin-bottom: 10px; }
.part p { font-size: 34px; line-height: 1.55; }
.ops { display: flex; justify-content: center; gap: 20px; margin-top: 24px; }
.ops button { min-height: 64px; padding: 0 34px; border-radius: 14px; font-size: 28px; font-weight: 800; }
.ops .close { background: var(--accent); color: #fff; }
.ops .hide { background: rgba(0, 0, 0, .06); color: #6b5a45; }
</style>
