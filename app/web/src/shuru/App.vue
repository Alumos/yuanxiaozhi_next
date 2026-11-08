<script setup lang="ts">
/* 输入环节 · 秋假攻略：左边给元小智“喂”偏好（选项 + 一句话，可用输入法语音），右边流式出攻略。
 * 教学重点：元小智收到的输入是什么；输入变了，攻略就变；输入不变，再试一次也可能不同。 */
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import YxzAvatar from '../common/components/YxzAvatar.vue'
import SeatPicker from '../common/components/SeatPicker.vue'
import LookAtScreen from '../common/components/LookAtScreen.vue'
import RetryButton from '../common/components/RetryButton.vue'
import Toasts from '../common/components/Toasts.vue'
import { events, get, stream, token } from '../common/api'
import { ME, loadMe } from '../common/seat'
import { toast } from '../common/toast'
import { ICON } from '../common/icons'

interface Place { name: string; prov: string; why: string; hits: string[]; trip: string }
interface Guide { n: number; kind: string; sent: string; n_info: number; focus?: string; places: Place[]; title: string; days: { n: number; text: string }[]; bring: string[]; tip: string; ms: number; first_ms: number }
interface Group { k: string; t: string; multi: boolean; items: string[] }

const stage = ref<'loading' | 'seat' | 'work'>('loading')
const meta = reactive({ groups: [] as Group[], text_ph: '', from_default: '苏州' })
const prefs = reactive<Record<string, string[]>>({})
const text = ref('')
const voice = ref(false)
const guides = ref<Guide[]>([])
const max = ref(6)
const view = ref(0)
const live = ref<Guide | null>(null)
const reading = ref(-1) // 等待时逐条“读”输入
const look = ref(false)
const moved = ref(false)
const ta = ref<HTMLTextAreaElement>()
const outBox = ref<HTMLElement>()
let off: (() => void) | null = null
const KEY = 'yxz_sr_draft'

async function init() {
  await loadMe()
  if (!ME.seat) { stage.value = 'seat'; return }
  Object.assign(meta, await get('/api/shuru/meta'))
  for (const g of meta.groups) prefs[g.k] = []
  try { const d = JSON.parse(localStorage.getItem(KEY) || '{}'); Object.assign(prefs, d.prefs || {}); text.value = d.text || '' } catch { /* */ }
  const st = await get('/api/shuru/state')
  guides.value = st.guides || []; max.value = st.max
  view.value = Math.max(0, guides.value.length - 1)
  stage.value = 'work'
  off = events('shuru', 'student', {
    look: (d) => (look.value = d.on),
    reset: () => { try { localStorage.removeItem(KEY) } catch { /* */ } location.reload() },
    seats_cleared: () => location.reload(),
    seat_moved: (d) => { if (d.keep !== token()) moved.value = true },
  })
}
onMounted(init)
onBeforeUnmount(() => off && off())
watch([prefs, text], () => { try { localStorage.setItem(KEY, JSON.stringify({ prefs, text: text.value })) } catch { /* */ } }, { deep: true })

function toggle(g: Group, it: string) {
  const a = prefs[g.k]
  const i = a.indexOf(it)
  if (i >= 0) a.splice(i, 1)
  else if (g.multi) a.push(it)
  else prefs[g.k] = [it]
}
const nInfo = computed(() => Object.values(prefs).reduce((s, a) => s + a.length, 0) + (text.value.trim() ? 1 : 0))
function useVoice() {
  voice.value = true
  ta.value?.focus()
  toast('点键盘上的话筒，说完再点“发给元小智”', 'info', 4000)
}

// 预览：元小智会收到的输入（和服务器拼法一致）
const preview = computed(() => meta.groups.filter((g) => prefs[g.k]?.length).map((g) => ({ t: g.t, v: prefs[g.k].join('、') })).concat(text.value.trim() ? [{ t: '补充', v: text.value.trim() }] : []))

