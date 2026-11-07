export interface RoadNode { id: string; name: string; kind: string; x: number; y: number; floor?: number; building?: string; rate?: number }
export interface Marker { type: string; at: string; text?: string; color?: string }
export interface DrawSpec {
  path: string[]
  line: { color: string; width: string; style?: string; dash?: boolean; arrow: boolean; arrow_shape?: string }
  markers: Marker[]; numbers?: boolean; title?: string; labels?: string; theme: string; audience?: string; explain?: string
}
export interface RouteIssue { kind: string; label: string; from?: string; to?: string; node?: string; warn?: boolean }
export interface DrawResult { spec: DrawSpec; path: string[]; issues?: RouteIssue[] }
export interface Route { path: string[]; stair: string; dist_m: number; walk_s: number; guess?: boolean; direct?: boolean }
export interface GroupPlan { group: number; class: string; known: boolean; info: { Stairs: boolean; Roads: boolean; Crowd: boolean; Goal: boolean }; cands: Route[]; chosen: number; total_s: number; color: string }
export interface StairLoad { people: number; queue_s: number; color: string; groups: number[] }
export interface PlanRound { assign: Record<number, string>; times: Record<number, number>; loads: Record<string, StairLoad>; moved?: { group: number; from: string; to: string; before: number; after: number; why: string } }
export interface PlanTrace { groups: GroupPlan[]; rounds: PlanRound[]; first_red: number; red_stair: string; ops: number }
