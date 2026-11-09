<script setup lang="ts">
/* 学生平板：环节没开放时盖住页面（顶栏除外），开放后自动消失。 */
import { LOCK } from '../lock'
</script>
<template>
    <div v-if="LOCK.on" class="lk" role="status">
      <div class="box">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2.5" /><path d="M8 11V7a4 4 0 0 1 8 0v4" /><circle cx="12" cy="16" r="1.4" fill="currentColor" /></svg>
        <h2>先听老师讲</h2>
        <p>老师开放这个环节后，这里会自动打开</p>
      </div>
    </div>
</template>
<style scoped>
.lk { position: fixed; left: 0; right: 0; top: 64px; bottom: 0; z-index: 45; display: flex; align-items: center; justify-content: center; background: rgba(246, 243, 236, .9); }
.box { display: flex; flex-direction: column; align-items: center; gap: 10px; text-align: center; color: var(--ink-2); padding: 0 24px; }
.box svg { width: 96px; height: 96px; color: var(--accent); }
.box h2 { font-size: 40px; color: var(--ink); }
.box p { font-size: 22px; }
</style>
