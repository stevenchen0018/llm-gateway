<template>
  <span class="badge" :class="tone"><i />{{ text }}</span>
</template>

<script setup lang="ts">
export type Tone = 'success' | 'warning' | 'danger' | 'info' | 'primary' | 'accent'
defineProps<{ text: string; tone: Tone }>()
</script>

<style scoped>
.badge { display: inline-flex; align-items: center; gap: 6px; height: 22px; padding: 0 8px; border-radius: 999px; font-size: 12px; font-weight: 500; white-space: nowrap; }
i { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.success { color: #15803D; background: var(--success-soft); }
.success i { background: var(--success); }
.warning { color: #B45309; background: var(--warning-soft); }
.warning i { background: var(--warning); }
.danger { color: #B91C1C; background: var(--danger-soft); }
.danger i { background: var(--danger); }
.info { color: #4B5563; background: #F1F2F4; }
.info i { background: #9CA3AF; }
.primary { color: #1D4ED8; background: var(--primary-soft); }
.primary i { background: var(--primary); }
.accent { color: #6D28D9; background: var(--accent-soft); }
.accent i { background: var(--accent); }
</style>
