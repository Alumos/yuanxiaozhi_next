<script setup lang="ts">
defineProps<{ text: string; color?: string }>()
</script>
<template><span class="stamp" :style="{ color: color || 'var(--red)' }">{{ text }}</span></template>
<style scoped>
.stamp { display: inline-block; border: 4px solid currentColor; border-radius: 12px; padding: 4px 14px; font-weight: 900; font-size: 24px; letter-spacing: 2px; transform: rotate(-10deg); animation: stamp .5s var(--ease) both; background: rgba(255, 255, 255, .7); white-space: nowrap; }
@keyframes stamp { 0% { transform: rotate(-10deg) scale(2.4); opacity: 0; } 60% { transform: rotate(-10deg) scale(.92); opacity: 1; } 100% { transform: rotate(-10deg) scale(1); } }
</style>
