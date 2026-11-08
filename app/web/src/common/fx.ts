/* 低性能检测：页面加载完 1.5 秒后测 2 秒帧率，低于 24 才加 .lowfx（也可以由教师控制台强制打开）。
 * 低性能模式只关掉耗电的装饰动画，不改变讲解节奏（旧平板上不能“快进”）。 */
import { reactive } from 'vue'
export const FX = reactive({ low: false, scale: 1 })
export function setLow(v: boolean) { FX.low = v; document.documentElement.classList.toggle('lowfx', v) }
export function probeFps() {
  try { localStorage.removeItem('yxz_lowfx') } catch { /* 旧版本留下的标记 */ }
  const go = () => setTimeout(() => {
    let n = 0
    const t0 = performance.now()
    const tick = () => {
      n++
      const t = performance.now() - t0
      if (t < 2000) requestAnimationFrame(tick)
      else if (n / (t / 1000) < 24 && document.visibilityState === 'visible') setLow(true)
    }
    requestAnimationFrame(tick)
  }, 1500)
  if (document.readyState === 'complete') go(); else window.addEventListener('load', go, { once: true })
}
/** 动画节奏（等待时长）：只受教师设置的整体倍速影响 */
export const dur = (ms: number) => ms * FX.scale
