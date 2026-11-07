/* 按“画图说明书”在校园平面图上画路线，并栅格化成 PNG。
 * 学生看到的是“元小智把地图改好发回来”，说明书默认不展示。 */
import { esc, label, renderCampus, wrapSvg, SVG_FONT, PLAN_VB, STAIR_POS, blockedBadge } from './campus'
import type { DrawSpec, DrawResult, RoadNode, RouteIssue } from '../types'

export const COLORS: Record<string, string> = {
  默认: '#7f98a8', 绿色: '#16a05a', 红色: '#dc3b2f', 橙色: '#ee8a12', 蓝色: '#2c6fe0',
  紫色: '#8a4fd8', 黄色: '#f2b705', 粉色: '#ec5a9e', 青色: '#11a5b5', 黑色: '#22292e', 棕色: '#8a5a3c',
}
export const WIDTHS: Record<string, number> = { 默认: 3, 细: 3, 中: 6.5, 粗: 11, 特粗: 16 }
export const LINE_STYLES = ['实线', '虚线', '点线', '双线', '发光']
export const ARROW_SHAPES = ['三角', '燕尾', '脚印']
export const MARKER_TYPES = ['五角星', '红旗', '安全出口', '暂停使用', '文字', '圆圈', '警告', '小人', '灭火器', '对勾', '爱心']
export const THEMES = ['默认', '简洁', '醒目', '卡通', '夜间', '黑白']

function star(x: number, y: number, r: number, fill = '#e3342f') {
  const pts: string[] = []
  for (let i = 0; i < 10; i++) {
    const a = -Math.PI / 2 + (i * Math.PI) / 5
    const rr = i % 2 ? r * 0.45 : r
    pts.push(`${(x + rr * Math.cos(a)).toFixed(1)},${(y + rr * Math.sin(a)).toFixed(1)}`)
  }
  return `<polygon points="${pts.join(' ')}" fill="${fill}" stroke="#fff" stroke-width="3" stroke-linejoin="round"/>`
}
function flag(x: number, y: number, k: number, fill = '#e3342f') {
  return `<g transform="translate(${x},${y}) scale(${k})"><rect x="-2" y="-46" width="4" height="48" rx="2" fill="#6b4b3a"/><path d="M2,-46 L34,-36 L2,-24 Z" fill="${fill}" stroke="#fff" stroke-width="2" stroke-linejoin="round"/></g>`
}
function exitSign(x: number, y: number, k: number) {
  // green box with a simple running figure and an arrow
  return `<g transform="translate(${x},${y}) scale(${k})"><rect x="-26" y="-20" width="52" height="40" rx="6" fill="#119a4f" stroke="#fff" stroke-width="3"/>` +
    `<circle cx="-9" cy="-9" r="4.5" fill="#fff"/><path d="M-9,-4 L-12,6 L-18,13 M-11,2 L-3,9 M-10,-2 L-2,-5 M-10,-1 L-17,1" stroke="#fff" stroke-width="3.2" stroke-linecap="round" fill="none"/>` +
    `<path d="M3,0 H18 M12,-6 L18,0 L12,6" stroke="#fff" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round" fill="none"/></g>`
}
function runner(x: number, y: number, k: number, fill: string) {
  return `<g transform="translate(${x},${y - 30 * k}) scale(${k})"><circle r="22" fill="${fill}" stroke="#fff" stroke-width="3"/>` +
    `<circle cx="2" cy="-11" r="4.5" fill="#fff"/><path d="M1,-6 L-2,4 L-9,12 M-1,0 L7,7 M0,-4 L9,-6 M0,-3 L-8,-1" stroke="#fff" stroke-width="3.4" stroke-linecap="round" fill="none"/></g>`
}
function warn(x: number, y: number, k: number) {
  return `<g transform="translate(${x},${y - 30 * k}) scale(${k})"><path d="M0,-24 L24,18 L-24,18 Z" fill="#f2b705" stroke="#fff" stroke-width="3.5" stroke-linejoin="round"/>` +
    `<rect x="-3" y="-10" width="6" height="16" rx="2" fill="#22292e"/><circle cy="12" r="3.4" fill="#22292e"/></g>`
}
function extinguisher(x: number, y: number, k: number) {
  return `<g transform="translate(${x + 26 * k},${y - 26 * k}) scale(${k})"><rect x="-9" y="-14" width="18" height="34" rx="7" fill="#d6301e" stroke="#fff" stroke-width="2.5"/>` +
    `<rect x="-5" y="-21" width="10" height="7" rx="2" fill="#22292e"/><path d="M5,-18 Q18,-20 16,-6" stroke="#22292e" stroke-width="3" fill="none"/></g>`
}
function check(x: number, y: number, k: number) {
  return `<g transform="translate(${x},${y - 28 * k}) scale(${k})"><circle r="20" fill="#16a05a" stroke="#fff" stroke-width="3"/><path d="M-9,0 L-2,8 L10,-7" stroke="#fff" stroke-width="4.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/></g>`
}
function heart(x: number, y: number, k: number, fill: string) {
  return `<g transform="translate(${x},${y - 26 * k}) scale(${k})"><path d="M0,16 C-26,-2 -16,-24 0,-12 C16,-24 26,-2 0,16 Z" fill="${fill}" stroke="#fff" stroke-width="3" stroke-linejoin="round"/></g>`
}

