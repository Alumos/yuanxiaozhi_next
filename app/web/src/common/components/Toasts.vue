<script setup lang="ts">
import { T } from '../toast'
</script>
<template>
  <div class="ts">
    <TransitionGroup name="fade"><div v-for="t in T.list" :key="t.id" class="t" :class="t.kind">{{ t.text }}</div></TransitionGroup>
  </div>
</template>
<style scoped>
.ts { position: fixed; left: 0; right: 0; top: 78px; display: flex; flex-direction: column; align-items: center; gap: 8px; z-index: 120; pointer-events: none; }
.t { background: rgba(31, 45, 51, .92); color: #fff; padding: 12px 22px; border-radius: 14px; font-size: 19px; font-weight: 600; max-width: 90vw; }
.t.warn { background: #c0432f; } .t.ok { background: #18864f; }
</style>
