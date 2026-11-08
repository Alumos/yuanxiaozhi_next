/* 前端和服务器通信：JSON 接口、POST 流式响应（SSE 格式）、推送事件。 */
const TOKEN = 'yxz_seat'
export function token(): string { try { return localStorage.getItem(TOKEN) || '' } catch { return '' } }
export function setToken(t: string) { try { localStorage.setItem(TOKEN, t) } catch { /* private mode */ } }

export class ApiError extends Error {
  constructor(public status: number, msg: string, public code = '') { super(msg) }
}

function headers(): Record<string, string> {
  const h: Record<string, string> = { 'Content-Type': 'application/json' }
  const t = token()
  if (t) h['X-Seat-Token'] = t
  return h
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  let r: Response
  try {
    r = await fetch(path, { method, headers: headers(), body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, '网络有点慢，再试一次', 'network')
  }
  const txt = await r.text()
  let d: any = {}
  try { d = txt ? JSON.parse(txt) : {} } catch { /* not json */ }
  if (!r.ok) throw new ApiError(r.status, d.error || '出错了，再试一次', d.code || '')
  return d as T
}
export const get = <T = any>(p: string) => req<T>('GET', p)
export const post = <T = any>(p: string, b: unknown = {}) => req<T>('POST', p, b)

/** POST whose response body is an SSE stream; calls on(name, data) for each event. */
export async function stream(path: string, body: unknown, on: (ev: string, data: any) => void, signal?: AbortSignal): Promise<void> {
  const r = await fetch(path, { method: 'POST', headers: headers(), body: JSON.stringify(body ?? {}), credentials: 'same-origin', signal })
  if (!r.ok) {
    let d: any = {}
    try { d = await r.json() } catch { /* */ }
    throw new ApiError(r.status, d.error || '出错了，再试一次', d.code || '')
  }
  const reader = r.body!.getReader()
  const dec = new TextDecoder()
  let buf = '', ev = 'message', data = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    let i: number
    while ((i = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, i).replace(/\r$/, '')
      buf = buf.slice(i + 1)
      if (line === '') {
        if (data) { let v: any = data; try { v = JSON.parse(data) } catch { /* */ } on(ev, v) }
        ev = 'message'; data = ''
      } else if (line.startsWith('event:')) ev = line.slice(6).trim()
      else if (line.startsWith('data:')) data += line.slice(5).trim()
    }
  }
}

/** 可续传的流：连接中途断开（旧平板常见 “Error in input stream”）就重新发起同一个请求。
 *  服务器会让新请求接上同一个任务、从头重放，这里跳过已经收到的事件，界面不会重复。 */
export async function streamResume(path: string, body: unknown, on: (ev: string, data: any) => void, tries = 5): Promise<void> {
  let seen = 0
  for (let k = 0; ; k++) {
    let n = 0, finished = false
    try {
      await stream(path, body, (ev, d) => {
        if (ev === 'done') finished = true
        if (++n > seen) { seen = n; on(ev, d) }
      })
      if (finished || k >= tries) return
    } catch (e) {
      if (e instanceof ApiError) throw e
      if (k >= tries) throw new ApiError(0, '网络断了一下，点“再试一次”', 'network')
    }
    await sleep(800 + k * 700)
  }
}

/** Server push for a module (EventSource reconnects by itself and replays with Last-Event-ID). */
export function events(mod: string, role: 'student' | 'screen', handlers: Record<string, (d: any) => void>, onOpen?: () => void): () => void {
  const url = `/events/${mod}?role=${role}&t=${encodeURIComponent(token())}`
  let es: EventSource | null = null
  let closed = false
  const open = () => {
    es = new EventSource(url)
    es.onopen = () => onOpen && onOpen()
    for (const [k, fn] of Object.entries(handlers)) es.addEventListener(k, (e) => { try { fn(JSON.parse((e as MessageEvent).data)) } catch { /* */ } })
    es.onerror = () => { if (es && es.readyState === 2 && !closed) setTimeout(open, 2000) }
  }
  open()
  return () => { closed = true; es && es.close() }
}

export const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))
