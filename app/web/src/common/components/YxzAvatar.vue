<script setup lang="ts">
/* 元小智头像：idle 待机 / listen 聆听 / think 思考（外圈转动）/ speak 说话 */
defineProps<{ state?: 'idle' | 'listen' | 'think' | 'speak'; size?: number }>()
</script>
<template>
  <span class="av" :class="state || 'idle'" :style="{ width: (size || 44) + 'px', height: (size || 44) + 'px' }">
    <svg viewBox="0 0 64 64" aria-hidden="true">
      <circle class="ring" cx="32" cy="32" r="30" fill="none" stroke="var(--accent)" stroke-width="3" stroke-dasharray="30 12" />
      <rect x="12" y="16" width="40" height="34" rx="14" fill="#fff" stroke="var(--accent)" stroke-width="3" />
      <path d="M32 16V9" stroke="var(--accent)" stroke-width="3" stroke-linecap="round" />
      <circle cx="32" cy="8" r="3.5" fill="var(--amber)" />
      <rect x="18" y="24" width="28" height="16" rx="8" fill="#1f2d33" />
      <circle class="eye" cx="26" cy="32" r="3" fill="#7ff0ff" />
      <circle class="eye" cx="38" cy="32" r="3" fill="#7ff0ff" />
      <rect class="mouth" x="27" y="43" width="10" height="3" rx="1.5" fill="var(--accent)" />
    </svg>
  </span>
</template>
<style scoped>
.av { display: inline-block; flex: none; }
svg { width: 100%; height: 100%; display: block; overflow: visible; }
.ring { opacity: 0; transform-origin: 32px 32px; }
.think .ring { opacity: 1; animation: spin 1.4s linear infinite; }
.listen .ring { opacity: .6; animation: pulse 1.2s ease-in-out infinite; }
.speak .mouth { animation: talk .35s ease-in-out infinite alternate; transform-origin: 32px 44px; }
.eye { animation: blink 4s infinite; transform-origin: center; transform-box: fill-box; }
@keyframes spin { to { transform: rotate(360deg); } }
@keyframes pulse { 50% { transform: scale(1.08); } }
@keyframes talk { to { transform: scaleY(2.4); } }
@keyframes blink { 0%, 96%, 100% { transform: scaleY(1); } 98% { transform: scaleY(.1); } }
</style>
