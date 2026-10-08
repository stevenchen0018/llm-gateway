<template>
  <span class="logo" :style="{ width: size + 'px', height: size + 'px', background: tint, color: st.color, fontSize: size * 0.42 + 'px' }">{{ st.text }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { vendorStyle } from '../constants'

const props = withDefaults(defineProps<{ code: string; size?: number }>(), { size: 32 })
const st = computed(() => vendorStyle(props.code))
// 12% tint of the vendor colour
const tint = computed(() => st.value.color + '1F')
</script>

<style scoped>
.logo { display: inline-flex; align-items: center; justify-content: center; flex: none; border-radius: 8px; font-weight: 700; letter-spacing: -.02em; line-height: 1; }
</style>
