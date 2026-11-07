<script setup lang="ts">
/* 规划器慢放的地图：淡化底图 + 道路网 + 候选路线 + 楼梯口按人数变色。 */
import { computed } from 'vue'
import { renderCampus, PLAN_VB } from './map/campus'
import type { PlanRound, PlanTrace, RoadNode } from './types'
const p = defineProps<{ nodes: RoadNode[]; edges: { from: string; to: string }[]; trace: PlanTrace | null; step: number; round: number; flash: number }>()
const base = renderCampus({ dim: true, legend: false, trees: false }).body
const by = computed(() => Object.fromEntries(p.nodes.map((n) => [n.id, n])))
const GC = ['#e0442f', '#2a73c9', '#149a58', '#e89414', '#8a55d6', '#11a5b5']
const pts = (path: string[]) => path.map((id) => by.value[id]).filter(Boolean).map((n) => `${n.x},${n.y}`).join(' ')
const cur = computed<PlanRound | null>(() => p.trace?.rounds[Math.min(p.round, (p.trace?.rounds.length || 1) - 1)] || null)
const COL: Record<string, string> = { green: '#2bc06b', yellow: '#f2b705', red: '#e0442f' }
const routeOf = (g: number) => {
  const gp = p.trace?.groups.find((x) => x.group === g)
  if (!gp || !gp.cands.length || !cur.value) return null
  const st = cur.value.assign[g]
  const r = gp.cands.find((c) => c.stair === st) || gp.cands.find((c) => c.guess) || gp.cands[gp.chosen]
  return r
}
const dots = (n: number) => Array.from({ length: Math.min(30, Math.round(n / 4)) }, (_, i) => ({ a: (i * 137.5 * Math.PI) / 180, r: 16 + (i % 5) * 6 }))
</script>
<template>
  <svg :viewBox="PLAN_VB.join(' ')" class="pm">
    <g :style="{ opacity: step >= 1 ? 0.35 : 1, transition: 'opacity .8s' }" v-html="base" />
    <g v-if="step >= 1" class="net">
      <line v-for="(e, i) in edges" :key="i" :x1="by[e.from]?.x" :y1="by[e.from]?.y" :x2="by[e.to]?.x" :y2="by[e.to]?.y" />
      <circle v-for="n in nodes.filter((n) => n.kind !== 'guess')" :key="n.id" :cx="n.x" :cy="n.y" :r="n.kind === 'stair' ? 8 : 4" :class="n.kind" />
    </g>
    <!-- ② 候选路线依次闪过 -->
    <g v-if="step === 2 && trace">
      <template v-for="gp in trace.groups" :key="gp.group">
        <template v-if="gp.group === flash">
          <polyline v-for="(c, k) in gp.cands" :key="k" :points="pts(c.path)" class="cand" :stroke="GC[(gp.group - 1) % 6]" :stroke-dasharray="k ? '14 10' : ''" />
          <text v-for="(c, k) in gp.cands" :key="'t' + k" :x="by[c.stair]?.x" :y="(by[c.stair]?.y || 0) - 24 - k * 34" class="dist">{{ c.dist_m }} 米</text>
        </template>
      </template>
    </g>
    <!-- ③–⑥ 全班一起：楼梯口变色 -->
    <g v-if="step >= 3 && trace && cur">
      <polyline v-for="gp in trace.groups" :key="gp.group" :points="pts(routeOf(gp.group)?.path || [])" class="route" :stroke="GC[(gp.group - 1) % 6]" :stroke-width="step >= 5 ? 9 : 6" :opacity="step >= 5 ? 0.95 : 0.6" />
      <g v-for="(l, st) in cur.loads" :key="st" :transform="`translate(${by[st]?.x},${by[st]?.y})`">
        <circle :r="26 + Math.min(30, l.people / 6)" :fill="COL[l.color]" opacity=".35" class="halo" :class="l.color" />
        <circle v-for="(d, i) in dots(l.people)" :key="i" :cx="Math.cos(d.a) * d.r" :cy="Math.sin(d.a) * d.r" r="3.2" fill="#1f2d33" class="crowd" :style="{ animationDelay: i * 40 + 'ms' }" />
        <circle r="13" :fill="COL[l.color]" stroke="#fff" stroke-width="3" />
        <text y="-40" class="ld">{{ l.people }} 人 · 排队 {{ l.queue_s }} 秒</text>
      </g>
    </g>
  </svg>
</template>
<style scoped>
.pm { width: 100%; height: 100%; display: block; }
.net line { stroke: #3fd7e8; stroke-width: 3; opacity: .75; }
.net circle { fill: #3fd7e8; } .net circle.stair { fill: #9b7bff; } .net circle.assembly { fill: #ff8a4c; r: 12; }
.cand { fill: none; stroke-width: 9; stroke-linejoin: round; stroke-linecap: round; animation: dash 1s ease both; }
@keyframes dash { from { opacity: 0; } }
.dist { font-size: 30px; font-weight: 800; fill: #1f2d33; paint-order: stroke; stroke: #fff; stroke-width: 6px; text-anchor: middle; }
.route { fill: none; stroke-linejoin: round; stroke-linecap: round; transition: stroke-width .5s; }
.halo { transition: fill .6s, r .6s; }
.halo.red { animation: pulse 1s infinite; }
@keyframes pulse { 50% { opacity: .7; } }
.crowd { animation: jig 1.2s ease-in-out infinite alternate; }
@keyframes jig { to { transform: translate(2px, -2px); } }
.ld { font-size: 26px; font-weight: 800; text-anchor: middle; fill: #1f2d33; paint-order: stroke; stroke: #fff; stroke-width: 6px; }
</style>
