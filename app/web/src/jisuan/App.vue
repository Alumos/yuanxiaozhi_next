<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import YxzHeader from '../common/components/YxzHeader.vue'
import YxzAvatar from '../common/components/YxzAvatar.vue'
import SeatPicker from '../common/components/SeatPicker.vue'
import BookCover from '../common/components/BookCover.vue'
import LookAtScreen from '../common/components/LookAtScreen.vue'
import RetryButton from '../common/components/RetryButton.vue'
import Toasts from '../common/components/Toasts.vue'
import Analysis from './Analysis.vue'
import { ApiError, events, get, post, stream, token } from '../common/api'
import { ME, loadMe } from '../common/seat'
import { toast } from '../common/toast'
import { ICON } from '../common/icons'
import { META, catOf, newFeed, type BookView, type Feed, type Result } from './types'

const st = reactive({
  stage: 'loading' as string, attempt: 1, max: 3, picked: 0, chosen: [] as BookView[], attempts: [] as any[], stars: 0, againUsed: false,
  paused: false, moved: false, projected: false,
})
const round = reactive({ n: 0, books: [] as BookView[], opened: [] as string[], open: '' as string, t0: 0, choosing: '' as string, loading: false })
const quip = ref('')
const feed = ref<Feed | null>(null)
const feedKey = ref(0)
const replaying = ref(false)
const view = ref(0) // which attempt tab (index into st.attempts)
const likes = reactive<Record<string, boolean | undefined>>({})
const againOpen = ref(false)
const againText = ref('')
let off: (() => void) | null = null

async function loadMeta() {
  if (META.loaded) return
  const m = await get('/api/jisuan/meta')
  META.cats = m.categories; META.spines = m.spines; META.books = m.books; META.tags = m.tags; META.loaded = true
}
function apply(d: any) {
  st.stage = d.stage; st.attempt = d.attempt; st.max = d.max_attempts; st.picked = d.picked; st.chosen = d.chosen
  st.attempts = d.attempts; st.stars = d.stars; st.againUsed = d.again_used; st.paused = d.paused
  view.value = Math.max(0, st.attempts.length - 1)
}
async function refresh() {
  const d = await get('/api/jisuan/state')
  apply(d)
  if (st.stage === 'picking') await loadRound()
  else if (st.stage === 'analyzing') startAnalysis()
}
async function init() {
  await loadMe()
  if (!ME.seat) { st.stage = 'seat'; return }
  await loadMeta()
  await refresh()
  subscribe()
}
function subscribe() {
  off && off()
  off = events('jisuan', 'student', {
    paused: (d) => (st.paused = d.paused),
    seat_moved: (d) => { if (d.keep !== token()) st.moved = true },
    reset: () => location.reload(),
    seats_cleared: () => location.reload(),
    projected: () => { st.projected = true; setTimeout(() => (st.projected = false), 8000) },
  })
}
onMounted(init)
onBeforeUnmount(() => off && off())

async function onSeat(seat: number, group: number) { ME.seat = seat; ME.group = group; await loadMeta(); await refresh(); subscribe() }
async function begin() { apply(await post('/api/jisuan/start')); await loadRound() }

// ---------------- 选书 ----------------
async function loadRound() {
  round.loading = true
  try {
    const d = await get('/api/jisuan/round')
    round.n = d.n; round.books = d.books; round.opened = []; round.open = ''; round.choosing = ''; round.t0 = performance.now()
  } catch (e) {
    if (e instanceof ApiError && e.code === 'done') return refresh()
    toast((e as Error).message, 'warn')
  } finally { round.loading = false }
}
function toggleIntro(b: BookView) {
  round.open = round.open === b.id ? '' : b.id
  if (round.open && !round.opened.includes(b.id)) round.opened.push(b.id)
}
const QUIPS = ['记下了', '嗯……', '有意思', '好的', '收到', '我记住啦']
async function choose(b: BookView) {
  if (round.choosing || st.paused) return
  round.choosing = b.id
  const ms = Math.round(performance.now() - round.t0)
  try {
    const d = await post('/api/jisuan/pick', { round: round.n, book_id: b.id, ms, opened: round.opened })
    st.chosen.push(b); st.picked++
    quip.value = QUIPS[Math.floor(Math.random() * QUIPS.length)]
    setTimeout(() => (quip.value = ''), 1400)
    await new Promise((r) => setTimeout(r, 600))
    if (d.next === 'analyze') { st.stage = 'analyzing'; startAnalysis() }
    else await loadRound()
  } catch (e) {
    round.choosing = ''
    if (e instanceof ApiError && e.code === 'paused') st.paused = true
    else { toast((e as Error).message, 'warn'); refresh() }
  }
}

