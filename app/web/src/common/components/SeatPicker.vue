<script setup lang="ts">
/* 选座位：5×5 宫格，已占用的变灰。换平板时先确认。 */
import { onMounted, ref } from 'vue'
import { ApiError, get, post, setToken } from '../api'
import YxzAvatar from './YxzAvatar.vue'
const emit = defineEmits<{ (e: 'done', seat: number, group: number): void }>()
const props = defineProps<{ hint?: string }>()
const seats = ref<{ seat: number; group: number }[]>([])
const taken = ref<Set<number>>(new Set())
const confirm = ref(0)
const busy = ref(false)
const err = ref('')
onMounted(async () => {
  const d = await get('/api/common/seats')
  seats.value = d.seats
  taken.value = new Set(d.taken)
})
async function pick(n: number, force = false) {
  if (busy.value) return
  if (taken.value.has(n) && !force) { confirm.value = n; return }
  busy.value = true
  try {
    const d = await post('/api/common/join', { seat: n, force })
    setToken(d.token)
    emit('done', d.seat, d.group)
  } catch (e) {
    if (e instanceof ApiError && e.code === 'seat_taken') confirm.value = n
    else err.value = (e as Error).message
  } finally { busy.value = false }
}
</script>
<template>
  <div class="sp">
    <div class="head"><YxzAvatar state="idle" :size="64" /><div><h1>你们坐几号座位？</h1><p class="muted">{{ props.hint || '选平板背面贴的号码，不用写名字' }}</p></div></div>
    <div class="grid">
      <button v-for="s in seats" :key="s.seat" class="cell num" :class="{ taken: taken.has(s.seat) }" @click="pick(s.seat)">
        {{ s.seat }}<small>第 {{ s.group }} 组</small>
      </button>
    </div>
    <p v-if="err" class="err">{{ err }}</p>
    <div v-if="confirm" class="mask" @click.self="confirm = 0">
      <div class="dlg card">
        <p>{{ confirm }} 号座位已经有平板了，<br />要换到这台吗？</p>
        <div class="row"><button class="btn ghost" @click="confirm = 0">不换</button><button class="btn" @click="pick(confirm, true); confirm = 0">换到这台</button></div>
      </div>
    </div>
  </div>
</template>
<style scoped>
.sp { max-width: 760px; margin: 0 auto; padding: 28px 20px; }
.head { display: flex; gap: 16px; align-items: center; margin-bottom: 22px; }
h1 { font-size: var(--fs-title); }
.grid { display: grid; grid-template-columns: repeat(5, 1fr); gap: 14px; }
.cell { aspect-ratio: 1.25; border-radius: 18px; background: #fff; box-shadow: var(--shadow); font-size: 38px; font-weight: 800; display: flex; flex-direction: column; align-items: center; justify-content: center; line-height: 1.1; color: var(--ink); transition: transform var(--t-light); }
.cell small { font-size: 15px; font-weight: 600; color: var(--muted); margin-top: 4px; }
.cell:active { transform: scale(.94); background: var(--accent-soft); }
.cell.taken { background: #ece8e0; color: #9aa4a6; box-shadow: none; }
.err { color: var(--red); margin-top: 14px; }
.mask { position: fixed; inset: 0; background: rgba(20, 28, 32, .45); display: flex; align-items: center; justify-content: center; z-index: 50; }
.dlg { padding: 28px; max-width: 420px; text-align: center; font-size: 22px; }
.row { display: flex; gap: 14px; justify-content: center; margin-top: 22px; }
</style>