const busy = computed(() => !!live.value)
async function send(retry = false, focus = '') {
  if (busy.value) return
  if (!retry && !focus && nInfo.value === 0) { toast('先告诉元小智一点你的想法吧', 'warn'); return }
  const g: Guide = reactive({ n: guides.value.length + 1, kind: retry ? 'retry' : focus ? 'focus' : 'ask', sent: '', n_info: 0, focus, places: [], title: '', days: [], bring: [], tip: '', ms: 0, first_ms: 0 })
  live.value = g
  reading.value = -1
  const t0 = Date.now()
  let readT = 0
  try {
    await stream('/api/shuru/ask', { prefs, text: text.value, voice: voice.value, retry, focus }, (ev, d) => {
      if (ev === 'input') {
        g.sent = d.sent; g.n_info = d.n_info
        // 等第一个推荐时，一条一条“读”输入
        const n = g.sent.split(/[；\n]/).length
        readT = window.setInterval(() => { reading.value = Math.min(n, reading.value + 1) }, 700)
      } else if (ev === 'place') { g.places.push(d); clearInterval(readT); reading.value = 99 }
      else if (ev === 'title') g.title = d.title
      else if (ev === 'day') g.days.push(d)
      else if (ev === 'bring') g.bring = d
      else if (ev === 'tip') g.tip = d
      else if (ev === 'final') Object.assign(g, d)
      nextTick(() => { const el = outBox.value; if (el && ev !== 'input') el.scrollTop = el.scrollHeight })
    })
    if (g.places.length) { guides.value.push({ ...g, ms: g.ms || Date.now() - t0 }); view.value = guides.value.length - 1 }
    voice.value = false
  } catch (e) { toast((e as Error).message, 'warn') } finally {
    clearInterval(readT)
    live.value = null
    nextTick(() => { if (outBox.value) outBox.value.scrollTop = 0 })
  }
}
const cur = computed<Guide | null>(() => live.value || guides.value[view.value] || null)
const sentLines = computed(() => (cur.value?.sent || '').split(/[；\n]/).filter(Boolean))
const left = computed(() => max.value - guides.value.length)
const KIND: Record<string, string> = { ask: '', retry: ' · 再试', focus: ' · 换地方' }
</script>

