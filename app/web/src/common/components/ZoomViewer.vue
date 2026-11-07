<script setup lang="ts">
/* 看图放大（移植旧平台）：只改一张 <img> 的 transform，旧平板也流畅。点哪儿放大哪儿、双指缩放、拖动、双击还原。 */
import { nextTick, ref, watch } from 'vue'
import { Z, closeZoom } from '../zoom'
const img = ref<HTMLImageElement>()
let s = 1, tx = 0, ty = 0, raf = 0
const MAX = 5
function apply(smooth = false) {
  const el = img.value
  if (!el) return
  el.style.transition = smooth ? 'transform .22s ease-out' : 'none'
  cancelAnimationFrame(raf)
  raf = requestAnimationFrame(() => { el.style.transform = `translate3d(${tx}px,${ty}px,0) scale(${s})` })
}
function clamp() {
  const el = img.value
  if (!el) return
  s = Math.min(MAX, Math.max(1, s))
  const mx = ((s - 1) * el.offsetWidth) / 2 + 40, my = ((s - 1) * el.offsetHeight) / 2 + 40
  if (s === 1) { tx = 0; ty = 0 }
  tx = Math.min(mx, Math.max(-mx, tx)); ty = Math.min(my, Math.max(-my, ty))
}
function zoomAt(ns: number, px: number, py: number) {
  const el = img.value
  if (!el) return
  const r = el.parentElement!.getBoundingClientRect()
  const cx = r.left + r.width / 2, cy = r.top + r.height / 2
  const qx = (px - cx - tx) / s, qy = (py - cy - ty) / s
  ns = Math.min(MAX, Math.max(1, ns))
  tx = px - cx - ns * qx; ty = py - cy - ns * qy; s = ns
  clamp()
}
function onLoad() {
  const el = img.value!
  s = 2.2
  tx = -s * (Z.fx - 0.5) * el.offsetWidth
  ty = -s * (Z.fy - 0.5) * el.offsetHeight
  clamp(); apply(true)
}
watch(() => Z.src, async (v) => { if (v) { s = 1; tx = 0; ty = 0; await nextTick(); apply() } })
const pts = new Map<number, { x: number; y: number }>()
let moved = false, lastTap = 0, pinch0 = 0, s0 = 1
function down(e: PointerEvent) {
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  pts.set(e.pointerId, { x: e.clientX, y: e.clientY })
  moved = false
  if (pts.size === 2) { const [a, b] = [...pts.values()]; pinch0 = Math.hypot(a.x - b.x, a.y - b.y); s0 = s }
}
function move(e: PointerEvent) {
  const p = pts.get(e.pointerId)
  if (!p) return
  const dx = e.clientX - p.x, dy = e.clientY - p.y
  if (Math.abs(dx) + Math.abs(dy) > 4) moved = true
  if (pts.size === 1) { if (s > 1) { tx += dx; ty += dy; clamp(); apply() } }
  else if (pts.size === 2) {
    const [a, b] = [...pts.entries()].map(([id, q]) => (id === e.pointerId ? { x: e.clientX, y: e.clientY } : q))
    const d = Math.hypot(a.x - b.x, a.y - b.y)
    if (pinch0 > 0) { zoomAt((s0 * d) / pinch0, (a.x + b.x) / 2, (a.y + b.y) / 2); apply() }
  }
  pts.set(e.pointerId, { x: e.clientX, y: e.clientY })
}
function up(e: PointerEvent) {
  const wasPinch = pts.size > 1
  pts.delete(e.pointerId)
  if (pts.size === 1) pinch0 = 0
  if (wasPinch || moved) return
  const t = Date.now()
  if (t - lastTap < 300) { if (s > 1.05) { s = 1; tx = 0; ty = 0 } else zoomAt(2.5, e.clientX, e.clientY); apply(true); lastTap = 0 }
  else lastTap = t
}
function wheel(e: WheelEvent) { zoomAt(s * (e.deltaY < 0 ? 1.15 : 1 / 1.15), e.clientX, e.clientY); apply() }
function btn(k: number) {
  const el = img.value?.parentElement?.getBoundingClientRect()
  if (!el) return
  if (k === 0) { s = 1; tx = 0; ty = 0 } else zoomAt(s * k, el.left + el.width / 2, el.top + el.height / 2)
  apply(true)
}
function bgTap(e: MouseEvent) { if (e.target !== img.value && !moved && s <= 1.05) closeZoom() }
</script>
<template>
  <div v-if="Z.src" class="zv">
    <div class="stage" @click="bgTap" @pointerdown="down" @pointermove="move" @pointerup="up" @pointercancel="up" @wheel.prevent="wheel">
      <img ref="img" :src="Z.src" alt="放大查看" draggable="false" @load="onLoad" />
    </div>
    <div class="bar">
      <button @click="btn(1 / 1.5)" aria-label="缩小">－</button>
      <button @click="btn(0)">原图</button>
      <button @click="btn(1.5)" aria-label="放大">＋</button>
      <button class="x" @click="closeZoom">关闭</button>
    </div>
    <p class="tip">双指缩放 · 拖动查看 · 双击放大 / 还原</p>
  </div>
</template>
<style scoped>
.zv { position: fixed; inset: 0; z-index: 95; background: rgba(20, 28, 32, .94); display: flex; flex-direction: column; }
.stage { flex: 1; min-height: 0; display: flex; align-items: center; justify-content: center; overflow: hidden; touch-action: none; padding: 12px; }
.stage img { max-width: 100%; max-height: 100%; border-radius: 8px; background: #fffdf8; transform-origin: center center; will-change: transform; user-select: none; -webkit-user-drag: none; }
.bar { position: absolute; top: 14px; right: 16px; display: flex; gap: 10px; }
.bar button { min-width: 60px; height: 52px; padding: 0 14px; border-radius: 14px; background: rgba(20, 28, 32, .8); color: #fff; font-size: 22px; font-weight: 700; }
.bar .x { background: rgba(255, 255, 255, .92); color: #2a3a40; }
.tip { position: absolute; bottom: 12px; left: 0; right: 0; text-align: center; color: #fff; opacity: .65; font-size: 16px; pointer-events: none; }
</style>
