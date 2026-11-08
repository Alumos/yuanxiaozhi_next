<script setup lang="ts">
/* 跳动数字：从旧值滚到新值（只改文字，不触发布局动画）。 */
import { onBeforeUnmount, ref, watch } from 'vue'
import { FX } from '../fx'
const p = defineProps<{ value: number; ms?: number; decimals?: number; sep?: boolean }>()
const shown = ref(0)
let raf = 0
function fmt(v: number) {
  const s = v.toFixed(p.decimals || 0)
  return p.sep === false ? s : s.replace(/\B(?=(\d{3})+(?!\d))/g, ' ')
}
watch(() => p.value, (to, from) => {
  cancelAnimationFrame(raf)
  const a = from ?? 0, t0 = performance.now(), d = p.ms ?? 900
  const step = () => {
    const k = Math.min(1, (performance.now() - t0) / d)
    shown.value = a + (to - a) * (1 - Math.pow(1 - k, 3))
    if (k < 1) raf = requestAnimationFrame(step)
  }
  step()
}, { immediate: true })
onBeforeUnmount(() => cancelAnimationFrame(raf))
</script>
<template><span class="num">{{ fmt(shown) }}</span></template>