export interface RouteRenderOpts {
  nodes: RoadNode[]
  incidentNode?: string | null
  showIssues?: boolean
  animate?: boolean
}

type Pt = { x: number; y: number }
/** 沿折线每隔 gap 取一个点（含方向角），用来画燕尾箭头和脚印。 */
function along(pts: Pt[], gap: number, offset = gap / 2) {
  const out: { x: number; y: number; a: number }[] = []
  let carry = offset
  for (let i = 1; i < pts.length; i++) {
    const p = pts[i - 1], q = pts[i]
    const len = Math.hypot(q.x - p.x, q.y - p.y)
    const a = Math.atan2(q.y - p.y, q.x - p.x)
    let d = carry
    while (d < len) {
      out.push({ x: p.x + (q.x - p.x) * (d / len), y: p.y + (q.y - p.y) * (d / len), a })
      d += gap
    }
    carry = d - len
  }
  return out
}
const deg = (a: number) => ((a * 180) / Math.PI).toFixed(1)

export function routeLayer(res: DrawResult, o: RouteRenderOpts): string {
  const spec: DrawSpec = res.spec || ({} as DrawSpec)
  const byId: Record<string, RoadNode> = {}
  for (const n of o.nodes) byId[n.id] = n
  const pts = (res.path || []).map((id) => byId[id]).filter(Boolean)
  const line: DrawSpec['line'] = { color: '默认', width: '默认', dash: false, arrow: false, ...((spec.line || {}) as Partial<DrawSpec['line']>) }
  const color = COLORS[line.color] || COLORS['默认']
  const theme = THEMES.includes(spec.theme) ? spec.theme : '默认'
  const loud = theme === '醒目', cartoon = theme === '卡通', night = theme === '夜间'
  let style = LINE_STYLES.includes(line.style || '') ? line.style! : line.dash ? '虚线' : '实线'
  if (night && style === '实线') style = '发光'
  let w = WIDTHS[line.width] || 3
  if (loud || cartoon) w *= 1.25
  const shape = ARROW_SHAPES.includes(line.arrow_shape || '') ? line.arrow_shape! : '三角'
  const arrows = !!line.arrow || !!line.arrow_shape
  const markScale = loud ? 1.35 : cartoon ? 1.3 : 1
  let s = ''
  const d = pts.map((p, i) => `${i ? 'L' : 'M'}${p.x},${p.y}`).join(' ')
  const anim = o.animate ? 'class="route-draw" pathLength="1"' : ''
  const stroke = (sw: number, col: string, extra = '') => `<path d="${d}" fill="none" stroke="${col}" stroke-width="${sw.toFixed(1)}" stroke-linejoin="round" stroke-linecap="round" ${extra}/>`
  if (pts.length > 1) {
    // 用脚印表示方向时，线淡一些，让脚印更显眼
    s += `<g opacity="${shape === '脚印' && arrows ? 0.4 : 1}">`
    // 底衬：让线在地图上跳出来
    if (cartoon) s += stroke(w + 12, '#2a2a2a') + stroke(w + 7, '#ffffff')
    else if (style !== '发光') s += stroke(loud ? w + 8 : w + 3, '#ffffff', `stroke-opacity="${loud ? 1 : 0.8}"`)
    switch (style) {
      case '虚线': s += stroke(w, color, `stroke-dasharray="${(w * 2.6).toFixed(1)} ${(w * 1.8).toFixed(1)}" stroke-linecap="butt"`); break
      case '点线': s += stroke(w * 1.15, color, `stroke-dasharray="0 ${(w * 2.4).toFixed(1)}"`); break
      case '双线': s += stroke(w * 1.9, color, anim) + stroke(w * 0.7, '#ffffff'); break
      case '发光': s += stroke(w * 4.2, color, 'stroke-opacity=".16"') + stroke(w * 2.6, color, 'stroke-opacity=".3"') + stroke(w, color, anim) + stroke(Math.max(1.5, w * 0.35), '#ffffff', 'stroke-opacity=".85"'); break
      default: s += stroke(w, color, anim)
    }
    s += '</g>'
  }
  if (pts.length > 1 && arrows) {
    const k = Math.max(w, 5)
    if (shape === '燕尾') {
      for (const p of along(pts, Math.max(46, k * 6))) {
        s += `<path class="route-mark" d="M${-k},${-k} L${k * 0.4},0 L${-k},${k}" transform="translate(${p.x.toFixed(1)},${p.y.toFixed(1)}) rotate(${deg(p.a)})" fill="none" stroke="${style === '双线' || style === '发光' ? '#fff' : color}" stroke-width="${Math.max(3, k * 0.45).toFixed(1)}" stroke-linecap="round" stroke-linejoin="round"/>`
      }
    } else if (shape === '脚印') {
      along(pts, 30).forEach((p, i) => {
        const side = i % 2 ? 1 : -1
        const ox = -Math.sin(p.a) * 7 * side, oy = Math.cos(p.a) * 7 * side
        s += `<g class="route-mark" transform="translate(${(p.x + ox).toFixed(1)},${(p.y + oy).toFixed(1)}) rotate(${deg(p.a)})"><ellipse rx="8" ry="5" fill="${color}" stroke="#fff" stroke-width="1.6"/><circle cx="10" cy="0" r="2.6" fill="${color}"/></g>`
      })
    } else {
      // repeated direction arrows along the route
      for (let i = 1; i < pts.length; i++) {
        const a = pts[i - 1], b = pts[i]
        const len = Math.hypot(b.x - a.x, b.y - a.y)
        if (len < 60 && i < pts.length - 1) continue
        const mx = a.x + (b.x - a.x) * 0.55, my = a.y + (b.y - a.y) * 0.55
        const kk = k * 1.5
        s += `<path class="route-mark" d="M${-kk},${-kk * 0.9} L${kk * 0.8},0 L${-kk},${kk * 0.9} z" transform="translate(${mx.toFixed(1)},${my.toFixed(1)}) rotate(${deg(Math.atan2(b.y - a.y, b.x - a.x))})" fill="${color}" stroke="#fff" stroke-width="2"/>`
      }
    }
    // 终点的大箭头
    const a = pts[pts.length - 2], b = pts[pts.length - 1]
    const kk = k * 2
    s += `<path class="route-mark" d="M${-kk},${-kk} L${kk},0 L${-kk},${kk} z" transform="translate(${b.x},${b.y}) rotate(${deg(Math.atan2(b.y - a.y, b.x - a.x))})" fill="${color}" stroke="#fff" stroke-width="2.5"/>`
  }
  // 每个转弯处标序号（给低年级一步一步看）
  if (spec.numbers && pts.length > 1) {
    pts.forEach((p, i) => {
      if (i === 0) return
      const r = 13 * markScale
      s += `<g class="route-mark" style="animation-delay:${0.8 + i * 0.12}s"><circle cx="${p.x}" cy="${p.y}" r="${r}" fill="#fff" stroke="${color}" stroke-width="3.5"/>` +
        `<text x="${p.x}" y="${p.y + r * 0.42}" text-anchor="middle" font-size="${(r * 1.25).toFixed(1)}" font-weight="800" fill="${color === '#ffffff' ? '#22292e' : color}">${i}</text></g>`
    })
  }
  // markers
  ;(spec.markers || []).slice(0, 8).forEach((m, i) => {
    const n = byId[(m.at || '').toUpperCase()]
    if (!n) return
    const delay = `style="animation-delay:${1 + i * 0.25}s"`
    const mc = COLORS[m.color || ''] || ''
    let g = ''
    const k = markScale
    let ty = 34
    switch (m.type) {
      case '五角星': g = star(n.x, n.y - 4, 20 * k, mc || '#e3342f'); break
      case '红旗': g = flag(n.x, n.y, 1.1 * k, mc || '#e3342f'); break
      case '安全出口': g = exitSign(n.x + 30 * k, n.y - 26 * k, k); break
      case '暂停使用': g = blockedBadge(n.x, n.y, k); break
      case '圆圈': g = `<circle cx="${n.x}" cy="${n.y}" r="${30 * k}" fill="none" stroke="${mc || '#dc3b2f'}" stroke-width="5"/>`; ty = 46; break
      case '警告': g = warn(n.x, n.y, k); break
      case '小人': g = runner(n.x, n.y, k, mc || '#16a05a'); break
      case '灭火器': g = extinguisher(n.x, n.y, k); break
      case '对勾': g = check(n.x, n.y, k); break
      case '爱心': g = heart(n.x, n.y, k, mc || '#ec5a9e'); break
      case '文字': ty = 0; break
      default: g = `<circle cx="${n.x}" cy="${n.y}" r="${7 * k}" fill="${mc || color}" stroke="#fff" stroke-width="2"/>`
    }
    const txt = (m.text || '').slice(0, 8)
    if (txt) {
      if (m.type === '文字') {
        const fs = 22 * k, bw = txt.length * fs + 22
        g += `<rect x="${n.x - bw / 2}" y="${n.y - fs - 16}" width="${bw}" height="${fs + 16}" rx="${(fs + 16) / 2}" fill="${mc || color}" stroke="#fff" stroke-width="2.5"/>` +
          `<text x="${n.x}" y="${n.y - 14}" text-anchor="middle" font-size="${fs}" font-weight="800" fill="#fff">${esc(txt)}</text>`
      } else if (night) {
        g += `<text x="${n.x}" y="${n.y + ty * k}" text-anchor="middle" font-size="${19 * k}" font-weight="700" fill="#ffffff" stroke="#0b1726" stroke-width="5" stroke-linejoin="round" paint-order="stroke">${esc(txt)}</text>`
      } else g += label(n.x, n.y + ty * k, txt, 19 * k, '#1d2b33')
    }
    s += `<g class="route-mark" ${delay}>${g}</g>`
  })
  if (o.incidentNode && STAIR_POS[o.incidentNode]) {
    const [x, y] = STAIR_POS[o.incidentNode]
    s += blockedBadge(x, y)
  }
  // 标题横幅
  const title = String(spec.title || '').slice(0, 14)
  if (title) {
    const fs = 34, bw = title.length * fs + 60
    const bg = night ? '#0d1b2a' : cartoon ? '#fff3c4' : '#ffffff'
    s += `<g class="route-mark" style="animation-delay:.6s"><rect x="${640 - bw / 2}" y="6" width="${bw}" height="58" rx="16" fill="${bg}" fill-opacity=".95" stroke="${color}" stroke-width="4"/>` +
      `<text x="640" y="47" text-anchor="middle" font-size="${fs}" font-weight="800" fill="${night ? '#fff' : '#22292e'}">${esc(title)}</text></g>`
  }
  // explanation text block (only when labels are kept and style is not 简洁)
  if (spec.explain && spec.labels !== '只留关键' && theme !== '简洁' && !title) {
    const txt = String(spec.explain).slice(0, 70)
    const lines: string[] = []
    for (let i = 0; i < txt.length; i += 22) lines.push(txt.slice(i, i + 22))
    const bx = 760, by = 40, bw = 22 * 17 + 30, bh = lines.length * 24 + 22
    s += `<g class="route-mark" style="animation-delay:1.2s"><rect x="${bx}" y="${by}" width="${bw}" height="${bh}" rx="8" fill="#ffffff" fill-opacity=".92" stroke="#c9d3cf"/>`
    lines.forEach((l, i) => { s += `<text x="${bx + 15}" y="${by + 28 + i * 24}" font-size="17" fill="#334a52">${esc(l)}</text>` })
    s += '</g>'
  }
  if (o.showIssues) s += issuesLayer(res.issues || [], byId)
  return s
}

