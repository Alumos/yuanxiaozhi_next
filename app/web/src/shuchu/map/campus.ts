/* 校园结构图渲染：移植自 素材/校园/校园结构图.html（用户校准版，唯一的校园数据源）。
 * 统一地理坐标：x 向东，y 向南。平面图 viewBox = -22 -12 1370 1115。
 * 在原版基础上增加：图层开关、楼名隐藏、淡化底图、AI 路线层（画图说明书）、核验标记。 */

export type View = 'plan' | 'iso'
export interface MapOptions {
  view?: View
  corridor?: boolean
  pin?: boolean
  sampleRoutes?: boolean
  trees?: boolean
  labels?: 'all' | 'none'
  legend?: boolean
  dim?: boolean
  blockedStair?: string | null // node id (SE) → draws a 暂停使用 badge at the stair
}

const FH = 22
const EAST_EDGE: number[][] = [[610, 40], [800, 250], [1038, 445], [1145, 735], [1190, 985]]
const CAMPUS_POLY = [[60, 40], ...EAST_EDGE, [60, 985]]
const LAND_POLY = [[-25, -10], [720, -10], ...EAST_EDGE.map(([x, y]) => [x + 170, y]), [1360, 1065], [-25, 1065]]
const FIELD = { x: 245, y: 65, w: 315, h: 535 }
const COURTS = { x: 79, y: 67, w: 145, h: 268 }
const AXIS = { x: 660, y1: 305, y2: 940, w: 24 }
interface Bld { id: string; no?: number; name: string; sub?: string; x: number; y: number; w: number; d: number; floors: number; roof: string; wall?: string; tone?: string; stairs?: string[]; stairInsetW?: number; knownFloors?: boolean; bridge?: boolean; tower?: boolean; solar?: boolean; labelOff?: number[] }
const BUILDINGS: Bld[] = [
  { id: 'library', no: 3, name: '图书馆', x: 592, y: 305, w: 116, d: 98, floors: 3, roof: 'flat', wall: 'white', labelOff: [8, 0] },
  { id: 'library-link', name: '', x: 638, y: 403, w: 42, d: 12, floors: 2, roof: 'flat', bridge: true },
  { id: 'zheng', no: 2, name: '正源楼', x: 592, y: 415, w: 116, d: 142, floors: 3, roof: 'gable', tone: 'light', wall: 'white', labelOff: [0, 0] },
  { id: 'tower', name: '', x: 582, y: 246, w: 25, d: 25, floors: 5, roof: 'flat', tower: true, wall: 'white' },
  { id: 'ben', no: 4, name: '本源楼', x: 672, y: 557, w: 325, d: 67, floors: 5, roof: 'gable', stairs: ['W', 'E'], knownFloors: true },
  { id: 'qing', no: 5, name: '清源楼', x: 672, y: 713, w: 425, d: 67, floors: 4, roof: 'gable', stairs: ['W', 'E'], knownFloors: true },
  { id: 'xing-west', no: 7, name: '行远楼', sub: '西段', x: 672, y: 871, w: 251, d: 67, floors: 4, roof: 'gable', stairs: ['W'], stairInsetW: 91, knownFloors: true },
  { id: 'xing-link', name: '', x: 923, y: 892, w: 32, d: 25, floors: 4, roof: 'flat', bridge: true },
  { id: 'xing-east', name: '行远楼', sub: '东段', x: 955, y: 871, w: 165, d: 67, floors: 4, roof: 'gable', stairs: ['E'], knownFloors: true },
  { id: 'cheng', no: 8, name: '澄源楼', x: 485, y: 637, w: 163, d: 65, floors: 4, roof: 'gable', stairs: ['W'], knownFloors: true },
  { id: 'duan', no: 6, name: '端源楼', x: 401, y: 857, w: 247, d: 67, floors: 4, roof: 'gable', stairs: ['W'], knownFloors: true },
  { id: 'jian', no: 9, name: '健远楼', x: 204, y: 637, w: 253, d: 143, floors: 3, roof: 'hip', tone: 'blue' },
  { id: 'yang', no: 10, name: '养元楼', x: 204, y: 831, w: 151, d: 111, floors: 4, roof: 'flat', wall: 'white', solar: true },
  { id: 'gate-house', name: '', x: 1000, y: 455, w: 35, d: 80, floors: 1, roof: 'flat', wall: 'white' },
]
const CONNECTORS: Bld[] = [
  { id: 'west-jian', name: '', x: 457, y: 659, w: 28, d: 24, floors: 1, roof: 'flat', bridge: true },
  { id: 'west-yang', name: '', x: 355, y: 879, w: 46, d: 24, floors: 1, roof: 'flat', bridge: true },
  { id: 'spine-a', name: '', x: 648, y: 557, w: 24, d: 80, floors: 1, roof: 'flat', bridge: true },
  { id: 'spine-b', name: '', x: 648, y: 637, w: 24, d: 76, floors: 1, roof: 'flat', bridge: true },
  { id: 'spine-c', name: '', x: 648, y: 713, w: 24, d: 144, floors: 1, roof: 'flat', bridge: true },
  { id: 'spine-d', name: '', x: 648, y: 857, w: 24, d: 83, floors: 1, roof: 'flat', bridge: true },
  { id: 'east-a', name: '', x: 970, y: 624, w: 22, d: 89, floors: 1, roof: 'flat', bridge: true },
  { id: 'east-b', name: '', x: 1071, y: 780, w: 22, d: 91, floors: 1, roof: 'flat', bridge: true },
]
const PATHS = [
  { x: AXIS.x - 12, y: AXIS.y1, w: 24, d: AXIS.y2 - AXIS.y1, corridor: true },
  { x: 709, y: 430, w: 326, d: 124 }, { x: 562, y: 278, w: 174, d: 27 }, { x: 708, y: 278, w: 28, d: 152 },
  { x: 736, y: 278, w: 80, d: 25 }, { x: 224, y: 603, w: 424, d: 25 }, { x: 562, y: 305, w: 20, d: 298 },
  { x: 70, y: 350, w: 160, d: 25 }, { x: 224, y: 365, w: 16, d: 263 }, { x: 103, y: 958, w: 1048, d: 22 },
  { x: 355, y: 790, w: 293, d: 32 }, { x: 996, y: 624, w: 155, d: 21 }, { x: 1097, y: 759, w: 54, d: 21 },
  { x: 1120, y: 909, w: 31, d: 24 },
]
const TREE_ZONES = [
  [593, 90, 20, 125, 5], [756, 333, 70, 65, 5], [750, 652, 175, 32, 7], [730, 807, 280, 33, 10], [440, 732, 170, 45, 7],
  [93, 419, 90, 480, 14], [365, 846, 20, 80, 3], [84, 938, 470, 12, 15], [1149, 810, 12, 110, 6], [70, 390, 20, 530, 12], [738, 397, 140, 12, 5],
]
const PIN = { x: 1083, y: 871, z: 2 * FH + 11, plan: [1083, 871], label: '六10 · 行远楼东段3层' }
const ROUTES: Record<string, { color: string; dash: string; pts: number[][] }> = {
  normal: { color: '#27805d', dash: '', pts: [[1083, 871, 55], [1083, 904, 55], [1107, 904, 55], [1107, 904, 0], [1140, 920, 0], [1115, 800, 0], [1115, 735, 0], [1060, 570, 0], [1020, 550, 0], [920, 420, 0], [770, 290, 0], [572, 290, 0], [490, 290, 0], [408, 330, 0]] },
  incident: { color: '#c8862a', dash: '10 7', pts: [[1083, 871, 55], [1083, 904, 55], [939, 904, 55], [763, 904, 55], [763, 904, 0], [660, 904, 0], [660, 616, 0], [572, 616, 0], [488, 616, 0], [488, 550, 0], [408, 330, 0]] },
}

