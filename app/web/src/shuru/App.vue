<script setup lang="ts">
import YxzHeader from '../common/components/YxzHeader.vue'
import YxzAvatar from '../common/components/YxzAvatar.vue'
</script>
<template>
  <YxzHeader title="元小智 · 输入" link="input" />
  <main class="p"><YxzAvatar state="listen" :size="120" /><h1>输入环节正在准备中</h1><p class="muted">请先完成“计算”和“输出”环节</p><a class="btn" href="/">回到入口</a></main>
</template>
<style scoped>
.p { display: flex; flex-direction: column; align-items: center; gap: 18px; padding-top: 120px; text-align: center; }
h1 { font-size: 34px; }
</style>
