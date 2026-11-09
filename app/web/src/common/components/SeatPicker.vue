<script setup lang="ts">
/* 平板编号：不用学生选，按平板进入课堂的先后自动分配（已经有号的平板保持原号）。 */
import { onMounted, ref } from 'vue'
import { post, setToken } from '../api'
import YxzAvatar from './YxzAvatar.vue'
const emit = defineEmits<{ (e: 'done', seat: number, group: number): void }>()
defineProps<{ hint?: string }>()
const err = ref('')
const busy = ref(false)
async function join() {
  if (busy.value) return
  busy.value = true; err.value = ''
  try {
    const d = await post('/api/common/join', { seat: 0 })
    setToken(d.token)
    emit('done', d.seat, d.group)
  } catch (e) { err.value = (e as Error).message } finally { busy.value = false }
}
onMounted(join)
</script>
<template>
  <div class="sp">
    <YxzAvatar :state="err ? 'idle' : 'think'" :size="72" />
    <h1 v-if="!err">正在进入课堂……</h1>
    <template v-else>
      <h1>{{ err }}</h1>
      <button class="btn big" :disabled="busy" @click="join">再试一次</button>
    </template>
  </div>
</template>
<style scoped>
.sp { min-height: 60vh; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; padding: 20px; text-align: center; }
h1 { font-size: var(--fs-title); }
</style>
