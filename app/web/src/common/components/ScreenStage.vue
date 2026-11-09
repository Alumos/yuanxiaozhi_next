<script setup lang="ts">
/* 大屏舞台：内部固定 1920×1080，外面等比缩放。底部隐藏教师操作条（鼠标移到底部或按 Space）。 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { get, post } from '../api'
const props = defineProps<{ mode?: string; lock?: string }>()
// 环节锁：本环节大屏右上角的开关，开放后学生平板才能操作
const locked = ref<boolean | null>(null)
let lockT = 0
async function loadLock() { if (!props.lock) return; try { const d = await get('/api/common/locks'); locked.value = !!d[props.lock] } catch { /* */ } }
async function toggleLock() {
  if (!props.lock || locked.value === null) return
  try { const d = await post('/api/common/lock', { mod: props.lock, locked: !locked.value }); locked.value = !!d[props.lock] } catch { /* */ }
}
const emit = defineEmits<{ (e: 'key', k: string): void }>()
const scale = ref(1), ox = ref(0), oy = ref(0)
const bar = ref(false)
let hideT = 0, digits = '', digitT = 0
function fit() {
  const k = Math.min(innerWidth / 1920, innerHeight / 1080)
  scale.value = k; ox.value = (innerWidth - 1920 * k) / 2; oy.value = (innerHeight - 1080 * k) / 2
}
function show() { bar.value = true; clearTimeout(hideT); hideT = window.setTimeout(() => (bar.value = false), 5000) }
function onMove(e: MouseEvent) { if (e.clientY > innerHeight - 90) show() }
function onKey(e: KeyboardEvent) {
  const t = e.target as HTMLElement
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA')) return
  const k = e.key
  if (k === ' ') { e.preventDefault(); bar.value ? (bar.value = false) : show(); return }
  if (k === 'f' || k === 'F') { document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen().catch(() => {}); return }
  if (/^[0-9]$/.test(k)) {
    digits += k; clearTimeout(digitT)
    digitT = window.setTimeout(() => { emit('key', 'seat:' + Number(digits)); digits = '' }, 550)
    return
  }
  const map: Record<string, string> = { ArrowRight: 'next', ArrowLeft: 'prev', p: 'pause', P: 'pause', r: 'replay', R: 'replay', Escape: 'home' }
  if (map[k]) { e.preventDefault(); emit('key', map[k]) }
}
onMounted(() => { loadLock(); lockT = window.setInterval(loadLock, 8000); document.body.classList.add('has-stage'); fit(); addEventListener('resize', fit); addEventListener('mousemove', onMove); addEventListener('keydown', onKey) })
onBeforeUnmount(() => { clearInterval(lockT); removeEventListener('resize', fit); removeEventListener('mousemove', onMove); removeEventListener('keydown', onKey) })
</script>
<template>
  <div class="vp" @scroll="($event.target as HTMLElement).scrollTo(0, 0)">
    <div class="stage" :style="{ transform: `translate(${ox}px,${oy}px) scale(${scale})` }">
      <slot />
      <div class="ctl" :class="{ on: bar }" @mouseenter="show" @mousemove="show"><slot name="controls" /></div>
      <button v-if="props.lock && locked !== null" class="lock" :class="{ open: !locked }" :title="locked ? '点击开放：学生平板可以开始操作' : '点击锁住：学生平板暂停操作'" @click="toggleLock">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2.5" /><path :d="locked ? 'M8 11V7a4 4 0 0 1 8 0v4' : 'M8 11V7a4 4 0 0 1 7.6-1.8'" /></svg>
        <span>{{ locked ? '平板已锁 · 点击开放' : '平板已开放' }}</span>
      </button>
      <i class="dot" :class="props.mode" :title="props.mode" />
      <a class="sicp" href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">苏ICP备2021038338号-1</a>
    </div>
  </div>
</template>
<style scoped>
.vp { position: fixed; inset: 0; background: #0d1020; overflow: hidden; overflow: clip; }
.stage { position: absolute; left: 0; top: 0; width: 1920px; height: 1080px; transform-origin: 0 0; background: var(--bg); overflow: hidden; font-size: 28px; }
.ctl { position: absolute; left: 0; right: 0; bottom: 0; min-height: 96px; padding: 14px 30px; display: flex; gap: 14px; align-items: center; flex-wrap: wrap; background: rgba(20, 26, 46, .94); transform: translateY(100%); transition: transform var(--t-mid) var(--ease); z-index: 60; color: #fff; }
.ctl.on { transform: none; }
.ctl :deep(button) { min-height: 60px; padding: 0 22px; border-radius: 12px; background: rgba(255, 255, 255, .12); color: #fff; font-size: 24px; font-weight: 700; }
.ctl :deep(button.on) { background: var(--accent); }
.ctl :deep(button.warn) { background: #a8362a; }
.ctl :deep(.sep) { width: 2px; height: 40px; background: rgba(255, 255, 255, .2); }
.sicp { position: absolute; left: 18px; bottom: 10px; z-index: 55; font-size: 16px; color: rgba(140, 134, 124, .55); text-decoration: none; }
.lock { position: absolute; right: 40px; bottom: 26px; z-index: 58; display: inline-flex; align-items: center; gap: 10px; height: 56px; padding: 0 22px 0 16px; border-radius: 28px; font-size: 24px; font-weight: 800; color: #fff; background: #c2412f; box-shadow: 0 6px 18px rgba(0, 0, 0, .18); cursor: pointer; transition: background-color var(--t-mid), opacity var(--t-mid); }
.lock svg { width: 30px; height: 30px; }
.lock.open { background: rgba(20, 154, 88, .16); color: #149a58; box-shadow: none; opacity: .75; }
.lock.open:hover { opacity: 1; }
.dot { position: absolute; right: 14px; bottom: 12px; width: 14px; height: 14px; border-radius: 50%; background: #9aa; z-index: 70; opacity: .8; }
.dot.live { background: #2bc06b; } .dot.replay { background: #f0b400; } .dot.mock { background: #9aa4a6; }
</style>
