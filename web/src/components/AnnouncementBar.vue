<template>
  <div v-if="items.length" class="bar" :class="current.level" :title="current.content">
    <el-icon :size="14"><Bell v-if="current.level === 'info'" /><WarningFilled v-else /></el-icon>
    <transition name="slide" mode="out-in"><span :key="idx" class="txt">{{ current.content }}</span></transition>
    <span v-if="items.length > 1" class="cnt num">{{ idx + 1 }}/{{ items.length }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Bell, WarningFilled } from '@element-plus/icons-vue'
import { platform } from '../api'
import type { Announcement } from '../types'

const items = ref<Announcement[]>([])
const idx = ref(0)
const current = computed(() => items.value[idx.value % Math.max(items.value.length, 1)] ?? { content: '', level: 'info' })

let rotate: number, refresh: number
async function load() { try { items.value = await platform.announcements(true); idx.value = 0 } catch { /* the bar is decorative */ } }
onMounted(() => {
  load()
  rotate = window.setInterval(() => { if (items.value.length > 1) idx.value = (idx.value + 1) % items.value.length }, 8000)
  refresh = window.setInterval(load, 5 * 60_000)
})
onBeforeUnmount(() => { clearInterval(rotate); clearInterval(refresh) })
</script>

<style scoped>
.bar { display: flex; align-items: center; gap: 8px; min-width: 0; max-width: 640px; height: 28px; padding: 0 12px; border-radius: 999px; font-size: 12.5px; }
.bar.info { background: var(--primary-soft); color: #1E40AF; }
.bar.warning { background: var(--warning-soft); color: #B45309; }
.txt { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cnt { flex: none; font-size: 11px; opacity: .7; }
.slide-enter-active, .slide-leave-active { transition: opacity .25s, transform .25s; }
.slide-enter-from { opacity: 0; transform: translateY(6px); }
.slide-leave-to { opacity: 0; transform: translateY(-6px); }
</style>
