<script setup lang="ts">
/* 三环节入口：三个大按钮和各自的二维码（用服务器局域网地址生成）。 */
import { onMounted, ref } from 'vue'
import qrcode from 'qrcode-generator'
import { get } from '../common/api'
const base = ref(location.origin)
const hop = ref(false)
let hopT = 0
function poke() { hop.value = false; requestAnimationFrame(() => (hop.value = true)); clearTimeout(hopT); hopT = window.setTimeout(() => (hop.value = false), 1600) }
const items = [
  { k: 'input', t: '输入', d: '秋假攻略', p: '/shuru/' },
  { k: 'compute', t: '计算', d: '智能图书管理员', p: '/jisuan/' },
  { k: 'output', t: '输出', d: '疏散路线图', p: '/shuchu/' },
  { k: 'imagine', t: '畅想', d: '元小智的新本领', p: '/changxiang/' },
]
function qr(url: string) { const q = qrcode(0, 'M'); q.addData(url); q.make(); return q.createSvgTag({ cellSize: 5, margin: 2, scalable: true }) }
onMounted(async () => {
  if (location.hostname === 'localhost' || location.hostname === '127.0.0.1') {
    try { const d = await get('/api/common/lan'); if (d.ip) base.value = `${location.protocol}//${d.ip}:${location.port || 80}` } catch { /* */ }
  }
})
</script>
<template>
  <main class="home">
    <header>
      <button class="hero" :class="{ hop }" aria-label="元小智" @click="poke">
        <i class="glow" />
        <svg viewBox="0 0 120 120" aria-hidden="true">
          <ellipse class="shadow" cx="60" cy="112" rx="26" ry="4.5" fill="#1f2d33" opacity=".12" />
          <g class="bob">
            <g class="ant"><path d="M60 26V13" stroke="var(--accent)" stroke-width="4" stroke-linecap="round" /><circle class="bulb" cx="60" cy="11" r="6" fill="var(--amber)" /></g>
            <g class="wave"><path d="M96 66 q10 -6 12 -18" stroke="var(--accent)" stroke-width="4.5" fill="none" stroke-linecap="round" /><circle cx="108" cy="46" r="6" fill="#fff" stroke="var(--accent)" stroke-width="3.5" /></g>
            <path d="M24 70 q-10 6 -10 16" stroke="var(--accent)" stroke-width="4.5" fill="none" stroke-linecap="round" />
            <circle cx="14" cy="88" r="5.5" fill="#fff" stroke="var(--accent)" stroke-width="3.5" />
            <rect x="22" y="26" width="76" height="64" rx="26" fill="#fff" stroke="var(--accent)" stroke-width="4.5" />
            <rect x="33" y="40" width="54" height="30" rx="15" fill="#1f2d33" />
            <g class="eyes"><g class="blink"><circle cx="48" cy="55" r="6" fill="#7ff0ff" /><circle cx="72" cy="55" r="6" fill="#7ff0ff" /><circle cx="50" cy="53" r="1.8" fill="#fff" /><circle cx="74" cy="53" r="1.8" fill="#fff" /></g></g>
            <ellipse cx="35" cy="76" rx="5" ry="3" fill="#ff9aa8" opacity=".55" /><ellipse cx="85" cy="76" rx="5" ry="3" fill="#ff9aa8" opacity=".55" />
            <path class="mouth" d="M52 78 q8 7 16 0" stroke="var(--accent)" stroke-width="3.5" fill="none" stroke-linecap="round" />
          </g>
        </svg>
        <span v-if="hop" class="hi">你好呀！</span>
      </button>
      <div><h1>元小智课堂</h1><p class="muted">人工智能系统的三个环节：输入 → 计算 → 输出，再畅想元小智的新本领</p></div></header>
    <div class="row">
      <a v-for="it in items" :key="it.k" :href="it.p" class="it card" :class="it.k">
        <b>{{ it.t }}</b><span>{{ it.d }}</span>
        <i class="qr" v-html="qr(base + it.p)" />
        <small class="num">{{ base }}{{ it.p }}</small>
      </a>
    </div>
  </main>
