<template>
  <div class="kpi">
    <div class="label">{{ label }}</div>
    <div class="value kpi-value" :class="{ bad: danger }">{{ value }}<small v-if="unit">{{ unit }}</small></div>
    <div class="foot">
      <span class="hint">{{ hint }}</span>
      <svg v-if="path" class="spark" viewBox="0 0 80 24" preserveAspectRatio="none" aria-hidden="true">
        <path :d="path" fill="none" :stroke="color" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
      </svg>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ label: string; value: string; unit?: string; hint?: string; series?: number[]; color?: string; danger?: boolean }>()
const color = computed(() => props.color || '#2563EB')

const path = computed(() => {
  const s = props.series
  if (!s || s.length < 2) return ''
  const min = Math.min(...s), max = Math.max(...s), span = max - min || 1
  return s.map((v, i) => `${i ? 'L' : 'M'}${((i / (s.length - 1)) * 80).toFixed(1)},${(22 - ((v - min) / span) * 20).toFixed(1)}`).join(' ')
})
</script>

<style scoped>
.kpi { background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); padding: 14px 16px 12px; min-width: 0; transition: border-color .15s; }
.kpi:hover { border-color: #D1D5DB; }
.label { font-size: 12px; color: var(--text-2); font-weight: 500; }
.value { display: flex; align-items: baseline; height: 32px; margin-top: 6px; font-size: 26px; line-height: 32px; font-weight: 600; letter-spacing: -.02em; }
.value small { font-size: 13px; font-weight: 500; color: var(--text-2); margin-left: 4px; letter-spacing: 0; }
.value.bad { color: var(--danger); }
.foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 6px; min-height: 24px; }
.hint { margin: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text-2); font-size: 12px; }
.spark { width: 72px; height: 24px; flex: none; }
</style>
