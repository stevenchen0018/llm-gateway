<template>
  <div class="frow">
    <span class="lab">{{ label }}</span>
    <div class="chips">
      <button v-for="o in options" :key="String(o.value)" type="button" class="chip" :class="{ on: modelValue === o.value }" @click="$emit('update:modelValue', o.value)">{{ o.label }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ label: string; modelValue: string | number; options: { value: string | number; label: string }[] }>()
defineEmits<{ 'update:modelValue': [v: string | number] }>()
</script>

<style scoped>
.frow { display: flex; align-items: flex-start; gap: 12px; padding: 5px 0; }
.lab { flex: none; width: 64px; padding-top: 4px; font-size: 12px; color: var(--text-2); }
.chips { display: flex; flex-wrap: wrap; gap: 4px 6px; }
.chip { height: 26px; padding: 0 10px; border: 1px solid transparent; border-radius: 6px; background: transparent; font: inherit; font-size: 13px; color: var(--text); cursor: pointer; transition: background .12s; }
.chip:hover { background: #F3F4F6; }
.chip.on { background: var(--primary-soft); color: var(--primary); font-weight: 500; }
</style>
