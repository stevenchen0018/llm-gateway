<template>
  <div v-if="total > 0" class="pager">
    <span class="info muted">第 {{ from }}–{{ to }} 条，共 {{ total.toLocaleString() }} 条</span>
    <el-pagination
      :current-page="page" :page-size="pageSize" :total="total" :page-sizes="PAGE_SIZES"
      layout="sizes, prev, pager, next, jumper" background :pager-count="7"
      @update:current-page="(v: number) => $emit('update:page', v)"
      @update:page-size="(v: number) => $emit('update:pageSize', v)"
    />
  </div>
</template>

<script setup lang="ts">
// TablePager: the one pagination bar used under every list table.
import { computed } from 'vue'
import { PAGE_SIZES } from '../composables/usePaged'

const props = defineProps<{ total: number; page: number; pageSize: number }>()
defineEmits<{ 'update:page': [v: number]; 'update:pageSize': [v: number] }>()
const from = computed(() => (props.page - 1) * props.pageSize + 1)
const to = computed(() => Math.min(props.page * props.pageSize, props.total))
</script>

<style scoped>
.pager { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; padding: 12px 16px; border-top: 1px solid var(--border-soft); }
.info { font-size: 12.5px; font-variant-numeric: tabular-nums; }
</style>
