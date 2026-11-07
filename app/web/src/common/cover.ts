/* 书封面：不用真实图片，按大类生成底色和花纹（CSS 背景，零网络流量）。 */
export interface Cat { name: string; color: string; motif: string; icon: string }
const W = 'rgba(255,255,255,.18)'
const MOTIF: Record<string, string> = {
  斜条纹: `repeating-linear-gradient(45deg, ${W} 0 8px, transparent 8px 18px)`,
  星点: `radial-gradient(${W} 2px, transparent 2.5px) 0 0/18px 18px, radial-gradient(${W} 1.2px, transparent 1.6px) 9px 9px/18px 18px`,
  网格: `linear-gradient(${W} 1px, transparent 1px) 0 0/16px 16px, linear-gradient(90deg, ${W} 1px, transparent 1px) 0 0/16px 16px`,
  菱格: `repeating-linear-gradient(45deg, ${W} 0 1.5px, transparent 1.5px 14px), repeating-linear-gradient(-45deg, ${W} 0 1.5px, transparent 1.5px 14px)`,
  叶片: `radial-gradient(ellipse 6px 10px at 50% 50%, ${W} 98%, transparent) 0 0/22px 22px`,
  横格线: `repeating-linear-gradient(0deg, ${W} 0 1.5px, transparent 1.5px 14px)`,
  圆点: `radial-gradient(${W} 4px, transparent 4.5px) 0 0/20px 20px`,
  回纹: `linear-gradient(90deg, ${W} 2px, transparent 2px) 0 0/12px 24px, linear-gradient(${W} 2px, transparent 2px) 0 0/24px 12px`,
  六边形: `radial-gradient(circle at 50% 0, transparent 7px, ${W} 7.5px, transparent 9px) 0 0/18px 16px`,
  波点: `radial-gradient(${W} 5px, transparent 5.5px) 0 0/24px 24px, radial-gradient(${W} 3px, transparent 3.5px) 12px 12px/24px 24px`,
  竖条纹: `repeating-linear-gradient(90deg, ${W} 0 6px, transparent 6px 16px)`,
  水波: `radial-gradient(circle at 50% 100%, transparent 8px, ${W} 9px, transparent 10px) 0 0/22px 12px`,
}
function hash(s: string) { let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0; return Math.abs(h) }
export function shade(hex: string, k: number) {
  const n = parseInt(hex.slice(1), 16)
  const f = (c: number) => Math.max(0, Math.min(255, Math.round(k > 0 ? c + (255 - c) * k : c * (1 + k))))
  return '#' + [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => f(c).toString(16).padStart(2, '0')).join('')
}
export function coverStyle(c: Cat | undefined, id: string) {
  const col = c ? c.color : '#7d8c90'
  const k = ((hash(id) % 7) - 3) * 0.06
  const base = shade(col, k)
  return { background: `${MOTIF[c?.motif || ''] || ''}${MOTIF[c?.motif || ''] ? ',' : ''} linear-gradient(160deg, ${shade(base, 0.12)}, ${shade(base, -0.18)})`, '--c': base } as Record<string, string>
}
