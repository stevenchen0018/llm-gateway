<template>
  <div>
    <PageHeader title="调用日志" desc="完整记录网关每次调用的来源、路由结果、Token、成本与耗时，便于问题追踪与审计">
      <TransferBar name="call_logs" :filters="{ from: fmtDateTime((trRef?.current() ?? range).from), to: fmtDateTime((trRef?.current() ?? range).to), key_id: keyId, model_id: modelId, status }" @imported="load" />
    </PageHeader>

    <Panel class="filters">
      <div class="toolbar">
        <TimeRange ref="trRef" v-model="range" @change="reset" />
        <KeySelect v-model="keyId" placeholder="全部 Key" width="200px" @change="reset" />
        <el-select v-model="modelId" clearable filterable placeholder="全部模型" style="width: 200px" @change="reset"><el-option v-for="m in state.models" :key="m.id" :label="modelLabel(m.id)" :value="m.id" /></el-select>
        <el-select v-model="status" clearable placeholder="全部状态" style="width: 120px" @change="reset"><el-option label="成功" value="success" /><el-option label="失败" value="failed" /></el-select>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </Panel>

    <Panel title="调用记录" :count="total" flush>
      <el-table :data="items" v-loading="loading" empty-text="所选范围内暂无调用日志">
        <el-table-column label="时间" width="160"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="Request ID" width="110"><template #default="{ row }"><el-tooltip :content="row.request_id" placement="top"><span class="mono">{{ row.request_id.slice(0, 8) }}…</span></el-tooltip></template></el-table-column>
        <el-table-column label="API Key" min-width="130" show-overflow-tooltip><template #default="{ row }">{{ keyName(row.key_id) }}</template></el-table-column>
        <el-table-column label="命中模型" min-width="190">
          <template #default="{ row }"><div class="cell-title">{{ modelName(row.model_id) }}</div><div class="cell-sub">{{ providerName(row.provider_id) }}</div></template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }"><StatusBadge :text="row.status === 'success' ? '成功' : '失败'" :tone="row.status === 'success' ? 'success' : 'danger'" /><div v-if="row.error_code" class="cell-sub mono">{{ row.error_code }}</div></template>
        </el-table-column>
        <el-table-column label="耗时" width="80" align="right"><template #default="{ row }"><span class="num">{{ fmtDuration(row.latency_ms) }}</span></template></el-table-column>
        <el-table-column label="输入 / 输出" width="110" align="right"><template #default="{ row }"><span class="num">{{ fmtNum(row.prompt_tokens) }} / {{ fmtNum(row.completion_tokens) }}</span></template></el-table-column>
        <el-table-column label="成本" width="100" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(row.cost) }}</span></template></el-table-column>
        <el-table-column label="来源 IP" width="120"><template #default="{ row }"><span class="mono">{{ row.source_ip }}</span></template></el-table-column>
        <el-table-column v-if="canViewPrompts" label="" width="70" align="right"><template #default="{ row }"><el-button link type="primary" @click="openDetail(row.request_id)">内容</el-button></template></el-table-column>
      </el-table>
      <TablePager v-model:page="page" v-model:page-size="pageSize" :total="total" />
    </Panel>
    <RequestLogDrawer v-model:visible="detailVisible" :request-id="detailRid" />
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import KeySelect from '../../components/KeySelect.vue'
import RequestLogDrawer from '../../components/RequestLogDrawer.vue'
import { useAuth } from '../../composables/useAuth'
import { onMounted, ref } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import TimeRange from '../../components/TimeRange.vue'
import { dashboard } from '../../api'
import { useLookups } from '../../composables/useLookups'
import { defaultRange, fmtDateTime, fmtDuration, fmtMoney, fmtNum, fmtTime } from '../../utils'

const { state, keyName, modelName, modelLabel, providerName } = useLookups()
const range = ref(defaultRange('24h'))
const trRef = ref<InstanceType<typeof TimeRange>>()
const keyId = ref<number>()
const modelId = ref<number>()
const status = ref('')
const { items, total, page, pageSize, loading, load, reset } = usePaged((p) => {
  const r = trRef.value?.current() ?? range.value
  return dashboard.logs({ ...p, from: fmtDateTime(r.from), to: fmtDateTime(r.to), key_id: keyId.value, model_id: modelId.value, status: status.value })
})
onMounted(load)

// prompt/response content lives in the request records (same request id)
const { canViewPrompts } = useAuth()
const detailVisible = ref(false)
const detailRid = ref('')
function openDetail(rid: string) { detailRid.value = rid; detailVisible.value = true }
</script>

<style scoped>
.filters { margin-bottom: 16px; }
</style>
