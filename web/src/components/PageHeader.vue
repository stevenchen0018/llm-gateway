<template>
  <div class="ph">
    <div class="ph-main">
      <h1>{{ title }}</h1>
      <p v-if="desc">{{ desc }}</p>
    </div>
    <div v-if="$slots.default" class="ph-actions"><slot /></div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string; desc?: string }>()
</script>

<style scoped>
.ph { display: flex; justify-content: space-between; align-items: flex-end; gap: 16px 24px; margin-bottom: 18px; flex-wrap: wrap; }
.ph-main { min-width: 0; flex: 1 1 420px; }
h1 { font-size: 20px; line-height: 28px; font-weight: 650; letter-spacing: -.01em; }
p { margin: 4px 0 0; color: var(--text-2); font-size: 13px; line-height: 1.6; max-width: 920px; }
.ph-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
</style>
