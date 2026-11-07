<script setup lang="ts">
import { ref } from 'vue'
import { post } from '../common/api'
import YxzAvatar from '../common/components/YxzAvatar.vue'
const pin = ref('')
const err = ref('')
async function go() {
  try { await post('/api/common/login', { pin: pin.value }); location.reload() } catch (e) { err.value = (e as Error).message }
}
</script>
<template>
  <main class="lg">
    <div class="card box">
      <YxzAvatar :size="80" />
      <h1>教师口令</h1>
      <input v-model="pin" type="password" inputmode="numeric" autofocus placeholder="输入口令" @keyup.enter="go" />
      <p v-if="err" class="err">{{ err }}</p>
      <button class="btn big" @click="go">进入</button>
    </div>
  </main>
</template>
<style scoped>
.lg { min-height: 100vh; display: flex; align-items: center; justify-content: center; }
.box { padding: 40px; display: flex; flex-direction: column; gap: 18px; align-items: center; width: 420px; }
h1 { font-size: 30px; }
input { width: 100%; min-height: 64px; font-size: 30px; text-align: center; border-radius: 14px; border: 2px solid var(--line); letter-spacing: 6px; }
.err { color: var(--red); }
</style>