type P3 = (x: number, y: number, z?: number) => number[]
interface Proj { iso: boolean; p: P3 }
export const PLAN_VB = [-22, -12, 1370, 1115]

const f = (v: number) => Math.round(v * 10) / 10
export const esc = (s: unknown) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;')
const box = (x: number, y: number, w: number, d: number, z = 0) => [[x, y, z], [x + w, y, z], [x + w, y + d, z], [x, y + d, z]]
export function proj(view: View): Proj {
  return view === 'plan' ? { iso: false, p: (x, y) => [x, y] } : { iso: true, p: (x, y, z = 0) => [-(x + y) * 0.8660254 * 0.8, (x - y) * 0.5 * 0.8 - z * 0.8] }
}
const points = (P: Proj, a: number[][]) => a.map(([x, y, z = 0]) => P.p(x, y, z).map(f).join(',')).join(' ')
const poly = (P: Proj, a: number[][], style: string) => `<polygon points="${points(P, a)}" ${style}/>`
const line = (P: Proj, a: number[][], style: string) => `<polyline points="${points(P, a)}" fill="none" ${style}/>`
function text(x: number, y: number, s: string, size = 16, attrs = '') {
  return `<text x="${f(x)}" y="${f(y)}" text-anchor="middle" font-size="${size}" ${attrs}>${esc(s)}</text>`
}
export function label(x: number, y: number, s: string, size = 16, color = '#273a43') {
  return text(x, y, s, size, `fill="${color}" font-weight="650" stroke="#fffdf8" stroke-width="4" stroke-linejoin="round" paint-order="stroke"`)
}
function stadium(x: number, y: number, w: number, h: number, n = 40) {
  const r = w / 2, cx = x + r, a: number[][] = []
  for (let i = 0; i <= n; i++) { const t = Math.PI + (i * Math.PI) / n; a.push([cx + r * Math.cos(t), y + r + r * Math.sin(t)]) }
  for (let i = 0; i <= n; i++) { const t = (i * Math.PI) / n; a.push([cx + r * Math.cos(t), y + h - r + r * Math.sin(t)]) }
  return a
}
function circle(P: Proj, x: number, y: number, r: number, attrs: string, z = 0) {
  const a: number[][] = []
  for (let i = 0; i <= 48; i++) { const t = (i / 48) * Math.PI * 2; a.push([x + r * Math.cos(t), y + r * Math.sin(t), z]) }
  return poly(P, a, attrs)
}

