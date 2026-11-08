<script setup lang="ts">
/* 教师控制台：运行模式、模型状态与缓存命中率、内容校验、预热、重置。 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { get, post } from '../common/api'
import YxzHeader from '../common/components/YxzHeader.vue'
const d = ref<any>(null)
let t = 0
async function load() { d.value = await get('/api/common/admin') }
onMounted(() => { load(); t = window.setInterval(load, 3000) })
onBeforeUnmount(() => clearInterval(t))
async function act(body: any, ask?: string) { if (ask && !confirm(ask)) return; await post('/api/common/admin', body); load() }
const pct = (v: number) => (v * 100).toFixed(0) + '%'
const cleared = ref('')
async function newClass() {
  if (!confirm('确定清空本节课的全部数据？\n学生记录、路线图、畅想、座位都会删除，不能恢复。')) return
  if (!confirm('再确认一次：现在要换下一个班了吗？')) return
  await post('/api/common/admin', { action: 'new_class' })
  cleared.value = new Date().toLocaleTimeString()
  load()
}
</script>
<template>
  <YxzHeader title="教师控制台" link="compute" />
  <main v-if="d" class="ad">
    <section class="card">
      <h2>运行模式</h2>
      <div class="seg">
        <button v-for="m in [['live', '实时'], ['replay', '回放'], ['mock', '模拟']]" :key="m[0]" :class="{ on: d.mode === m[0] }" @click="act({ action: 'mode', mode: m[0] })">{{ m[1] }}</button>
      </div>
      <p class="muted">实时：调用真实模型并自动录制；回放：断网时用彩排录下的回答；模拟：不联网。本地计算永远真实运行。</p>
      <label class="sw"><input type="checkbox" :checked="d.config.use_llm" @change="act({ action: 'config', config: { use_llm: ($event.target as HTMLInputElement).checked } })" /> 使用大模型（关掉后全部走本地算法）</label>
      <label class="sw">同时请求模型的上限
        <input class="numin" type="number" min="1" max="40" :value="d.config.max_concurrency" @change="act({ action: 'config', config: { max_concurrency: Number(($event.target as HTMLInputElement).value) } })" />
        <small class="muted">网关稳定时可调到 25；网关报错多时调小（默认 12），多出来的请求在本机排队</small></label>
      <label class="sw"><input type="checkbox" :checked="d.config.lowfx" @change="act({ action: 'config', config: { lowfx: ($event.target as HTMLInputElement).checked } })" /> 低性能模式（旧平板）</label>
    </section>
    <section class="card">
      <h2>状态 <span class="ok" :class="{ bad: !d.llm.gateway.configured }">模型网关</span><span class="ok" :class="{ bad: !d.llm.content_ok }">内容校验</span><span class="ok" :class="{ bad: !String(d.llm.jisuan.warm).startsWith('已预热') }">缓存预热</span></h2>
      <p>模型：<b>{{ d.llm.gateway.model }}</b> · 并发 {{ d.llm.gateway.running }}/{{ d.llm.gateway.limit }}（上限 {{ d.llm.gateway.max }}）· 排队 {{ d.llm.gateway.queued }} · 成功 {{ d.llm.gateway.ok }} · 失败 {{ d.llm.gateway.fail }}</p>
      <p class="muted">{{ d.llm.jisuan.warm }} · 前缀哈希 {{ d.llm.jisuan.prefix_hash }}</p>
      <p v-if="d.llm.gateway.last_error" class="err">最近错误：{{ d.llm.gateway.last_error }}</p>
      <table>
        <tr><th>场景</th><th>调用</th><th>平均用时</th><th>缓存命中率</th><th>兜底</th><th>出错</th></tr>
        <tr v-for="(s, k) in d.llm.gateway.scenes" :key="k"><td>{{ k }}</td><td>{{ s.calls }}</td><td>{{ (s.avg_ms / 1000).toFixed(1) }} 秒</td><td>{{ pct(s.cache_hit) }}</td><td>{{ s.fallbacks }}</td><td>{{ s.errors }}</td></tr>
      </table>
      <p class="muted">回放录音：{{ JSON.stringify(d.llm.replay) }}</p>
    </section>
    <section class="card nc">
      <h2>换下一个班</h2>
      <p>一键清空<b>这节课产生的所有临时数据</b>：四个环节的学生记录、选书和推荐、各组的路线图和对话、畅想、投票、平板座位。所有平板会自动回到选座位。</p>
      <p class="muted">保留：设置、运行模式、彩排录下的回放（断网备用）。</p>
      <button class="btn big danger" @click="newClass">清空本节课数据，准备下一个班</button>
      <p v-if="cleared" class="ok2">已清空（{{ cleared }}），可以开始下一个班了</p>
    </section>
    <section class="card">
      <h2>课堂</h2>
      <p>已入座平板 <b>{{ d.seats }}</b> 台 · 在线：计算 {{ d.hubs.jisuan?.students }} / 输出 {{ d.hubs.shuchu?.students }} · 大屏 {{ (d.hubs.jisuan?.screens || 0) + (d.hubs.shuchu?.screens || 0) }}</p>
      <p>本机局域网地址：<b v-for="ip in d.ips" :key="ip" class="ip">{{ ip }}</b></p>
      <div class="seg">
        <a class="btn soft" href="/jisuan/screen" target="_blank">计算大屏</a>
        <a class="btn soft" href="/shuchu/screen" target="_blank">输出大屏</a>
        <a class="btn soft" href="/changxiang/screen" target="_blank">畅想大屏</a>
        <a class="btn soft" href="/" target="_blank">入口二维码</a>
      </div>
      <div class="seg">
        <button class="btn ghost" @click="act({ action: 'clear_seats' }, '清空所有座位？平板需要重新选座位。')">清空座位</button>
        <button class="btn ghost" @click="act({ action: 'reset', module: 'jisuan' }, '重置计算环节？')">重置计算环节</button>
        <button class="btn ghost" @click="act({ action: 'reset', module: 'shuchu' }, '重置输出环节？')">重置输出环节</button>
        <button class="btn ghost" @click="act({ action: 'reset', module: 'changxiang' }, '清空畅想？')">清空畅想</button>
      </div>
    </section>
  </main>
</template>
<style scoped>
.ad { max-width: 1100px; margin: 0 auto; padding: 20px; display: flex; flex-direction: column; gap: 18px; }
section { padding: 22px 26px; display: flex; flex-direction: column; gap: 10px; }
h2 { font-size: 26px; display: flex; gap: 12px; align-items: center; }
.seg { display: flex; gap: 10px; flex-wrap: wrap; }
.seg > button:not(.btn) { min-height: 52px; padding: 0 26px; border-radius: 12px; background: #f2eee6; font-weight: 700; font-size: 20px; }
.seg > button.on { background: var(--accent); color: #fff; }
.sw { display: flex; gap: 10px; align-items: center; font-size: 19px; }
.sw input { width: 26px; height: 26px; }
.sw .numin { width: 90px; height: 46px; font-size: 22px; text-align: center; border-radius: 10px; border: 2px solid var(--line); }
.ok { font-size: 16px; padding: 4px 12px; border-radius: 999px; background: #e3f5ea; color: #148a4f; }
.ok.bad { background: #fdeae6; color: #c0432f; }
.err { color: var(--red); }
.nc { border: 3px solid #f3c9c0; }
.nc b { color: var(--red); }
.btn.danger { background: var(--red); align-self: flex-start; }
.ok2 { color: var(--output); font-weight: 800; font-size: 20px; }
table { border-collapse: collapse; font-size: 17px; }
td, th { border-bottom: 1px solid var(--line); padding: 8px 12px; text-align: left; }
.ip { margin-right: 14px; font-size: 22px; }
</style>
