import { reactive } from 'vue'
export const T = reactive({ list: [] as { id: number; text: string; kind: string }[] })
let seq = 0
export function toast(text: string, kind: 'info' | 'warn' | 'ok' = 'info', ms = 3000) {
  const id = ++seq
  T.list.push({ id, text, kind })
  if (T.list.length > 3) T.list.shift()
  setTimeout(() => { const i = T.list.findIndex((x) => x.id === id); if (i >= 0) T.list.splice(i, 1) }, ms)
}
