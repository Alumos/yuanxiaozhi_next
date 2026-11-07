<script setup lang="ts">
/* 顶栏：头像 · 环节名 · 三环节指示 · 座位号 */
import YxzAvatar from './YxzAvatar.vue'
defineProps<{ title: string; link: 'input' | 'compute' | 'output'; seat?: number | string; state?: 'idle' | 'listen' | 'think' | 'speak'; big?: boolean }>()
const steps = [{ k: 'input', t: '输入' }, { k: 'compute', t: '计算' }, { k: 'output', t: '输出' }]
</script>
<template>
  <header class="hd" :class="{ big }">
    <div class="l">
      <YxzAvatar :state="state" :size="big ? 68 : 44" />
      <b>{{ title }}</b>
    </div>
    <nav class="steps" aria-label="三环节">
      <template v-for="(s, i) in steps" :key="s.k">
        <span v-if="i" class="bar" />
        <span class="st" :class="[s.k, { on: s.k === link }]"><i />{{ s.t }}</span>
      </template>
    </nav>
    <div class="r"><slot>{{ seat ? seat + ' 号平板' : '' }}</slot></div>
  </header>
</template>
<style scoped>
.hd { height: 64px; display: flex; align-items: center; justify-content: space-between; padding: 0 20px; background: var(--paper); border-bottom: 1px solid var(--line); position: relative; z-index: 5; }
.l, .r { display: flex; align-items: center; gap: 12px; min-width: 0; flex: 1; }
.l b { font-size: 22px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.r { justify-content: flex-end; font-size: 18px; color: var(--ink-2); font-weight: 600; }
.steps { display: flex; align-items: center; gap: 10px; flex: none; }
.st { display: flex; align-items: center; gap: 6px; color: #b3bbbd; font-size: 17px; font-weight: 700; transition: transform var(--t-mid) var(--ease); }
.st i { width: 12px; height: 12px; border-radius: 50%; background: currentColor; }
.st.on { transform: scale(1.18); }
.st.on.input { color: var(--input); } .st.on.compute { color: var(--compute); } .st.on.output { color: var(--output); }
.bar { width: 28px; height: 2px; background: #d9d4cb; }
.big { height: 104px; padding: 0 44px; }
.big .l b { font-size: 40px; } .big .r { font-size: 30px; }
.big .st { font-size: 30px; gap: 10px; } .big .st i { width: 20px; height: 20px; } .big .bar { width: 60px; height: 3px; }
@media (max-width: 760px) { .steps { display: none; } }
</style>
