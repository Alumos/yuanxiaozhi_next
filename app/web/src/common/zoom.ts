import { reactive } from 'vue'
export const Z = reactive({ src: '', fx: 0.5, fy: 0.5 })
export function openZoom(src: string, e?: MouseEvent) {
  Z.fx = 0.5; Z.fy = 0.5
  if (e && e.currentTarget instanceof Element) {
    const r = e.currentTarget.getBoundingClientRect()
    if (r.width && r.height) { Z.fx = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)); Z.fy = Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)) }
  }
  Z.src = src
}
export function closeZoom() { Z.src = '' }
