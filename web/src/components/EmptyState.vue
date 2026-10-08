<template>
  <div class="empty" :style="{ height }">
    <svg width="40" height="40" viewBox="0 0 40 40" fill="none" aria-hidden="true">
      <rect x="6" y="8" width="28" height="24" rx="4" stroke="#D1D5DB" stroke-width="1.5" />
      <path d="M12 26l6-7 5 4 6-8" stroke="#9CA3AF" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
    <div class="t">{{ text }}</div>
    <div v-if="hint" class="h">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ text: string; hint?: string; height?: string }>(), { height: '220px' })
</script>

<style scoped>
.empty { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; color: var(--text-2); }
.t { font-size: 13px; font-weight: 500; }
.h { font-size: 12px; color: var(--text-3); }
</style>