</template>
<style scoped>
.home { max-width: 1200px; margin: 0 auto; padding: 40px 24px; }
header { display: flex; gap: 26px; align-items: center; margin-bottom: 30px; }
.hero { position: relative; flex: none; width: 150px; height: 150px; padding: 0; border: 0; background: none; cursor: pointer; -webkit-tap-highlight-color: transparent; }
.hero svg { position: relative; width: 100%; height: 100%; display: block; overflow: visible; }
.hero .glow { position: absolute; inset: 14px; border-radius: 50%; background: radial-gradient(circle, rgba(138, 85, 214, .28), rgba(138, 85, 214, 0) 70%); animation: glow 3s ease-in-out infinite; }
.hero .bob { animation: bob 3s ease-in-out infinite; }
.hero .shadow { transform-box: fill-box; transform-origin: center; animation: shadow 3s ease-in-out infinite; }
.hero .ant { transform-box: fill-box; transform-origin: 50% 100%; animation: ant 2.4s ease-in-out infinite; }
.hero .bulb { animation: bulb 1.6s ease-in-out infinite; }
.hero .wave { transform-box: fill-box; transform-origin: 0% 100%; animation: wave 2.8s ease-in-out infinite; }
.hero .eyes { animation: look 6s ease-in-out infinite; }
.hero .blink { transform-box: fill-box; transform-origin: center; animation: blink 4.2s infinite; }
.hero .mouth { transform-box: fill-box; transform-origin: center; }
.hero.hop .bob { animation: hop .6s cubic-bezier(.3, 1.6, .5, 1) 2; }
.hero.hop .mouth { animation: talk .18s ease-in-out 8 alternate; }
.hi { position: absolute; z-index: 3; left: 112px; top: -30px; white-space: nowrap; background: #fff; color: var(--accent); font-size: 20px; font-weight: 800; padding: 6px 14px; border-radius: 16px 16px 16px 4px; box-shadow: var(--shadow); animation: pop .3s var(--ease) both; }
@keyframes bob { 50% { transform: translateY(-7px); } }
@keyframes shadow { 50% { transform: scaleX(.8); opacity: .07; } }
@keyframes glow { 50% { transform: scale(1.12); opacity: .6; } }
@keyframes ant { 25% { transform: rotate(-10deg); } 75% { transform: rotate(10deg); } }
@keyframes bulb { 50% { opacity: .55; } }
@keyframes wave { 0%, 40%, 100% { transform: rotate(0); } 10%, 30% { transform: rotate(-22deg); } 20% { transform: rotate(8deg); } }
@keyframes look { 0%, 20%, 100% { transform: translateX(0); } 30%, 45% { transform: translateX(-4px); } 55%, 70% { transform: translateX(4px); } }
@keyframes blink { 0%, 94%, 100% { transform: scaleY(1); } 97% { transform: scaleY(.1); } }
@keyframes hop { 40% { transform: translateY(-18px) rotate(-4deg); } }
@keyframes talk { to { transform: scaleY(.4); } }
@keyframes pop { from { transform: scale(.6); opacity: 0; } }
@media (prefers-reduced-motion: reduce) { .hero * { animation: none !important; } }
h1 { font-size: 52px; }
.row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 20px; }
.it { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 30px 20px; text-decoration: none; color: var(--ink); border-top: 10px solid var(--c); }
.it.input { --c: var(--input); } .it.compute { --c: var(--compute); } .it.output { --c: var(--output); } .it.imagine { --c: var(--imagine); }
.it b { font-size: 48px; color: var(--c); } .it span { font-size: 24px; }
.qr { display: block; width: 200px; height: 200px; margin-top: 10px; }
.qr :deep(svg) { width: 100%; height: 100%; }
small { color: var(--muted); font-size: 15px; }
@media (max-width: 1000px) { .row { grid-template-columns: 1fr 1fr; } }
@media (max-width: 560px) { .row { grid-template-columns: 1fr; } }
</style>
