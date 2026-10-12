<template>
  <el-table :data="results" size="small" :max-height="maxHeight" class="trt">
    <el-table-column label="检查项" min-width="200">
      <template #default="{ row }">
        <div class="cell-title"><span v-if="row.check_type === 'probe'">连通性 / 鉴权</span><span v-else class="mono">{{ row.model_key }}</span></div>
        <div class="cell-sub">{{ row.check_type === 'probe' ? '请求模型列表，不消耗 Token' : TEST_CHECK[row.check_type] ?? row.check_type }}<template v-if="row.check_type !== 'probe' && multiRound"> · 第 {{ row.round }} 次</template></div>
      </template>
    </el-table-column>
    <el-table-column label="结果" width="92"><template #default="{ row }"><StatusBadge :text="STATUS[row.status].text" :tone="STATUS[row.status].tone" /></template></el-table-column>
    <el-table-column label="耗时" width="96" align="right"><template #default="{ row }"><span class="num" :class="{ faint: row.status === 'skipped' }">{{ row.status === 'skipped' ? '—' : fmtDuration(row.latency_ms) }}</span></template></el-table-column>
    <el-table-column label="Token" width="90" align="right"><template #default="{ row }"><span class="num muted">{{ row.prompt_tokens || row.completion_tokens ? `${row.prompt_tokens} / ${row.completion_tokens}` : '—' }}</span></template></el-table-column>
    <el-table-column label="详情" min-width="300">
      <template #default="{ row }">
        <template v-if="row.status === 'failed'">
          <span class="etype">{{ TEST_ERROR[row.error_type] ?? row.error_type }}<template v-if="row.http_status"> · HTTP {{ row.http_status }}</template></span>
          <div class="emsg" :title="row.error_message">{{ row.error_message }}</div>
        </template>
        <span v-else-if="row.status === 'skipped'" class="faint">{{ row.error_message }}</span>
        <span v-else class="preview" :title="row.response_preview">{{ row.response_preview }}</span>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import StatusBadge from '../StatusBadge.vue'
import { TEST_CHECK, TEST_ERROR } from '../../constants'
import { fmtDuration } from '../../utils'
import type { ProviderTestResult } from '../../types'

const props = defineProps<{ results: ProviderTestResult[]; maxHeight?: string | number }>()
const multiRound = computed(() => props.results.some((r) => r.round > 1))
const STATUS: Record<string, { text: string; tone: 'success' | 'danger' | 'info' }> = {
  passed: { text: '通过', tone: 'success' },
  failed: { text: '失败', tone: 'danger' },
  skipped: { text: '跳过', tone: 'info' },
}
</script>

<style scoped>
.etype { color: var(--danger); font-weight: 500; font-size: 12.5px; }
.emsg, .preview { font-size: 12px; color: var(--text-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.preview { display: block; }
</style>
