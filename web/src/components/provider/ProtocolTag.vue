<template>
  <span class="ptag" :class="key"><i />{{ PROTOCOL[key].text }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { PROTOCOL } from '../../constants'
import type { Provider } from '../../types'

// The wire protocol a supplier speaks (mock:// suppliers keep their configured
// protocol; the "mock" tag is only shown for runs that recorded it).
const props = defineProps<{ provider?: Pick<Provider, 'base_url' | 'protocol'> | null; protocol?: string }>()
const key = computed(() => {
  if (props.protocol) return PROTOCOL[props.protocol] ? props.protocol : 'openai'
  return props.provider?.protocol && PROTOCOL[props.provider.protocol] ? props.provider.protocol : 'openai'
})
</script>

<style scoped>
.ptag { display: inline-flex; align-items: center; gap: 5px; height: 20px; padding: 0 7px; border-radius: 4px; font-size: 11.5px; font-weight: 500; border: 1px solid var(--border); color: var(--text-2); background: #fff; white-space: nowrap; }
.ptag i { width: 6px; height: 6px; border-radius: 2px; background: #9CA3AF; }
.openai i { background: #10A37F; }
.anthropic i { background: #D97706; }
.gemini i { background: #2563EB; }
</style>
