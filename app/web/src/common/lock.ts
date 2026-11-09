/* 环节锁：教师在大屏上解锁后，学生平板才能操作这个环节。 */
import { reactive } from 'vue'
import { get } from './api'
export const LOCK = reactive({ on: false, mod: '' })
export async function loadLock(mod: string) {
  LOCK.mod = mod
  try { const d = await get('/api/common/locks'); LOCK.on = !!d[mod] } catch { /* 网络不好时不挡学生 */ }
}
/** 放进 events() 的处理函数里：lock: onLock */
export function onLock(d: { mod: string; locked: boolean }) { if (d.mod === LOCK.mod) LOCK.on = d.locked }
