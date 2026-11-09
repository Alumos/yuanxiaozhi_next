<script setup lang="ts">
/* 畅想 · 学生平板：给元小智设计一个新功能（功能名 + 输入 / 计算 / 输出），提交到大屏。 */
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import YxzAvatar from '../common/components/YxzAvatar.vue'
import SeatPicker from '../common/components/SeatPicker.vue'
import LookAtScreen from '../common/components/LookAtScreen.vue'
import Toasts from '../common/components/Toasts.vue'
import StageLock from '../common/components/StageLock.vue'
import { loadLock, onLock } from '../common/lock'
import { events, get, post, token } from '../common/api'
import { ME, loadMe } from '../common/seat'
import { toast } from '../common/toast'
import { ICON } from '../common/icons'

interface Idea { id: string; name: string; input: string; compute: string; output: string }
const stage = ref<'loading' | 'seat' | 'work'>('loading')
const form = reactive({ name: '', input: '', compute: '', output: '' })
const mine = ref<Idea[]>([])
const max = ref(5)
const sending = ref(false)
const sent = ref<Idea | null>(null)
const showEx = ref(false)
const look = ref(false)
const moved = ref(false)
const bad = ref('')
let off: (() => void) | null = null
const KEY = 'yxz_cx_draft'

const COLS = [
  { k: 'input' as const, t: '输入', q: '元小智要收到什么？', ph: '例如：同学拍下的植物叶子照片，再说一句“它怎么了？”', c: 'var(--input)', s: 'var(--input-soft)' },
  { k: 'compute' as const, t: '计算', q: '它要怎么想、怎么算？', ph: '例如：把叶子上的斑点和植物病害图库一张张比对，找出最像的病', c: 'var(--compute)', s: 'var(--compute-soft)' },
  { k: 'output' as const, t: '输出', q: '最后给出什么？给谁？', ph: '例如：告诉同学植物得了什么病、该怎么浇水，再提醒负责的老师', c: 'var(--output)', s: 'var(--output-soft)' },
]

async function loadMine() { const d = await get('/api/changxiang/mine'); mine.value = d.ideas || []; max.value = d.max }
async function init() {
  await loadMe()
  if (!ME.seat) { stage.value = 'seat'; return }
  try { Object.assign(form, JSON.parse(localStorage.getItem(KEY) || '{}')) } catch { /* */ }
  await loadMine()
  stage.value = 'work'
  loadLock('changxiang')
  off = events('changxiang', 'student', {
    lock: onLock,
    look: (d) => (look.value = d.on),
    reset: () => { try { localStorage.removeItem(KEY) } catch { /* */ } location.reload() },
    seats_cleared: () => location.reload(),
    seat_moved: (d) => { if (d.keep !== token()) moved.value = true },
  })
}
onMounted(init)
onBeforeUnmount(() => off && off())
watch(form, (v) => { try { localStorage.setItem(KEY, JSON.stringify(v)) } catch { /* */ } })

const left = computed(() => max.value - mine.value.length)
const ready = computed(() => form.name.trim().length >= 2 && COLS.every((c) => form[c.k].trim().length >= 2))
async function submit() {
  if (sending.value) return
  bad.value = ''
  if (!form.name.trim()) { bad.value = 'name'; toast('先给新功能起个名字', 'warn'); return }
  for (const c of COLS) if (form[c.k].trim().length < 2) { bad.value = c.k; toast(`“${c.t}”还没写：${c.q}`, 'warn'); return }
  sending.value = true
  try {
    const d = await post('/api/changxiang/submit', { ...form })
    sent.value = d.idea
    mine.value.push(d.idea)
    form.name = form.input = form.compute = form.output = ''
  } catch (e) { toast((e as Error).message, 'warn') } finally { sending.value = false }
}
function again() { sent.value = null }
</script>