let clipSeq = 0
function drawGround(P: Proj) {
  const id = ++clipSeq
  let s = poly(P, LAND_POLY, 'fill="#e5eadb"')
  const strip = (near: number, far: number) => [...EAST_EDGE.map(([x, y]) => [x + near, y]), ...EAST_EDGE.slice().reverse().map(([x, y]) => [x + far, y])]
  s += poly(P, strip(80, 145), 'fill="#9dced6"')
  s += poly(P, box(55, 1015, 1295, 50), 'fill="#9dced6"')
  s += poly(P, strip(5, 65), 'fill="#aeb7bb"')
  s += line(P, EAST_EDGE.map(([x, y]) => [x + 35, y]), 'stroke="#fffefa" stroke-width="2" stroke-dasharray="15 12"')
  s += poly(P, box(-10, -20, 65, 1035), 'fill="#aeb7bb"')
  s += line(P, [[22, -20], [22, 1015]], 'stroke="#fffefa" stroke-width="2" stroke-dasharray="15 12"')
  s += poly(P, CAMPUS_POLY, 'fill="#dce6ca" stroke="#98ac92" stroke-width="2"')
  s += `<clipPath id="cc${id}"><polygon points="${points(P, CAMPUS_POLY)}"/></clipPath><g clip-path="url(#cc${id})">`
  for (const a of [[576, 288, 150, 285], [190, 624, 930, 323], [708, 419, 327, 137]]) s += poly(P, box(a[0], a[1], a[2], a[3]), 'fill="#e7dfcf"')
  for (const a of [[690, 643, 260, 53], [690, 797, 355, 55], [420, 724, 207, 110], [220, 796, 155, 22]]) s += poly(P, box(a[0], a[1], a[2], a[3]), 'fill="#cbdcba"')
  for (const p of PATHS) s += poly(P, box(p.x, p.y, p.w, p.d), `fill="${p.corridor ? '#e8c7b0' : '#f4eddf'}" stroke="#d4cbbb" stroke-width=".7"`)
  s += line(P, [[1140, 945], [1115, 800], [1115, 735], [1060, 570], [1020, 550], [920, 420], [770, 290], [572, 290]], 'stroke="#f4eddf" stroke-width="23" stroke-linejoin="round"')
  s += poly(P, stadium(FIELD.x - 8, FIELD.y - 8, FIELD.w + 16, FIELD.h + 16), 'fill="#86aaa1"')
  s += poly(P, stadium(FIELD.x, FIELD.y, FIELD.w, FIELD.h), 'fill="#b96051"')
  for (let i = 1; i < 6; i++) { const o = i * 5.2; s += poly(P, stadium(FIELD.x + o, FIELD.y + o, FIELD.w - o * 2, FIELD.h - o * 2), 'fill="none" stroke="#e9b6a8" stroke-width=".8"') }
  const inner = stadium(FIELD.x + 32, FIELD.y + 32, FIELD.w - 64, FIELD.h - 64)
  s += `<clipPath id="fc${id}"><polygon points="${points(P, inner)}"/></clipPath><g clip-path="url(#fc${id})">`
  for (let i = 0; i < 14; i++) s += poly(P, box(FIELD.x, FIELD.y + (i * FIELD.h) / 14, FIELD.w, FIELD.h / 14 + 1), `fill="${i % 2 ? '#9eae70' : '#a8b879'}"`)
  s += '</g>'
  const fx = FIELD.x + 47, fy = FIELD.y + 111, fw = FIELD.w - 94, fh = FIELD.h - 222, cx = FIELD.x + FIELD.w / 2, cy = FIELD.y + FIELD.h / 2
  s += poly(P, box(fx, fy, fw, fh), 'fill="none" stroke="#f7f4dc" stroke-width="1.8"')
  s += line(P, [[fx, cy], [fx + fw, cy]], 'stroke="#f7f4dc" stroke-width="1.5"')
  s += circle(P, cx, cy, 28, 'fill="none" stroke="#f7f4dc" stroke-width="1.5"')
  for (const y of [fy, fy + fh - 37]) s += poly(P, box(cx - 51, y, 102, 37), 'fill="none" stroke="#f7f4dc" stroke-width="1.5"')
  s += poly(P, box(COURTS.x, COURTS.y, COURTS.w, COURTS.h), 'fill="#6a96b3"')
  for (let j = 0; j < 2; j++) for (let i = 0; i < 2; i++) {
    const x = COURTS.x + 8 + i * 68, y = COURTS.y + 9 + j * 129, w = 61, h = 119
    s += poly(P, box(x, y, w, h), 'fill="#4b82a7" stroke="#e0ecdf" stroke-width="1"')
    s += line(P, [[x, y + h / 2], [x + w, y + h / 2]], 'stroke="#e0ecdf" stroke-width="1"')
    s += circle(P, x + w / 2, y + h / 2, 10, 'fill="none" stroke="#e0ecdf" stroke-width="1"')
    for (const yy of [y, y + h - 19]) s += poly(P, box(x + w / 2 - 11, yy, 22, 19), 'fill="#b47c68" stroke="#e0ecdf" stroke-width="1"')
  }
  for (let i = 0; i < 5; i++) s += line(P, [[748 + i * 50, 438], [748 + i * 50, 544]], 'stroke="#d4ccbc" stroke-width="1"')
  s += '</g>'
  for (let y = 470; y < 530; y += 12) { const x = 1038 + ((y - 445) * 107) / 290; s += poly(P, box(x + 7, y, 55, 6), 'fill="#fffdf7"') }
  return s
}
const TONES: Record<string, string[]> = { dark: ['#687276', '#4d595e'], light: ['#bbc8cb', '#9aafb7'], blue: ['#9ab7c7', '#7899b0'] }
const roofHeight = (b: Bld) => (b.roof === 'flat' ? 0 : Math.min(b.w, b.d) * 0.24)
function drawRoof(P: Proj, b: Bld, z: number) {
  const { x, y, w, d } = b
  let s = ''
  const c = TONES[b.tone || 'dark']
  if (b.roof === 'flat') {
    s += poly(P, box(x, y, w, d, z), `fill="${b.bridge ? '#cbbfae' : '#e6e2d8'}" stroke="#a8a296" stroke-width="1"`)
    if (!b.bridge) s += poly(P, box(x + 5, y + 5, w - 10, d - 10, z), 'fill="none" stroke="#bbb7ae" stroke-width="1"')
    if (b.solar) for (let i = 0; i < 4; i++) s += poly(P, box(x + 15 + i * 30, y + 25, 24, 45, z + 0.5), 'fill="#5e778a" stroke="#acc4d0" stroke-width=".7"')
    return s
  }
  const r = roofHeight(b), ns = d > w
  if (b.roof === 'hip') {
    const k = Math.min(w, d) * 0.36, A = [x + k, y + d / 2, z + r], B = [x + w - k, y + d / 2, z + r]
    s += poly(P, [[x, y, z], [x + w, y, z], B, A], `fill="${c[0]}" stroke="#58686e" stroke-width="1"`)
    s += poly(P, [[x + w, y, z], [x + w, y + d, z], B], `fill="${c[1]}" stroke="#58686e" stroke-width="1"`)
    s += poly(P, [[x + w, y + d, z], [x, y + d, z], A, B], `fill="${c[1]}" stroke="#58686e" stroke-width="1"`)
    s += poly(P, [[x, y + d, z], [x, y, z], A], `fill="${c[0]}" stroke="#58686e" stroke-width="1"`)
    s += line(P, [A, B], 'stroke="#4d5b64" stroke-width="2"')
  } else if (ns) {
    const A = [x + w / 2, y, z + r], B = [x + w / 2, y + d, z + r]
    s += poly(P, [[x, y, z], [x, y + d, z], B, A], `fill="${c[0]}" stroke="#66777d" stroke-width="1"`)
    s += poly(P, [A, B, [x + w, y + d, z], [x + w, y, z]], `fill="${c[1]}" stroke="#66777d" stroke-width="1"`)
    if (P.iso) s += poly(P, [[x, y, z], [x + w, y, z], A], 'fill="#ece6d9" stroke="#b7b4a8" stroke-width=".8"')
    s += line(P, [A, B], 'stroke="#56656c" stroke-width="2"')
  } else {
    const A = [x, y + d / 2, z + r], B = [x + w, y + d / 2, z + r]
    s += poly(P, [A, B, [x + w, y + d, z], [x, y + d, z]], `fill="${c[1]}" stroke="#46545b" stroke-width="1"`)
    s += poly(P, [[x, y, z], [x + w, y, z], B, A], `fill="${c[0]}" stroke="#46545b" stroke-width="1"`)
    if (P.iso) s += poly(P, [[x + w, y, z], [x + w, y + d, z], B], 'fill="#e8d7ca" stroke="#a8998a" stroke-width=".8"')
    for (let xx = x + 12; xx < x + w; xx += 15) s += line(P, [[xx, y, z], [xx, y + d / 2, z + r]], 'stroke="#ffffff" stroke-opacity=".13" stroke-width=".7"')
    s += line(P, [A, B], 'stroke="#3f4a52" stroke-width="2"')
  }
  return s
}
function drawBuilding(P: Proj, b: Bld) {
  const { x, y, w, d } = b, h = b.floors * FH
  let s = '<g>'
  s += poly(P, box(x + 7, y + 8, w, d), 'fill="#263831" fill-opacity=".14"')
  if (P.iso) {
    const white = b.wall === 'white', front = white ? '#f1eee5' : '#e4d3c5', side = white ? '#deded2' : '#d1bbab'
    s += poly(P, [[x, y, 0], [x + w, y, 0], [x + w, y, h], [x, y, h]], `fill="${front}" stroke="#a99d8e" stroke-width=".7"`)
    s += poly(P, [[x + w, y, 0], [x + w, y + d, 0], [x + w, y + d, h], [x + w, y, h]], `fill="${side}" stroke="#a99d8e" stroke-width=".7"`)
    for (let fl = 0; fl < b.floors; fl++) {
      const z = fl * FH
      if (fl > 0) s += line(P, [[x, y, z], [x + w, y, z], [x + w, y + d, z]], 'stroke="#b7a494" stroke-width="1.2"')
      if (b.bridge) {
        for (let xx = x + 5; xx < x + w - 8; xx += 18) s += poly(P, [[xx, y, z + 2], [xx + 10, y, z + 2], [xx + 10, y, z + FH - 4], [xx, y, z + FH - 4]], 'fill="#6e7c79"')
        for (let yy = y + 5; yy < y + d - 7; yy += 20) s += poly(P, [[x + w, yy, z + 2], [x + w, yy + 12, z + 2], [x + w, yy + 12, z + FH - 4], [x + w, yy, z + FH - 4]], 'fill="#72807b"')
      } else if (!b.tower) {
        for (let xx = x + 9; xx + 12 < x + w - 5; xx += 24) s += poly(P, [[xx, y, z + 5], [xx + 12, y, z + 5], [xx + 12, y, z + 17], [xx, y, z + 17]], 'fill="#657b83"')
        for (let yy = y + 9; yy + 12 < y + d - 5; yy += 24) s += poly(P, [[x + w, yy, z + 5], [x + w, yy + 12, z + 5], [x + w, yy + 12, z + 17], [x + w, yy, z + 17]], 'fill="#5b717b"')
      }
    }
  }
  s += drawRoof(P, b, P.iso ? h : 0)
  return s + '</g>'
}
function rng(seed: number) { return () => { seed = (seed * 1664525 + 1013904223) >>> 0; return seed / 4294967296 } }
function trees() { const r = rng(2026), a: { x: number; y: number; r: number }[] = []; for (const [x, y, w, d, n] of TREE_ZONES) for (let i = 0; i < n; i++) a.push({ x: x + r() * w, y: y + r() * d, r: 5 + r() * 4 }); return a }
function drawTree(P: Proj, t: { x: number; y: number; r: number }) {
  const [x, y] = P.p(t.x, t.y, 0), [tx, ty] = P.p(t.x, t.y, P.iso ? 16 : 0)
  return `<ellipse cx="${f(x + 3)}" cy="${f(y + 3)}" rx="${f(t.r + 1)}" ry="${f(P.iso ? t.r * 0.5 : t.r)}" fill="#455d39" opacity=".14"/><path d="M${f(x)},${f(y)} L${f(tx)},${f(ty)}" stroke="#98846a" stroke-width="2"/><circle cx="${f(tx)}" cy="${f(ty)}" r="${f(t.r)}" fill="#7e9b67" stroke="#6a8959" stroke-width=".6"/>`
}
function badge(x: number, y: number, n: number) {
  return `<circle cx="${f(x)}" cy="${f(y)}" r="10" fill="#31758d" stroke="white" stroke-width="1.5"/><text x="${f(x)}" y="${f(y + 4)}" text-anchor="middle" font-size="12" fill="white" font-weight="700">${n}</text>`
}
function stairPoints(b: Bld) { return (b.stairs || []).map((s) => ({ x: s === 'W' ? b.x + (b.stairInsetW || 13) : b.x + b.w - 13, y: b.y + b.d / 2, z: b.floors * FH + roofHeight(b) + 6 })) }
function stairIcon(x: number, y: number) {
  return `<g transform="translate(${f(x)},${f(y)})"><rect x="-9" y="-9" width="18" height="18" rx="4" fill="#43846c" stroke="white" stroke-width="1.6"/><path d="M-6,5 h4 v-4 h4 v-4 h4" stroke="white" stroke-width="1.8" fill="none"/></g>`
}
function buildingLabels(P: Proj) {
  let s = ''
  for (const b of BUILDINGS) {
    if (!b.name) continue
    let [x, y] = P.p(b.x + b.w / 2, b.y + b.d / 2, P.iso ? b.floors * FH + roofHeight(b) + 12 : 0)
    if (P.iso && b.labelOff) { x += b.labelOff[0]; y += b.labelOff[1] }
    const title = b.name + (b.sub ? '·' + b.sub : '')
    const size = P.iso ? 15 : 17
    s += badge(x - (title.length * size) / 2 - 12, y - 5, b.no || 7) + label(x, y, title, size)
    if (b.knownFloors) s += label(x, y + 17, `${b.floors}层`, 12, '#566966')
  }
  return s
}
function corridor(P: Proj) {
  const a = [[AXIS.x, AXIS.y1, 0], [AXIS.x, AXIS.y2, 0]]
  let s = line(P, a, 'stroke="#fff8ed" stroke-width="7" stroke-linecap="round"')
  s += line(P, a, 'stroke="#cf4c36" stroke-width="3.5" stroke-dasharray="10 7" stroke-linecap="round"')
  for (const y of [305, 557, 713, 938]) { const [xp, yp] = P.p(AXIS.x, y, 0); s += `<circle cx="${f(xp)}" cy="${f(yp)}" r="4" fill="#cf4c36" stroke="#fff" stroke-width="1.5"/>` }
  return s
}
function corridorLabel(P: Proj) {
  if (!P.iso) { const x = AXIS.x - 29, y = 753; return ['一', '楼', '中', '廊', '道'].map((c, i) => label(x, y + i * 19, c, 14, '#a83f2e')).join('') }
  const [x, y] = P.p(AXIS.x, 803, 0)
  return label(x + 28, y + 26, '一楼中廊道', 14, '#a83f2e')
}
function pin(P: Proj) {
  const [x, y] = P.iso ? P.p(PIN.x, PIN.y, PIN.z) : PIN.plan
  let s = `<path d="M${f(x)},${f(y)} c-8,-12 -12,-18 -12,-25 a12,12 0 1 1 24,0 c0,7 -4,13 -12,25z" fill="#c13e3e" stroke="white" stroke-width="2"/><circle cx="${f(x)}" cy="${f(y - 25)}" r="4" fill="white"/>`
  s += label(x + (P.iso ? 22 : 0), y - (P.iso ? 46 : 45), PIN.label, 14, '#b33032')
  return s
}
function sampleRoutes(P: Proj) {
  let s = ''
  for (const [k, r] of Object.entries(ROUTES)) {
    s += line(P, r.pts, 'stroke="#fffdf7" stroke-width="7" stroke-linejoin="round" stroke-linecap="round"')
    s += line(P, r.pts, `stroke="${r.color}" stroke-width="3.5" stroke-dasharray="${r.dash}" stroke-linejoin="round" stroke-linecap="round" marker-end="url(#arrow-${k})"`)
  }
  return s
}
function compass(x: number, y: number, P: Proj) {
  const [o1, o2] = P.p(0, 0), [n1, n2] = P.p(0, -1)
  const a = (Math.atan2(n1 - o1, -(n2 - o2)) * 180) / Math.PI
  let s = `<circle cx="${x}" cy="${y}" r="29" fill="#fffdf7" stroke="#d6ded2"/><g transform="translate(${x},${y}) rotate(${a})"><path d="M0,-22 L8,10 L0,5 L-8,10z" fill="#aa493c"/><path d="M0,22 L8,10 L0,5 L-8,10z" fill="#a5b1aa"/></g>`
  const rad = (a * Math.PI) / 180
  s += label(x + Math.sin(rad) * 43, y - Math.cos(rad) * 43 + 5, '北', 17, '#9b473d')
  return s
}
function legend(x: number, y: number, w: number, P: Proj) {
  let s = `<rect x="${x}" y="${y}" width="${w}" height="70" rx="9" fill="#fffdf8" stroke="#d5ddcf"/>`
  s += text(x + w / 2, y + 26, P.iso ? '校园俯瞰 · 从东北侧观察' : '校园平面 · 上北下南 / 左西右东', 18, 'fill="#304d51" font-weight="700"')
  const entries = [['#cf4c36', '一楼中廊道'], ['#43846c', '楼梯'], ['#cabdad', '楼栋连接廊'], ['#f4eddf', '步道 / 广场']]
  const ew = Math.min(185, (w - 30) / entries.length), start = x + (w - ew * entries.length) / 2
  entries.forEach(([color, t], i) => {
    const xx = start + i * ew
    s += `<rect x="${f(xx)}" y="${y + 41}" width="17" height="12" rx="3" fill="${color}" stroke="#c7c8ba" stroke-width=".6"/>`
    s += text(xx + 25 + (t.length * 12) / 2, y + 52, t, 13, 'fill="#536862"')
  })
  return s
}