export function issuesLayer(issues: RouteIssue[], byId: Record<string, RoadNode>) {
  let s = ''
  for (const it of issues) {
    let x = 0, y = 0
    if (it.from && it.to && byId[it.from] && byId[it.to]) {
      x = (byId[it.from].x + byId[it.to].x) / 2
      y = (byId[it.from].y + byId[it.to].y) / 2
    } else if (it.node && byId[it.node]) {
      x = byId[it.node].x; y = byId[it.node].y
    } else continue
    s += `<circle cx="${x}" cy="${y}" r="42" fill="none" stroke="#e02424" stroke-width="5" stroke-dasharray="10 7"/>`
    s += label(x, y - 50, it.label, 22, '#d11f1f')
  }
  return s
}

export function drawnMapSvg(res: DrawResult | null, o: RouteRenderOpts & { cls?: string }) {
  const spec = res?.spec
  const theme = spec?.theme || '默认'
  const scene = renderCampus({
    view: 'plan',
    labels: spec?.labels === '只留关键' ? 'none' : 'all',
    dim: theme === '简洁' || theme === '黑白',
    trees: theme !== '简洁' && theme !== '黑白',
    legend: theme !== '简洁' && theme !== '夜间' && spec?.labels !== '只留关键',
    corridor: theme !== '简洁',
  })
  // 底图换风格：只在底图上盖一层颜色 / 去色，路线层保持原色
  const [x, y, w, h] = scene.vb
  let body = scene.body
  if (theme === '黑白') body = `<defs><filter id="bw"><feColorMatrix type="saturate" values="0"/></filter></defs><g filter="url(#bw)">${body}</g>`
  else if (theme === '夜间') body += `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="#0b1726" fill-opacity=".62"/>`
  else if (theme === '卡通') body += `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="#ffe08a" fill-opacity=".22"/>`
  return wrapSvg({ ...scene, body }, res ? routeLayer(res, o) : '', o.cls || '')
}