// ---------------- 分析 ----------------
function feedFrom(path: string, body: unknown) {
  const f = newFeed()
  stream(path, body, (ev, d) => {
    if (ev === 'trace') f.trace = d
    else if (ev === 'thought') f.thoughts.push({ kind: d.kind, text: d.text })
    else if (ev === 'persona') f.persona = d
    else if (ev === 'pick') f.picks.push(d)
    else if (ev === 'final') f.final = d.result
    else if (ev === 'done') f.done = true
  }).catch((e) => { f.error = (e as Error).message; f.done = true; toast(f.error, 'warn') })
  return f
}
function startAnalysis() { replaying.value = false; feedKey.value++; feed.value = feedFrom('/api/jisuan/analyze', {}) }
async function onFinish() {
  feed.value = null
  againOpen.value = false
  apply(await get('/api/jisuan/state'))
}
function replay() {
  const a = st.attempts[view.value]
  replaying.value = true
  feedKey.value++
  if (a.attempt === st.attempt) { feed.value = feedFrom('/api/jisuan/analyze', {}); return }
  const f = newFeed()
  get(`/api/jisuan/trace?attempt=${a.attempt}`).then((d) => {
    f.trace = d.trace
    for (const t of d.trace.llm?.thoughts || []) f.thoughts.push(t)
    f.persona = { name: d.result.persona, tags: d.result.top_tags }
    f.picks.push(...d.result.picks); f.final = d.result; f.done = true
  })
  feed.value = f
}

// ---------------- 结果 ----------------
const cur = computed(() => st.attempts[view.value])
const res = computed<Result | null>(() => cur.value?.result || null)
const books = computed<Record<string, BookView>>(() => cur.value?.books || {})
const isLatest = computed(() => view.value === st.attempts.length - 1)
const compare = computed(() => {
  if (st.attempts.length < 2) return ''
  const a = st.attempts[view.value - 1] || st.attempts[view.value + 1]
  const b = cur.value
  if (!a?.result || !b?.result) return ''
  const [x, y] = view.value > 0 ? [a, b] : [b, a]
  return `第 ${x.attempt} 次你是“${x.result.persona}”，第 ${y.attempt} 次是“${y.result.persona}”`
})
const maxV = computed(() => Math.max(...(res.value?.picks.map((p) => p.interest) || [100])))
async function setStars(n: number) { st.stars = n; await post('/api/jisuan/stars', { stars: n }); toast('谢谢你的评分！', 'ok') }
async function retry() {
  try { apply(await post('/api/jisuan/retry')); await loadRound() } catch (e) { toast((e as Error).message, 'warn') }
}
const doneFlash = ref(false)
async function finish() {
  await post('/api/jisuan/finish'); st.stage = 'done'
  doneFlash.value = true
  setTimeout(() => { doneFlash.value = false; toast('可以点顶栏，去体验其他环节', 'ok', 4000) }, 4000)
}
function openAgain() {
  for (const p of res.value?.picks || []) if (likes[p.book_id] === undefined) likes[p.book_id] = undefined
  againOpen.value = true
}
async function sendAgain() {
  const lk: Record<string, boolean> = {}
  for (const p of st.attempts[st.attempts.length - 1].first?.picks || res.value?.picks || []) if (likes[p.book_id] !== undefined) lk[p.book_id] = !!likes[p.book_id]
  if (!Object.keys(lk).length && !againText.value.trim()) { toast('先告诉元小智哪本想读、哪本不想读', 'warn'); return }
  // 先检查（个人信息会被拦下）
  const f = newFeed()
  try {
    await new Promise<void>((resolve, reject) => {
      let started = false
      stream('/api/jisuan/again', { likes: lk, text: againText.value }, (ev, d) => {
        if (!started) { started = true; againOpen.value = false; replaying.value = false; feedKey.value++; feed.value = f; resolve() }
        if (ev === 'trace') f.trace = d
        else if (ev === 'thought') f.thoughts.push({ kind: d.kind, text: d.text })
        else if (ev === 'persona') f.persona = d
        else if (ev === 'pick') f.picks.push(d)
        else if (ev === 'final') f.final = d.result
        else if (ev === 'done') f.done = true
      }).then(() => { if (!started) resolve() }).catch(reject)
    })
  } catch (e) { toast((e as Error).message, 'warn') }
}
const showFirst = ref(false)
const shownRes = computed<Result | null>(() => (showFirst.value && cur.value?.first ? cur.value.first : res.value))
</script>