export const STAIR_POS: Record<string, number[]> = { SE: [1107, 904], SW: [763, 904] }

export function blockedBadge(x: number, y: number, scale = 1) {
  const s = scale
  return `<g transform="translate(${f(x)},${f(y)}) scale(${s})"><circle r="19" fill="#d63a2f" stroke="#fff" stroke-width="3"/><rect x="-11" y="-3.5" width="22" height="7" rx="2" fill="#fff"/></g>` +
    label(x, y - 26 * s, '暂停使用', 15 * s, '#c0291f')
}

export interface Scene { body: string; vb: number[] }

/** Renders the campus base map as SVG markup (without the outer <svg>). */
export function renderCampus(o: MapOptions = {}): Scene {
  const opt = { view: 'plan' as View, corridor: true, pin: false, sampleRoutes: false, trees: true, labels: 'all' as const, legend: true, dim: false, ...o }
  const P = proj(opt.view)
  let s = `<defs>${Object.entries(ROUTES).map(([k, r]) => `<marker id="arrow-${k}" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="4" markerHeight="4" orient="auto"><path d="M0,0 L10,5 L0,10z" fill="${r.color}"/></marker>`).join('')}</defs>`
  const all = [...BUILDINGS, ...CONNECTORS]
  s += drawGround(P)
  if (P.iso) {
    const objects = all.map((b) => ({ depth: b.x + b.w - b.y, svg: drawBuilding(P, b) }))
    if (opt.trees) for (const t of trees()) objects.push({ depth: t.x - t.y, svg: drawTree(P, t) })
    objects.sort((a, b) => a.depth - b.depth)
    s += objects.map((o) => o.svg).join('')
  } else {
    if (opt.trees) for (const t of trees()) s += drawTree(P, t)
    for (const b of all) s += drawBuilding(P, b)
  }
  if (opt.sampleRoutes) s += sampleRoutes(P)
  if (opt.corridor) s += corridor(P)
  for (const b of BUILDINGS) for (const st of stairPoints(b)) { const [x, y] = P.p(st.x, st.y, P.iso ? st.z : 0); s += stairIcon(x, y) }
  let vb: number[]
  if (P.iso) {
    const corners = LAND_POLY.map(([x, y]) => P.p(x, y, 0))
    const xs = corners.map((c) => c[0]), ys = corners.map((c) => c[1])
    const minX = Math.floor(Math.min(...xs)) - 25, maxX = Math.ceil(Math.max(...xs)) + 25, minY = Math.floor(Math.min(...ys)) - 24, maxY = Math.ceil(Math.max(...ys)) + 18
    vb = [minX, minY, maxX - minX, maxY - minY + (opt.legend ? 90 : 0)]
    if (opt.dim) s += `<rect x="${vb[0]}" y="${vb[1]}" width="${vb[2]}" height="${vb[3]}" fill="#fffdf8" opacity=".45"/>`
    s += compass(maxX - 95, minY + 70, P)
    if (opt.legend) s += legend(minX + 80, maxY + 5, maxX - minX - 160, P)
  } else {
    vb = PLAN_VB
    if (opt.dim) s += `<rect x="${vb[0]}" y="${vb[1]}" width="${vb[2]}" height="${vb[3]}" fill="#fffdf8" opacity=".45"/>`
    s += compass(1100, 100, P)
    if (opt.legend) s += legend(75, 1010, 1110, P)
  }
  if (opt.labels === 'all') {
    s += buildingLabels(P)
    if (opt.corridor) s += corridorLabel(P)
    for (const [x, y, t, size, c] of [[402, 320, '⑪ 操场', P.iso ? 24 : 28, '#416748'], [151, 205, '⑫ 篮球场', P.iso ? 15 : 17, '#285b78'], [857, 495, '主入口广场', 17, '#71614e'], [1035, 430, '校门', 16, '#4f625f'], [1220, 590, '河道', 16, '#437b8b']] as [number, number, string, number, string][]) {
      const [a, b] = P.p(x, y, 0)
      s += label(a, b, t, size, c)
    }
    if (!P.iso) { s += label(1164, 685, '东侧道路', 14, '#556972'); s += label(22, 588, '西侧道路', 14, '#556972') }
  }
  if (opt.pin) s += pin(P)
  if (opt.blockedStair && STAIR_POS[opt.blockedStair]) {
    const [x, y] = STAIR_POS[opt.blockedStair]
    const [px, py] = P.p(x, y, P.iso ? 4 * FH + 20 : 0)
    s += blockedBadge(px, py)
  }
  s = `<rect x="${vb[0]}" y="${vb[1]}" width="${vb[2]}" height="${vb[3]}" fill="#fffdf8"/>` + s
  return { body: s, vb }
}

export const SVG_FONT = 'PingFang SC, Hiragino Sans GB, Noto Sans CJK SC, Noto Sans SC, Source Han Sans SC, Microsoft YaHei, sans-serif'

export function wrapSvg(scene: Scene, extra = '', cls = '') {
  return `<svg xmlns="http://www.w3.org/2000/svg" class="${cls}" viewBox="${scene.vb.join(' ')}" font-family="${SVG_FONT}" role="img" aria-label="校园平面图">${scene.body}${extra}</svg>`
}
