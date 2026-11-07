<script setup lang="ts">
import { ICON } from '../icons'
defineProps<{ text?: string; sub?: string }>()
</script>
<template>
  <div class="las"><div class="box"><span class="ic" v-html="ICON.screen" /><h2>{{ text || '请看大屏' }}</h2><p v-if="sub">{{ sub }}</p></div></div>
</template>
<style scoped>
.las { position: fixed; inset: 0; z-index: 80; background: rgba(247, 243, 236, .97); display: flex; align-items: center; justify-content: center; }
.box { text-align: center; color: var(--accent); }
.ic { display: inline-block; width: 120px; height: 120px; animation: bob 2s ease-in-out infinite; }
.ic :deep(svg) { width: 100%; height: 100%; }
h2 { font-size: 46px; margin-top: 12px; }
p { color: var(--ink-2); font-size: 22px; margin-top: 8px; }
@keyframes bob { 50% { transform: translateY(-10px); } }
</style>
