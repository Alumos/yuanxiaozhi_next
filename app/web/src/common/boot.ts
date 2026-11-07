/* 每个页面的入口：样式、低性能检测、全局配置推送。 */
import { createApp, type Component } from 'vue'
import './tokens.css'
import { probeFps, setLow, FX } from './fx'
import { get } from './api'
export function boot(App: Component, opts: { screen?: boolean } = {}) {
  if (!opts.screen) probeFps()
  get('/api/common/me').then((m) => { if (m.lowfx) setLow(true); FX.scale = m.anim_scale || 1 }).catch(() => {})
  createApp(App).mount('#app')
}
