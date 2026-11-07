/* 座位：平板刷新或换环节都不用重新选。 */
import { reactive } from 'vue'
import { get, token } from './api'
export const ME = reactive({ seat: 0, group: 0, ready: false })
export async function loadMe() {
  if (!token()) { ME.ready = true; return ME }
  try { const m = await get('/api/common/me'); ME.seat = m.seat; ME.group = m.group } catch { /* offline */ }
  ME.ready = true
  return ME
}
