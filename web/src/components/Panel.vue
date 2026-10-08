<template>
  <section class="panel">
    <header v-if="title || $slots.extra" class="panel-head">
      <div class="ph-l">
        <div class="ph-t">
          <h3>{{ title }}</h3>
          <span v-if="count !== undefined && count !== null" class="count num">{{ Number(count).toLocaleString() }}</span>
        </div>
        <span v-if="sub" class="sub">{{ sub }}</span>
      </div>
      <div v-if="$slots.extra" class="ph-r toolbar"><slot name="extra" /></div>
    </header>
    <!-- filters / search for the list below, on their own row -->
    <div v-if="$slots.toolbar" class="panel-toolbar toolbar"><slot name="toolbar" /></div>
    <div class="panel-body" :class="{ flush }"><slot /></div>
  </section>
</template>

<script setup lang="ts">
defineProps<{ title?: string; sub?: string; count?: number | null; flush?: boolean }>()
</script>

<style scoped>
.panel { background: var(--card); border: 1px solid var(--border); border-radius: var(--radius); min-width: 0; overflow: hidden; box-shadow: 0 1px 2px rgba(16, 24, 40, .03); }
.panel-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--border-soft); min-height: 52px; flex-wrap: wrap; }
.ph-l { min-width: 0; }
.ph-t { display: flex; align-items: center; gap: 8px; }
h3 { font-size: 14.5px; font-weight: 600; line-height: 22px; }
.count { display: inline-flex; align-items: center; height: 20px; padding: 0 8px; border-radius: 999px; background: #F1F3F6; color: var(--text-2); font-size: 12px; font-weight: 500; }
.sub { display: block; font-size: 12px; color: var(--text-2); line-height: 18px; margin-top: 1px; }
.ph-r { justify-content: flex-end; }
.panel-toolbar { padding: 10px 16px; background: #FAFBFC; border-bottom: 1px solid var(--border-soft); }
.panel-body { padding: 16px; }
.panel-body.flush { padding: 0; }
</style>
