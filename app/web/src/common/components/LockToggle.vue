<script setup lang="ts">
/* 大屏顶栏标题旁的环节锁开关：开放后学生平板才能操作这个环节 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { get, post } from '../api'
const props = defineProps<{ mod: string }>()
const locked = ref<boolean | null>(null)
let t = 0
async function load() { try { const d = await get('/api/common/locks'); locked.value = !!d[props.mod] } catch { /* */ } }
async function toggle() {
  if (locked.value === null) return
  try { const d = await post('/api/common/lock', { mod: props.mod, locked: !locked.value }); locked.value = !!d[props.mod] } catch { /* */ }
}
onMounted(() => { load(); t = window.setInterval(load, 8000) })
onBeforeUnmount(() => clearInterval(t))
</script>
<template>
  <button v-if="locked !== null" class="lock" :class="{ open: !locked }" :title="locked ? '点击开放：学生平板可以开始操作' : '点击锁住：学生平板暂停操作'" @click="toggle">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2.5" /><path :d="locked ? 'M8 11V7a4 4 0 0 1 8 0v4' : 'M8 11V7a4 4 0 0 1 7.6-1.8'" /></svg>
    <span>{{ locked ? '开放平板' : '已开放' }}</span>
  </button>
</template>
<style scoped>
.lock { flex: none; margin-left: 14px; display: inline-flex; align-items: center; gap: 8px; height: 50px; padding: 0 20px 0 14px; border-radius: 25px; font-size: 22px; font-weight: 800; color: #fff; background: #c2412f; box-shadow: 0 4px 12px rgba(0, 0, 0, .15); cursor: pointer; white-space: nowrap; transition: background-color var(--t-mid), opacity var(--t-mid); }
.lock svg { width: 26px; height: 26px; }
.lock.open { background: #149a58; color: #fff; box-shadow: none; }
</style>
