<script setup lang="ts">
import { computed } from 'vue'
import { coverStyle, type Cat } from '../cover'
import { icon } from '../icons'
const p = defineProps<{ id: string; title: string; cat?: Cat; w?: number }>()
const st = computed(() => ({ ...coverStyle(p.cat, p.id), width: (p.w || 120) + 'px', height: (p.w || 120) * 1.36 + 'px', fontSize: (p.w || 120) * 0.17 + 'px' }))
const short = computed(() => (p.title.length > 9 ? p.title.slice(0, 9) + '…' : p.title))
</script>
<template>
  <div class="cv" :style="st">
    <span class="spine" />
    <b class="t">{{ short }}</b>
    <span class="ic" v-html="icon(cat?.icon || '')" />
  </div>
</template>
<style scoped>
.cv { position: relative; border-radius: 4px 10px 10px 4px; overflow: hidden; color: #fff; box-shadow: 0 3px 8px rgba(0, 0, 0, .18); flex: none; }
.spine { position: absolute; left: 0; top: 0; bottom: 0; width: 9%; background: rgba(0, 0, 0, .18); }
.t { position: absolute; right: 12%; top: 9%; writing-mode: vertical-rl; letter-spacing: .08em; font-weight: 800; line-height: 1.1; max-height: 70%; text-shadow: 0 1px 2px rgba(0, 0, 0, .3); background: rgba(255, 255, 255, .16); padding: 6px 3px; border-radius: 4px; }
.ic { position: absolute; left: 16%; bottom: 8%; width: 30%; height: 30%; opacity: .9; }
.ic :deep(svg) { width: 100%; height: 100%; }
</style>