<template>
  <YxzHeader title="智能图书管理员" link="compute" :seat="ME.seat" :state="st.stage === 'analyzing' ? 'think' : 'idle'" />
  <Toasts />
  <main class="pg">
    <SeatPicker v-if="st.stage === 'seat'" @done="onSeat" />

    <!-- S1 开场 -->
    <section v-else-if="st.stage === 'intro'" class="intro">
      <YxzAvatar state="speak" :size="120" />
      <div class="bubble card">
        <p>我是智能图书管理员<b>元小智</b>。</p>
        <p>接下来给你看 <b>5 组书</b>，每组选一本你<b class="hl">最想读</b>的。</p>
        <p>选完后，我来猜猜你会喜欢什么书。</p>
        <small class="muted">共 5 轮 · 大约 2 分钟 · 选你最想读的，不是选你读过的</small>
      </div>
      <button class="btn big guide" @click="begin">开始<span v-html="ICON.play" /></button>
    </section>

    <!-- S2 选书 -->
    <section v-else-if="st.stage === 'picking'" class="pick">
      <div class="prog">
        <b class="num">第 {{ round.n }} / 5 轮</b>
        <div class="slots">
          <span v-for="i in 5" :key="i" class="slot" :class="{ now: i === round.n }">
            <BookCover v-if="st.chosen[i - 1]" :id="st.chosen[i - 1].id" :title="st.chosen[i - 1].title" :cat="catOf(st.chosen[i - 1])" :w="34" />
          </span>
        </div>
        <span class="q"><YxzAvatar :state="quip ? 'speak' : 'listen'" :size="40" /><Transition name="fade"><em v-if="quip">{{ quip }}</em></Transition></span>
      </div>
      <p class="ask">这 5 本书里，你<b>最想读</b>哪一本？</p>
      <div class="row" :key="round.n">
        <article v-for="(b, i) in round.books" :key="b.id" class="bk card" :class="{ picked: round.choosing === b.id, faded: round.choosing && round.choosing !== b.id }" :style="{ animationDelay: i * 60 + 'ms' }">
          <div class="cvw"><BookCover :id="b.id" :title="b.title" :cat="catOf(b)" :w="112" /></div>
          <h3>{{ b.title }}</h3>
          <p class="au">{{ b.author }}</p>
          <p class="tags"><span v-for="t in b.tags" :key="t" class="chip">#{{ t }}</span></p>
          <p class="hook">{{ b.hook }}</p>
          <div v-if="round.open === b.id" class="intro-pop" @click="toggleIntro(b)"><p>{{ b.summary }}</p><small>点一下收起</small></div>
          <div class="acts">
            <button class="btn soft sm" @click="toggleIntro(b)">{{ round.open === b.id ? '收起' : '看简介' }}</button>
            <button class="btn sm" @click="choose(b)">选这本</button>
          </div>
        </article>
      </div>
    </section>

    <!-- S4 结果 -->
    <section v-else-if="(st.stage === 'result' || st.stage === 'done' || st.stage === 'again') && res" class="res">
      <div v-if="st.attempts.length > 1" class="tabs">
        <button v-for="(a, i) in st.attempts" :key="a.attempt" :class="{ on: view === i }" :disabled="!a.result" @click="view = i; showFirst = false">第 {{ a.attempt }} 次</button>
        <span v-if="compare" class="cmp">{{ compare }}</span>
      </div>
      <div class="persona card">
        <YxzAvatar state="speak" :size="72" />
        <div>
          <p class="muted">元小智猜你是</p>
          <h2>“{{ shownRes!.persona }}”</h2>
          <p class="likes">你喜欢：<span v-for="t in shownRes!.top_tags" :key="t" class="chip">{{ t }}</span></p>
        </div>
        <div v-if="cur?.has_again" class="ver">
          <button :class="{ on: showFirst }" @click="showFirst = true">第一次推荐</button>
          <button :class="{ on: !showFirst }" @click="showFirst = false">再想想之后</button>
        </div>
      </div>
      <p v-if="!showFirst && shownRes!.note" class="note"><span v-html="ICON.bulb" />{{ shownRes!.note }}</p>
      <div class="picks">
        <div v-for="(p, i) in shownRes!.picks" :key="p.book_id" class="pk card">
          <b class="rank">{{ i + 1 }}</b>
          <BookCover :id="p.book_id" :title="books[p.book_id]?.title || ''" :cat="catOf(books[p.book_id])" :w="78" />
          <div class="pt">
            <h3>《{{ books[p.book_id]?.title }}》<small>{{ books[p.book_id]?.author }}</small></h3>
            <p class="why">{{ p.reason }}</p>
          </div>
          <div class="pi">
            <span class="muted">感兴趣程度</span>
            <b class="num">{{ p.interest }}%</b>
            <span class="bar"><i :style="{ transform: `scaleX(${p.interest / 100})` }" /></span>
            <div v-if="isLatest && !st.againUsed && st.stage !== 'done' && !showFirst" class="lk">
              <button :class="{ on: likes[p.book_id] === true }" @click="likes[p.book_id] = likes[p.book_id] === true ? undefined : true"><span v-html="ICON.thumbUp" />想读</button>
              <button :class="{ on: likes[p.book_id] === false, no: true }" @click="likes[p.book_id] = likes[p.book_id] === false ? undefined : false"><span v-html="ICON.thumbDown" />不想读</button>
            </div>
          </div>
        </div>
      </div>
      <p class="why100 muted">为什么没有 100%？因为这是元小智“猜”的，猜就有可能不准。</p>
      <footer class="bar-b">
        <button class="btn ghost" @click="replay"><span v-html="ICON.rewind" />再看一遍分析</button>
        <div v-if="isLatest" class="stars">
          <span>猜得准吗？</span>
          <button v-for="n in 5" :key="n" :class="{ on: n <= st.stars }" @click="setStars(n)" :aria-label="n + ' 星'"><span v-html="n <= st.stars ? ICON.starFill : ICON.star" /></button>
        </div>
        <span class="sp" />
        <template v-if="isLatest">
          <button v-if="!st.againUsed && st.stage !== 'done'" class="btn soft" @click="openAgain"><span v-html="ICON.bulb" />再想想</button>
          <RetryButton :left="st.max - st.attempt" confirm-text="重新选 5 轮书，看看元小智这次会怎么猜？" @go="retry" />
          <button v-if="st.stage !== 'done'" class="btn" @click="finish">完成</button>
          <span v-else class="donetip">已完成 · 可以点顶栏去别的环节</span>
        </template>
      </footer>
    </section>

    <section v-else-if="st.stage !== 'analyzing'" class="loading"><YxzAvatar state="think" :size="80" /></section>
  </main>

  <!-- S5 再想想 -->
  <div v-if="againOpen" class="mask" @click.self="againOpen = false">
    <div class="again card">
      <h2>告诉元小智哪里不准</h2>
      <div class="ag-books">
        <div v-for="p in res?.picks" :key="p.book_id" class="ag">
          <BookCover :id="p.book_id" :title="books[p.book_id]?.title || ''" :cat="catOf(books[p.book_id])" :w="70" />
          <b>{{ books[p.book_id]?.title }}</b>
          <div class="lk">
            <button :class="{ on: likes[p.book_id] === true }" @click="likes[p.book_id] = true"><span v-html="ICON.thumbUp" />想读</button>
            <button :class="{ on: likes[p.book_id] === false, no: true }" @click="likes[p.book_id] = false"><span v-html="ICON.thumbDown" />不想读</button>
          </div>
        </div>
      </div>
      <label class="say">
        <span>还想补充一句吗？（可以打字，也可以用输入法的语音）</span>
        <input v-model="againText" maxlength="60" placeholder="例如：我不喜欢太长的书" @keyup.enter="sendAgain" />
      </label>
      <div class="row"><button class="btn ghost" @click="againOpen = false">先不了</button><button class="btn" @click="sendAgain"><span v-html="ICON.bulb" />再想想</button></div>
    </div>
  </div>

  <Analysis v-if="feed" :key="feedKey" :feed="feed" :replay="replaying" @finish="onFinish" />
  <Transition name="fade"><LookAtScreen v-if="doneFlash" text="谢谢你！请看大屏" sub="我会继续学习你的口味" /></Transition>
  <LookAtScreen v-if="st.paused" text="请看大屏" />
  <LookAtScreen v-if="st.projected" text="你的分析正在大屏上播放" sub="和大家说说，元小智猜得准吗？" />
  <LookAtScreen v-if="st.moved" text="已在别的平板上继续" sub="这台平板可以交给老师" />