<template>
  <YxzHeader title="秋假攻略" link="input" :seat="ME.seat" :state="busy ? 'think' : 'listen'" />
  <Toasts />
  <SeatPicker v-if="stage === 'seat'" @done="() => { stage = 'loading'; init() }" />
  <main v-else-if="stage === 'work'" class="sr">
    <!-- 输入区 -->
    <section class="in">
      <div class="hi"><YxzAvatar state="listen" :size="48" /><p>秋假想去哪儿玩？把你的想法<b>告诉元小智</b>，它帮你挑地方、写攻略。</p></div>
      <div v-for="g in meta.groups" :key="g.k" class="grp">
        <h3>{{ g.t }}<small v-if="g.multi">（可多选）</small></h3>
        <div class="chips">
          <button v-for="it in g.items" :key="it" class="ch" :class="{ on: prefs[g.k]?.includes(it) }" @click="toggle(g, it)">{{ it }}</button>
        </div>
      </div>
    </section>

    <!-- 输出区 -->
    <section class="out">
      <div v-if="guides.length > 1 && !live" class="tabs">
        <button v-for="(g, i) in guides" :key="g.n" :class="{ on: view === i }" @click="view = i">第 {{ g.n }} 份{{ KIND[g.kind] }}</button>
      </div>
      <div ref="outBox" class="scroll">
        <!-- 元小智收到的输入 -->
        <div v-if="cur" class="got card">
          <h4><span v-html="ICON.send" />元小智收到的输入 <em>{{ cur.n_info }} 条信息</em></h4>
          <p v-for="(l, i) in sentLines" :key="i" :class="{ lit: !live || reading >= i }">{{ l }}</p>
        </div>
        <div v-else class="empty">
          <YxzAvatar state="idle" :size="90" />
          <p>左边选一选，下面写一句，<br />再点“发给元小智”</p>
          <div v-if="preview.length" class="pv card"><h4>元小智将会收到：</h4><p v-for="p in preview" :key="p.t"><b>{{ p.t }}：</b>{{ p.v }}</p></div>
        </div>

        <div v-if="live && !live.places.length" class="wait"><YxzAvatar state="think" :size="40" /><span>{{ reading < 0 ? '收到！' : '正在读你的每一条信息，在全国找合适的地方……' }}</span></div>

        <div v-if="cur?.places.length" class="places">
          <div v-for="(p, i) in cur.places" :key="p.name" class="pl card" :class="{ top: i === 0 }">
            <b class="rk">{{ i + 1 }}</b>
            <div class="pt">
              <h3>{{ p.name }} <small>{{ p.prov }}</small></h3>
              <p>{{ p.why }}</p>
              <div class="meta"><span v-for="h in p.hits" :key="h" class="hit">对上了：{{ h }}</span><span v-if="p.trip" class="trip">{{ p.trip }}</span></div>
            </div>
            <button v-if="i > 0 && !live && left > 0" class="btn sm soft" @click="send(false, p.name)">看它的攻略</button>
          </div>
        </div>

        <div v-if="cur?.title || cur?.days.length" class="plan card">
          <h3><span v-html="ICON.map" />{{ cur.places[0]?.name }} 攻略</h3>
          <p v-if="cur.title" class="tt">{{ cur.title }}</p>
          <div v-for="d in cur.days" :key="d.n" class="day"><b>第 {{ d.n }} 天</b><span>{{ d.text }}</span></div>
          <div v-if="cur.bring.length" class="bring"><b>带上：</b><span v-for="b in cur.bring" :key="b" class="tag">{{ b }}</span></div>
          <p v-if="cur.tip" class="tip"><b>提醒：</b>{{ cur.tip }}</p>
          <p v-if="!live && cur.ms" class="stat muted">输入 {{ cur.n_info }} 条信息 → 计算 {{ (cur.ms / 1000).toFixed(1) }} 秒 → 输出 3 个地点 + {{ cur.days.length }} 天攻略</p>
        </div>
      </div>
      <footer class="bar">
        <div v-if="guides.length" class="rrow">
          <RetryButton :left="left" :busy="busy" confirm-text="输入一个字都不改，让元小智再写一份，看看会不会一样？" @go="send(true)" />
        </div>
        <div class="say">
          <textarea ref="ta" v-model="text" maxlength="80" rows="2" :placeholder="meta.text_ph" />
          <button class="mic" :class="{ on: voice }" @click="useVoice" aria-label="用输入法语音"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><rect x="9" y="3" width="6" height="11" rx="3" /><path d="M5 11a7 7 0 0 0 14 0M12 18v3" /></svg>语音</button>
          <button class="btn send" :disabled="busy || left <= 0" @click="send(false)"><span v-html="ICON.send" />{{ busy ? '正在写…' : guides.length ? '再发给元小智' : '发给元小智' }}</button>
        </div>
      </footer>
    </section>
  </main>
  <LookAtScreen v-if="look" />
  <LookAtScreen v-if="moved" text="已在别的平板上继续" />
</template>

