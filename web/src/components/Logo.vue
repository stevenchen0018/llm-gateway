<template>
  <svg :width="size" :height="size" viewBox="0 0 28 28" fill="none" aria-hidden="true">
    <rect width="28" height="28" rx="7" fill="#2563EB" />
    <path d="M7 9.5h6.5a3 3 0 0 1 3 3v0a3 3 0 0 1-3 3H10" stroke="#fff" stroke-width="2" stroke-linecap="round" />
    <path d="M14.5 18.5H21" stroke="#C4B5FD" stroke-width="2" stroke-linecap="round" />
    <circle cx="21" cy="18.5" r="2" fill="#7C3AED" stroke="#fff" stroke-width="1.5" />
  </svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 26 })
</script>
