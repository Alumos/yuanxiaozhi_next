/* 低性能检测：首次打开时测 1 秒帧率，低于 40 就加 .lowfx（也可以由教师控制台强制打开）。 */
import { reactive } from 'vue'
export const FX = reactive({ low: false, scale: 1 })
export function setLow(v: boolean) { FX.low = v; document.documentElement.classList.toggle('lowfx', v) }
export function probeFps() {
  try { if (localStorage.getItem('yxz_lowfx') === '1') { setLow(true); return } } catch { /* */ }
  let n = 0
  const t0 = performance.now()
  const tick = () => {
    n++
    if (performance.now() - t0 < 1000) requestAnimationFrame(tick)
    else if (n < 40 && document.visibilityState === 'visible') { setLow(true); try { localStorage.setItem('yxz_lowfx', '1') } catch { /* */ } }
  }
  requestAnimationFrame(tick)
}
/** 动画时长：低性能模式下缩短 */
export const dur = (ms: number) => (FX.low ? Math.min(ms, 120) : ms) * FX.scale