<style scoped>
.sr { height: calc(100vh - 64px); height: calc(100dvh - 64px); display: flex; }
.in { width: 46%; overflow: auto; -webkit-overflow-scrolling: touch; padding: 14px 18px 24px; display: flex; flex-direction: column; gap: 6px; }
.in > * { flex-shrink: 0; }
.hi { display: flex; align-items: center; gap: 12px; font-size: 19px; margin-bottom: 4px; }
.hi b { color: var(--accent); }
h3 { font-size: 19px; margin-top: 6px; }
h3 small { font-size: 15px; color: var(--muted); font-weight: 500; }
.chips { display: flex; flex-wrap: wrap; gap: 8px; }
.ch { min-height: 46px; padding: 0 16px; border-radius: 999px; background: #fff; box-shadow: inset 0 0 0 2px var(--line); font-size: 18px; font-weight: 600; color: var(--ink-2); transition: transform var(--t-light); }
.ch:active { transform: scale(.94); }
.ch.on { background: var(--accent); color: #fff; box-shadow: none; }
.say { display: flex; gap: 8px; align-items: stretch; }
.say textarea { flex: 1; min-width: 0; min-height: 76px; border-radius: 14px; border: 2px solid var(--line); padding: 10px 14px; font-size: 19px; line-height: 1.5; resize: none; outline: none; background: #fff; }
.say textarea:focus { border-color: var(--accent); }
.mic { width: 70px; flex: none; border-radius: 14px; background: var(--accent-soft); color: var(--accent); font-weight: 700; font-size: 16px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; }
.mic svg { width: 30px; height: 30px; }
.mic.on { background: var(--accent); color: #fff; }
.out { width: 54%; display: flex; flex-direction: column; background: var(--paper); border-left: 1px solid var(--line); min-width: 0; }
.tabs { display: flex; gap: 8px; padding: 10px 14px 0; flex-wrap: wrap; }
.tabs button { min-height: 44px; padding: 0 16px; border-radius: 12px; background: #fff; font-weight: 700; font-size: 17px; box-shadow: inset 0 0 0 2px var(--line); }
.tabs button.on { background: var(--accent); color: #fff; box-shadow: none; }
.scroll { flex: 1; overflow: auto; -webkit-overflow-scrolling: touch; padding: 12px 16px; display: flex; flex-direction: column; gap: 12px; }
.scroll > * { flex-shrink: 0; }
.got { padding: 12px 16px; border-left: 6px solid var(--accent); }
.got h4 { display: flex; align-items: center; gap: 8px; font-size: 18px; color: var(--accent); margin-bottom: 4px; }
.got h4 :deep(svg) { width: 20px; height: 20px; }
.got h4 em { font-style: normal; margin-left: auto; font-size: 16px; background: var(--accent-soft); padding: 2px 10px; border-radius: 8px; }
.got p { font-size: 17px; color: var(--muted); transition: color .3s, transform .3s var(--ease); }
.got p.lit { color: var(--ink); }
.empty { text-align: center; color: var(--muted); font-size: 21px; padding-top: 30px; display: flex; flex-direction: column; align-items: center; gap: 12px; }
.pv { text-align: left; padding: 12px 16px; font-size: 17px; color: var(--ink); width: 100%; }
.pv h4 { color: var(--accent); margin-bottom: 4px; }
.wait { display: flex; align-items: center; gap: 10px; font-size: 19px; color: var(--accent); font-weight: 700; }
.places { display: flex; flex-direction: column; gap: 10px; }
.pl { display: flex; gap: 12px; align-items: flex-start; padding: 12px 16px; animation: up .35s var(--ease) both; }
@keyframes up { from { opacity: 0; transform: translateY(14px); } }
.pl.top { box-shadow: inset 0 0 0 3px var(--accent), var(--shadow); }
.rk { font-size: 30px; color: var(--accent); width: 28px; line-height: 1.2; }
.pt { flex: 1; min-width: 0; }
.pt h3 { font-size: 24px; margin: 0; }
.pt h3 small { font-size: 16px; color: var(--muted); font-weight: 500; }
.pt p { font-size: 18px; color: var(--ink-2); }
.meta { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px; }
.hit { font-size: 15px; background: var(--accent-soft); color: var(--accent); border-radius: 8px; padding: 1px 8px; font-weight: 600; }
.trip { font-size: 15px; color: var(--muted); }
.plan { padding: 14px 18px; display: flex; flex-direction: column; gap: 8px; animation: up .35s var(--ease) both; }
.plan h3 { display: flex; align-items: center; gap: 8px; font-size: 22px; margin: 0; color: var(--accent); }
.plan h3 :deep(svg) { width: 24px; height: 24px; }
.tt { font-size: 18px; font-weight: 700; }
.day { display: flex; gap: 12px; font-size: 18px; line-height: 1.55; padding: 8px 0; border-top: 1px dashed var(--line); animation: up .3s var(--ease) both; }
.day b { flex: none; width: 64px; color: var(--accent); }
.bring { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; font-size: 17px; }
.tip { font-size: 17px; background: #fff6e5; border-radius: 10px; padding: 8px 12px; }
.stat { font-size: 15px; }
.bar { display: flex; flex-direction: column; gap: 8px; padding: 10px 14px calc(12px + env(safe-area-inset-bottom)); border-top: 1px solid var(--line); background: #fff; }
.rrow { display: flex; justify-content: flex-start; }
.send { flex: none; flex-direction: column; gap: 4px; min-width: 120px; padding: 0 14px; font-size: 18px; border-radius: 14px; }
@media (orientation: portrait), (max-width: 900px) {
  .sr { flex-direction: column; height: auto; }
  .in, .out { width: 100%; }
  .out { min-height: 80vh; border-left: 0; border-top: 1px solid var(--line); }
}
</style>
