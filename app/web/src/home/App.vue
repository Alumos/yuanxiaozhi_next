<script setup lang="ts">
/* 三环节入口：三个大按钮和各自的二维码（用服务器局域网地址生成）。 */
import { onMounted, ref } from 'vue'
import qrcode from 'qrcode-generator'
import YxzAvatar from '../common/components/YxzAvatar.vue'
import { get } from '../common/api'
const base = ref(location.origin)
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
    <header><YxzAvatar state="speak" :size="84" /><div><h1>元小智课堂</h1><p class="muted">人工智能系统的三个环节：输入 → 计算 → 输出，再畅想元小智的新本领</p></div></header>
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
header { display: flex; gap: 20px; align-items: center; margin-bottom: 34px; }
h1 { font-size: 44px; }
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
