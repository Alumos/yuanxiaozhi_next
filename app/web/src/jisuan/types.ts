import { reactive } from 'vue'
import type { Cat } from '../common/cover'

export interface BookView { id: string; title: string; author: string; category: string; cat: number; tags: string[]; all_tags: string[]; form: string; length: string; hook: string; summary: string }
export interface Slot { book_id: string; purpose: string; note: string; pair?: string }
export interface Round { n: number; shown: Slot[]; chosen: string; ms: number; opened: string[] }
export interface TagDelta { tag: string; v: number; why: string }
export interface RoundDelta { round: number; chosen: string; conf: number; ms: number; deltas: TagDelta[]; obs: string[]; bars: Record<string, number> }
export interface TagW { tag: string; v: number }
export interface Profile { l: number; top: TagW[]; neg: TagW[]; form: string; len_word: string; all: TagW[] }
export interface Candidate { id: string; score: number; interest: number; hit: string[] }
export interface Trace {
  kind: 'first' | 'again'; rounds: Round[]; per_round?: RoundDelta[]; profile: Profile; scores: number[]
  funnel: { n: number; text: string }[]; candidates: Candidate[]; ops: number; compute_ms: number; books: Record<string, BookView>
  changes?: TagDelta[]; struck?: string[]; liked?: string[]; text?: string; old_picks?: string[]
  llm?: { thoughts: { kind: string; text: string; ms?: number }[]; draft?: string; ms: number; fallback: boolean; source: string }
}
export interface Pick { book_id: string; interest: number; reason: string; evidence: number[] }
export interface Result { persona: string; top_tags: string[]; picks: Pick[]; note?: string }

/** 一次分析的数据流：事件按到达顺序写进来，动画调度器按节奏读取。 */
export function newFeed() {
  return reactive({
    trace: null as Trace | null,
    thoughts: [] as { kind: string; text: string }[],
    persona: null as { name: string; tags: string[] } | null,
    picks: [] as Pick[],
    final: null as Result | null,
    done: false,
    error: '',
    t0: Date.now(),
  })
}
export type Feed = ReturnType<typeof newFeed>

export const META = reactive({ cats: [] as Cat[], spines: [] as number[], books: 301, tags: 32, loaded: false })
export const catOf = (b?: BookView) => (b ? META.cats[b.cat] : undefined)

export const KIND_ICON: Record<string, string> = { 证据: 'search', 推断: 'bulb', 排除: 'strike', 发现: 'star', 结论: 'check' }
