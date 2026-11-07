/* 平板和大屏共用的渲染入口：底图 SVG 缓存一次；按说明书叠加路线层。 */
import { drawnMapSvg, rasterize } from './route'
import { renderCampus, wrapSvg } from './campus'
import type { DrawSpec, RoadNode, RouteIssue } from '../types'

let thumbURL = ''
/** 校园平面图缩略图（学生“发给元小智的那张图”）。 */
export function campusThumb(): string {
  if (!thumbURL) thumbURL = URL.createObjectURL(new Blob([wrapSvg(renderCampus({ legend: false, trees: false }))], { type: 'image/svg+xml' }))
  return thumbURL
}
export function routeSvg(spec: DrawSpec, nodes: RoadNode[], o: { animate?: boolean; issues?: RouteIssue[] } = {}) {
  return drawnMapSvg({ spec, path: spec.path, issues: o.issues }, { nodes, animate: o.animate, showIssues: !!o.issues, cls: 'map-svg' + (o.animate ? ' anim' : '') })
}
export async function routeJpeg(spec: DrawSpec, nodes: RoadNode[], width = 1536) {
  return rasterize(routeSvg(spec, nodes), width)
}