/** Rasterize an SVG string to a PNG data URL (width px). */
export function rasterize(svg: string, width = 1536): Promise<string> {
  return new Promise((resolve, reject) => {
    const vb = PLAN_VB
    const height = Math.round((width * vb[3]) / vb[2])
    // inline the font-family and strip animation classes so the PNG shows the final state
    const clean = svg.replace(/class="route-draw"/g, '').replace(/class="route-mark"/g, '').replace(/style="animation-delay:[^"]*"/g, '')
    const withSize = clean.replace('<svg ', `<svg width="${width}" height="${height}" style="font-family:${SVG_FONT}" `)
    const url = URL.createObjectURL(new Blob([withSize], { type: 'image/svg+xml;charset=utf-8' }))
    const img = new Image()
    img.onload = () => {
      const c = document.createElement('canvas')
      c.width = width
      c.height = height
      const g = c.getContext('2d')!
      g.fillStyle = '#fffdf8'
      g.fillRect(0, 0, width, height)
      g.drawImage(img, 0, 0, width, height)
      URL.revokeObjectURL(url)
      // JPEG keeps the "edited picture" around 200–300 KB (a PNG of this size is >1 MB)
      resolve(c.toDataURL('image/jpeg', 0.85))
    }
    img.onerror = () => { URL.revokeObjectURL(url); reject(new Error('rasterize failed')) }
    img.src = url
  })
}
