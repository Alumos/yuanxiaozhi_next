<script setup lang="ts">
/* 顶栏：头像 · 环节名 · 四环节入口（点击切换）· 座位号 */
import YxzAvatar from './YxzAvatar.vue'
type Link = 'input' | 'compute' | 'output' | 'imagine'
const props = defineProps<{ title: string; link: Link; seat?: number | string; state?: 'idle' | 'listen' | 'think' | 'speak'; big?: boolean }>()
const steps: { k: Link; t: string; p: string }[] = [
  { k: 'input', t: '输入', p: '/shuru/' },
  { k: 'compute', t: '计算', p: '/jisuan/' },
  { k: 'output', t: '输出', p: '/shuchu/' },
  { k: 'imagine', t: '畅想', p: '/changxiang/' },
]
const href = (p: string) => (props.big ? p + 'screen' : p)
</script>
<template>
  <header class="hd" :class="{ big }">
    <div class="l">
      <YxzAvatar :state="state" :size="big ? 68 : 44" />
      <b>{{ title }}</b>
    </div>
    <nav class="steps" aria-label="环节入口">
      <template v-for="(s, i) in steps" :key="s.k">
        <span v-if="i" class="bar" :class="{ dash: s.k === 'imagine' }" />
        <a class="st" :class="[s.k, { on: s.k === link }]" :href="s.k === link ? undefined : href(s.p)" :aria-current="s.k === link ? 'page' : undefined"><i />{{ s.t }}</a>
      </template>
    </nav>
    <div class="r"><slot>{{ seat ? seat + ' 号平板' : '' }}</slot></div>
  </header>
</template>
<style scoped>
.hd { height: 64px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 20px; background: var(--paper); border-bottom: 1px solid var(--line); position: relative; z-index: 5; }
.l, .r { display: flex; align-items: center; gap: 12px; min-width: 0; flex: 1; }
.l b { font-size: 22px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.r { justify-content: flex-end; font-size: 18px; color: var(--ink-2); font-weight: 600; white-space: nowrap; }
.steps { display: flex; align-items: center; gap: 6px; flex: none; }
.st { display: flex; align-items: center; gap: 6px; min-height: 48px; padding: 0 12px; border-radius: 12px; color: #9aa4a6; font-size: 18px; font-weight: 700; text-decoration: none; transition: transform var(--t-mid) var(--ease), background-color var(--t-light); }
a.st[href]:active { transform: scale(.94); background: #efeae1; }
.st i { width: 12px; height: 12px; border-radius: 50%; background: currentColor; }
.st.on { transform: scale(1.12); background: var(--accent-soft); }
.st.on.input { color: var(--input); } .st.on.compute { color: var(--compute); } .st.on.output { color: var(--output); } .st.on.imagine { color: var(--imagine); }
.bar { width: 18px; height: 2px; background: #d9d4cb; }
.bar.dash { background: repeating-linear-gradient(90deg, #d9d4cb 0 4px, transparent 4px 7px); }
.big { height: 104px; padding: 0 44px; }
.big .l b { font-size: 40px; } .big .r { font-size: 30px; }
.big .st { font-size: 30px; gap: 10px; min-height: 64px; padding: 0 16px; } .big .st i { width: 20px; height: 20px; } .big .bar { width: 40px; height: 3px; }
@media (max-width: 900px) { .l b { display: none; } .st { padding: 0 8px; font-size: 17px; } .bar { width: 8px; } }
@media (max-width: 560px) { .r { display: none; } }
</style>
