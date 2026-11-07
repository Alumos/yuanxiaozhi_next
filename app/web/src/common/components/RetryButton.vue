<script setup lang="ts">
/* “再试一次”：结果区右下角；先确认；显示剩余次数；用完变灰。 */
import { ref } from 'vue'
import { ICON } from '../icons'
const props = defineProps<{ left: number; confirmText: string; busy?: boolean }>()
const emit = defineEmits<{ (e: 'go'): void }>()
const ask = ref(false)
</script>
<template>
  <span class="rb">
    <button class="btn ghost" :disabled="props.left <= 0 || props.busy" @click="ask = true"><span v-html="ICON.retry" />再试一次</button>
    <small class="muted">{{ props.left > 0 ? `还能试 ${props.left} 次` : '去看看大屏吧' }}</small>
    <div v-if="ask" class="pop card">
      <p>{{ props.confirmText }}</p>
      <div class="row"><button class="btn sm ghost" @click="ask = false">先不了</button><button class="btn sm" @click="ask = false; emit('go')">好，再试一次</button></div>
    </div>
  </span>
</template>
<style scoped>
.rb { position: relative; display: inline-flex; flex-direction: column; align-items: center; gap: 2px; }
small { font-size: 14px; }
.pop { position: absolute; bottom: calc(100% + 12px); right: 0; width: 320px; padding: 18px; z-index: 30; font-size: 19px; }
.pop::after { content: ""; position: absolute; bottom: -8px; right: 40px; border: 8px solid transparent; border-top-color: #fff; border-bottom: 0; }
.row { display: flex; gap: 10px; justify-content: flex-end; margin-top: 14px; }
</style>