<template>
  <YxzHeader title="畅想 · 元小智的新本领" link="imagine" :seat="ME.seat" :state="sending ? 'think' : 'idle'" />
  <Toasts />
  <StageLock v-if="ME.seat" />
  <SeatPicker v-if="stage === 'seat'" @done="() => { stage = 'loading'; init() }" />
  <main v-else-if="stage === 'work'" class="cx">
    <!-- 提交成功 -->
    <section v-if="sent" class="done">
      <div class="scroll">
        <i class="rod" />
        <div class="paper"><small>已送上大屏</small><h2>{{ sent.name }}</h2>
          <div class="trio"><p><b>输入</b>{{ sent.input }}</p><p><b>计算</b>{{ sent.compute }}</p><p><b>输出</b>{{ sent.output }}</p></div>
        </div>
        <i class="rod" />
      </div>
      <p class="tip">你的畅想已经飞到大屏的卷轴上啦！你已经交了 {{ mine.length }} 个。</p>
      <div class="acts">
        <button v-if="left > 0" class="btn big" @click="again"><span v-html="ICON.bulb" />再畅想一个</button>
        <span v-else class="muted">已经交满 {{ max }} 个，去看看大屏吧</span>
      </div>
    </section>

    <!-- 填写 -->
    <template v-else>
      <div class="intro">
        <YxzAvatar state="speak" :size="56" />
        <p>你希望元小智学会什么<b>新本领</b>？想清楚它的<b class="i">输入</b>、<b class="c">计算</b>、<b class="o">输出</b>，写下来送上大屏！</p>
        <button class="btn sm soft" @click="showEx = !showEx">{{ showEx ? '收起例子' : '看个例子' }}</button>
      </div>
      <p v-if="showEx" class="ex">例子 · <b>植物医生</b>：输入 = 叶子照片 + 一句提问；计算 = 和病害图库比对，找出最像的病；输出 = 告诉同学病因和养护办法。</p>

      <label class="name" :class="{ bad: bad === 'name' }">
        <span>新功能的名字</span>
        <input v-model="form.name" maxlength="16" placeholder="给它起个响亮的名字，例如：植物医生" />
      </label>

      <div class="cols">
        <template v-for="(c, i) in COLS" :key="c.k">
          <span v-if="i" class="arrow" aria-hidden="true">→</span>
          <label class="col" :class="{ bad: bad === c.k }" :style="{ '--c': c.c, '--s': c.s }">
            <span class="ct"><b>{{ c.t }}</b>{{ c.q }}</span>
            <textarea v-model="form[c.k]" maxlength="120" :placeholder="c.ph" />
            <small class="num">{{ form[c.k].length }} / 120</small>
          </label>
        </template>
      </div>

      <footer class="bar">
        <span class="muted">{{ mine.length ? `已交 ${mine.length} 个，还能交 ${left} 个` : '可以用输入法的语音来写' }}</span>
        <button class="btn big" :disabled="sending || left <= 0" :class="{ guide: ready }" @click="submit"><span v-html="ICON.send" />提交畅想</button>
      </footer>
    </template>
  </main>
  <LookAtScreen v-if="look" />
  <LookAtScreen v-if="moved" text="已在别的平板上继续" />
</template>

<style scoped>
.cx { max-width: 1280px; margin: 0 auto; padding: 16px 22px 110px; }
.intro { display: flex; align-items: center; gap: 14px; }
.intro p { flex: 1; font-size: 21px; }
.intro b { color: var(--accent); }
.intro b.i { color: var(--input); } .intro b.c { color: var(--compute); } .intro b.o { color: var(--output); }
.ex { margin-top: 10px; background: #fff; border-radius: 12px; padding: 12px 16px; font-size: 18px; color: var(--ink-2); box-shadow: var(--shadow); }
.name { display: flex; flex-direction: column; gap: 6px; margin-top: 16px; }
.name span { font-size: 20px; font-weight: 800; color: var(--accent); }
.name input { min-height: 78px; border-radius: 18px; border: 3px solid var(--accent-soft); background: #fff; padding: 0 22px; font-size: 34px; font-weight: 800; outline: none; color: var(--ink); }
.name input:focus { border-color: var(--accent); }
.cols { display: flex; align-items: stretch; gap: 8px; margin-top: 18px; }
.arrow { align-self: center; font-size: 34px; color: #c7bfb2; font-weight: 800; }
.col { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 8px; background: var(--s); border-radius: 18px; padding: 14px; border-top: 8px solid var(--c); }
.ct { font-size: 17px; color: var(--ink-2); }
.ct b { display: block; font-size: 28px; color: var(--c); }
.col textarea { flex: 1; min-height: 230px; border-radius: 14px; border: 2px solid transparent; background: #fff; padding: 12px 14px; font-size: 21px; line-height: 1.55; resize: none; outline: none; }
.col textarea:focus { border-color: var(--c); }
.col small { align-self: flex-end; color: var(--muted); font-size: 14px; }
.bad input, .bad textarea { border-color: var(--red) !important; animation: shake .3s; }
@keyframes shake { 25% { transform: translateX(-6px); } 75% { transform: translateX(6px); } }
.bar { position: fixed; left: 0; right: 0; bottom: var(--icp-h); display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 12px 24px; background: rgba(255, 253, 249, .97); border-top: 1px solid var(--line); z-index: 10; }
.bar .btn { min-width: 260px; }
.done { display: flex; flex-direction: column; align-items: center; gap: 20px; padding-top: 20px; }
.scroll { width: min(900px, 100%); display: flex; flex-direction: column; align-items: stretch; animation: unroll .7s var(--ease) both; transform-origin: top center; }
@keyframes unroll { from { transform: scaleY(.1); opacity: 0; } }
.rod { height: 22px; border-radius: 11px; background: linear-gradient(#a8743f, #6f4421); margin: 0 -16px; box-shadow: 0 3px 6px rgba(0, 0, 0, .2); }
.paper { background: linear-gradient(#fbf3df, #f3e4c2); padding: 22px 30px; text-align: center; }
.paper small { color: var(--accent); font-weight: 800; font-size: 18px; }
.paper h2 { font-size: 40px; margin: 6px 0 14px; }
.trio { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; text-align: left; }
.trio p { font-size: 18px; background: rgba(255, 255, 255, .6); border-radius: 10px; padding: 10px 12px; }
.trio b { display: block; font-size: 20px; }
.trio p:nth-child(1) b { color: var(--input); } .trio p:nth-child(2) b { color: var(--compute); } .trio p:nth-child(3) b { color: var(--output); }
.tip { font-size: 21px; }
@media (orientation: portrait), (max-width: 860px) {
  .cols { flex-direction: column; }
  .arrow { transform: rotate(90deg); }
  .col textarea { min-height: 130px; }
  .trio { grid-template-columns: 1fr; }
}
</style>