</template>

<style scoped>
.pg { height: calc(100vh - 64px); height: calc(100dvh - 64px); overflow: auto; -webkit-overflow-scrolling: touch; }
.loading { display: flex; justify-content: center; padding-top: 120px; }
.intro { max-width: 760px; margin: 0 auto; padding: 50px 24px; display: flex; flex-direction: column; align-items: center; gap: 26px; text-align: center; }
.bubble { padding: 28px 34px; font-size: 24px; line-height: 1.7; }
.bubble .hl { color: var(--accent); }
.bubble small { display: block; margin-top: 10px; font-size: 18px; }
.pick { padding: 16px 20px 24px; max-width: 1400px; margin: 0 auto; }
.prog { display: flex; align-items: center; gap: 18px; }
.prog > b { font-size: 26px; }
.slots { display: flex; gap: 8px; }
.slot { width: 44px; height: 56px; border-radius: 8px; border: 2px dashed #d3cdc2; display: flex; align-items: center; justify-content: center; }
.slot.now { border-color: var(--accent); border-style: solid; }
.q { display: flex; align-items: center; gap: 8px; margin-left: auto; min-width: 160px; justify-content: flex-end; }
.q em { font-style: normal; font-weight: 700; color: var(--accent); font-size: 20px; background: var(--accent-soft); padding: 4px 14px; border-radius: 12px; }
.ask { font-size: 22px; margin: 12px 0 14px; }
.ask b { color: var(--accent); }
.row { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 14px; }
.bk { position: relative; padding: 14px 12px 12px; display: flex; flex-direction: column; align-items: center; text-align: center; gap: 4px; animation: up .35s var(--ease) both; transition: transform .5s var(--ease), opacity .4s; overflow: hidden; }
@keyframes up { from { opacity: 0; transform: translateY(24px); } }
.bk.picked { transform: translateY(-26px) scale(1.04); box-shadow: 0 0 0 4px var(--accent), var(--shadow); }
.bk.faded { opacity: .3; transform: scale(.95); }
.cvw { margin-bottom: 6px; }
.bk h3 { font-size: 22px; line-height: 1.3; }
.au { font-size: 16px; color: var(--muted); }
.tags { display: flex; gap: 6px; flex-wrap: wrap; justify-content: center; margin: 4px 0; }
.tags .chip { font-size: 15px; min-height: 30px; }
.hook { font-size: 17px; color: var(--ink-2); line-height: 1.45; flex: 1; }
.acts { display: flex; flex-direction: column; gap: 8px; width: 100%; margin-top: 8px; }
.acts .btn { width: 100%; }
.intro-pop { position: absolute; inset: 0 0 112px 0; background: rgba(255, 255, 255, .98); padding: 16px 14px; text-align: left; font-size: 17px; line-height: 1.6; overflow: auto; animation: rise .24s var(--ease); }
.intro-pop small { display: block; color: var(--muted); margin-top: 6px; }
@keyframes rise { from { transform: translateY(30px); opacity: 0; } }
@media (max-width: 1100px) { .row { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 640px) { .row { grid-template-columns: repeat(2, minmax(0, 1fr)); } .q { display: none; } }

.res { max-width: 1180px; margin: 0 auto; padding: 18px 22px 120px; }
.tabs { display: flex; gap: 10px; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.tabs button { min-height: 48px; padding: 0 20px; border-radius: 12px; background: #fff; font-weight: 700; font-size: 18px; box-shadow: inset 0 0 0 2px var(--line); }
.tabs button.on { background: var(--accent); color: #fff; box-shadow: none; }
.cmp { color: var(--accent); font-weight: 700; font-size: 18px; }
.persona { display: flex; gap: 20px; align-items: center; padding: 12px 24px; flex-wrap: wrap; }
.persona h2 { font-size: 34px; color: var(--accent); }
.likes { display: flex; gap: 8px; align-items: center; margin-top: 4px; flex-wrap: wrap; }
.ver { margin-left: auto; display: flex; gap: 8px; }
.ver button { min-height: 44px; padding: 0 14px; border-radius: 10px; background: #f2eee6; font-weight: 700; font-size: 16px; }
.ver button.on { background: var(--accent-soft); color: var(--accent); }
.note { display: flex; gap: 8px; align-items: center; margin: 12px 4px 0; color: var(--accent); font-weight: 600; }
.note :deep(svg) { width: 22px; height: 22px; flex: none; }
.picks { display: flex; flex-direction: column; gap: 12px; margin-top: 14px; }
.pk { display: grid; grid-template-columns: 40px auto 1fr 250px; gap: 16px; align-items: center; padding: 10px 18px; }
.rank { font-size: 34px; color: var(--accent); text-align: center; }
.pt h3 { font-size: 24px; }
.pt h3 small { font-size: 16px; color: var(--muted); font-weight: 500; margin-left: 8px; }
.why { font-size: 18px; color: var(--ink-2); margin-top: 4px; }
.pi { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; }
.pi .muted { font-size: 15px; }
.pi b { font-size: 36px; color: var(--accent); line-height: 1.1; }
.pi .bar { width: 100%; height: 12px; border-radius: 6px; background: var(--accent-soft); overflow: hidden; }
.pi .bar i { display: block; height: 100%; background: var(--accent); transform-origin: left; }
.pi .lk { display: flex; gap: 8px; justify-content: flex-end; margin-top: 6px; }
.lk button, .ag .lk button { display: inline-flex; align-items: center; gap: 6px; min-height: 48px; padding: 0 16px; border-radius: 12px; background: #f4f1ea; font-weight: 700; font-size: 17px; }
.lk button :deep(svg) { width: 20px; height: 20px; }
.lk button.on { background: #e3f5ea; color: #148a4f; box-shadow: inset 0 0 0 2px #148a4f; }
.lk button.no.on { background: #fdeae6; color: #c0432f; box-shadow: inset 0 0 0 2px #c0432f; }
.why100 { margin: 14px 4px; font-size: 16px; }
.bar-b { position: fixed; left: 0; right: 0; bottom: 0; display: flex; align-items: center; gap: 14px; padding: 12px 22px calc(12px + env(safe-area-inset-bottom)); background: rgba(255, 253, 249, .97); border-top: 1px solid var(--line); flex-wrap: wrap; z-index: 10; }
.bar-b .sp { flex: 1; }
.donetip { color: var(--accent); font-weight: 700; font-size: 17px; }
.stars { display: flex; align-items: center; gap: 2px; font-size: 18px; }
.stars span:first-child { margin-right: 6px; }
.stars button { width: 48px; height: 48px; color: #d3c8b4; }
.stars button.on { color: var(--amber); }
.stars button :deep(svg) { width: 34px; height: 34px; }
@media (max-width: 900px) {
  .pk { grid-template-columns: 34px auto 1fr; }
  .pi { grid-column: 2 / 4; flex-direction: row; align-items: center; gap: 10px; }
  .pi b { font-size: 28px; }
}
.mask { position: fixed; inset: 0; background: rgba(20, 28, 32, .5); display: flex; align-items: center; justify-content: center; z-index: 60; padding: 16px; }
.again { width: 760px; max-width: 100%; padding: 26px; }
.again h2 { font-size: 28px; margin-bottom: 16px; }
.ag-books { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; }
.ag { display: flex; flex-direction: column; align-items: center; gap: 8px; text-align: center; font-size: 18px; }
.ag .lk { display: flex; gap: 6px; }
.ag .lk button, .ag .lk button { padding: 0 10px; font-size: 16px; }
.say { display: flex; flex-direction: column; gap: 8px; margin-top: 18px; font-size: 17px; color: var(--ink-2); }
.say input { min-height: 58px; border-radius: 14px; border: 2px solid var(--line); padding: 0 16px; font-size: 21px; outline: none; }
.say input:focus { border-color: var(--accent); }
.again .row { display: flex; justify-content: flex-end; gap: 12px; margin-top: 20px; }
</style>
