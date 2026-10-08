<template>
  <div>
    <PageHeader title="请求记录" desc="异步留存每次网关调用的提示词与响应内容，支持按 Key、模型、状态、内容检索，用于审计追溯与问题排查">
      <TransferBar name="request_logs" :filters="{ from: fmtDateTime((trRef?.current() ?? range).from), to: fmtDateTime((trRef?.current() ?? range).to), key_id: keyId, model_id: modelId, status, q, request_id: rid, filter_hit: hitOnly }" @imported="load" />
      <el-button :icon="Setting" @click="openSettings">记录设置</el-button>
    </PageHeader>

    <div class="strip" :class="{ off: !st?.request_log.enabled }">
      <el-icon><component :is="st?.request_log.enabled ? 'CircleCheck' : 'Warning'" /></el-icon>
      <template v-if="st?.request_log.enabled">
        记录中：{{ st.request_log.capture_body ? `保存请求 / 响应内容（单条 ≤ ${st.request_log.max_body_kb} KB）` : '仅记录元数据，不保存内容' }}
        · {{ st.request_log.mask_sensitive ? '按脱敏规则隐藏敏感信息' : '不脱敏' }} · 保留 {{ st.request_log.retention_days }} 天 · 异步批量写入，不影响调用耗时
      </template>
      <template v-else>请求记录已关闭，新的调用不会被留存。</template>
    </div>

    <Panel class="filters">
      <div class="toolbar">
        <TimeRange ref="trRef" v-model="range" @change="reset" />
        <KeySelect v-model="keyId" placeholder="全部 Key" width="200px" @change="reset" />
        <el-select v-model="modelId" clearable filterable placeholder="全部模型" style="width: 190px" @change="reset"><el-option v-for="m in state.models" :key="m.id" :label="modelLabel(m.id)" :value="m.id" /></el-select>
        <el-radio-group v-model="status" @change="reset">
          <el-radio-button value="">全部</el-radio-button><el-radio-button value="success">成功</el-radio-button>
          <el-radio-button value="failed">失败</el-radio-button><el-radio-button value="blocked">已拦截</el-radio-button>
        </el-radio-group>
      </div>
      <div class="toolbar" style="margin-top: 10px">
        <el-input v-model="q" :prefix-icon="Search" clearable placeholder="搜索提示词 / 请求内容" style="width: 260px" @input="debounced" />
        <el-input v-model="rid" clearable placeholder="Request ID" style="width: 280px" class="mono" @change="reset" />
        <el-checkbox v-model="hitOnly" @change="reset">仅看命中过滤规则</el-checkbox>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </Panel>

    <Panel title="请求记录" :count="total" flush>
      <el-table :data="items" v-loading="loading" empty-text="所选范围内暂无请求记录" row-class-name="clickable" @row-click="(r: RequestLog) => open(r.id)">
        <el-table-column label="时间" width="160"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="API Key" width="130" show-overflow-tooltip><template #default="{ row }">{{ keyName(row.key_id) }}</template></el-table-column>
        <el-table-column label="模型" width="160" show-overflow-tooltip>
          <template #default="{ row }"><div class="cell-title mono">{{ row.model || '—' }}</div><div class="cell-sub">{{ endpointText(row.endpoint) }}</div></template>
        </el-table-column>
        <el-table-column label="提示词" min-width="220">
          <template #default="{ row }"><div class="prompt" :class="{ faint: !row.prompt_preview }">{{ row.prompt_preview || '（未保存内容）' }}</div></template>
        </el-table-column>
        <el-table-column label="过滤命中" width="140">
          <template #default="{ row }">
            <div v-if="row.filter_hits?.length" class="hitcell">
              <StatusBadge v-for="(h, i) in row.filter_hits.slice(0, 2)" :key="i" :text="h.rule" :tone="ACTION_TONE[h.action as FilterAction]" />
              <span v-if="row.filter_hits.length > 2" class="muted">+{{ row.filter_hits.length - 2 }}</span>
            </div>
            <span v-else class="faint">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="150">
          <template #default="{ row }"><StatusBadge :text="STATUS[row.status as RequestLog['status']].text" :tone="STATUS[row.status as RequestLog['status']].tone" /><div class="cell-sub mono code" :title="row.error_code">{{ row.http_status }}<template v-if="row.error_code"> · {{ row.error_code }}</template></div></template>
        </el-table-column>
        <el-table-column label="Token / 耗时" width="110" align="right"><template #default="{ row }"><div class="num">{{ fmtNum(row.prompt_tokens + row.completion_tokens) }}</div><div class="cell-sub num">{{ fmtDuration(row.latency_ms) }}</div></template></el-table-column>
        <el-table-column label="来源 IP" width="115"><template #default="{ row }"><span class="mono">{{ row.source_ip }}</span></template></el-table-column>
      </el-table>
      <TablePager v-model:page="page" v-model:page-size="pageSize" :total="total" />
    </Panel>

    <RequestLogDrawer v-model:visible="drawer" :id="currentId" />

    <el-dialog v-model="setVisible" title="请求记录设置" width="500px" destroy-on-close>
      <el-alert v-if="!isSuper" type="info" :closable="false" show-icon title="仅超级管理员可修改记录设置" style="margin-bottom: 12px" />
      <el-form v-if="form" label-position="left" label-width="150px" :disabled="!isSuper">
        <el-form-item label="记录请求"><el-switch v-model="form.request_log.enabled" /><span class="hint sw">关闭后新请求不再留存</span></el-form-item>
        <el-form-item label="保存请求/响应内容"><el-switch v-model="form.request_log.capture_body" /><span class="hint sw">关闭则只记录元数据（Key、模型、状态、耗时）</span></el-form-item>
        <el-form-item label="敏感信息脱敏"><el-switch v-model="form.request_log.mask_sensitive" /><span class="hint sw">按「提示词过滤」中的脱敏规则处理后再保存</span></el-form-item>
        <el-form-item label="单条内容上限（KB）"><el-input-number v-model="form.request_log.max_body_kb" :min="1" :max="1024" controls-position="right" /></el-form-item>
        <el-form-item label="保留天数"><el-input-number v-model="form.request_log.retention_days" :min="1" :max="365" controls-position="right" /><span class="hint sw">过期记录每 6 小时清理</span></el-form-item>
      </el-form>
      <template #footer><el-button @click="setVisible = false">取消</el-button><el-button v-if="isSuper" type="primary" :loading="saving" @click="saveSettings">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh, Search, Setting } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import TimeRange from '../../components/TimeRange.vue'
import KeySelect from '../../components/KeySelect.vue'
import RequestLogDrawer from '../../components/RequestLogDrawer.vue'
import { security } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { useLookups } from '../../composables/useLookups'
import { defaultRange, fmtDateTime, fmtDuration, fmtNum, fmtTime } from '../../utils'
import type { FilterAction, GatewaySettings, RequestLog } from '../../types'

const STATUS = { success: { text: '成功', tone: 'success' }, failed: { text: '失败', tone: 'danger' }, blocked: { text: '已拦截', tone: 'warning' } } as const
const ACTION_TONE = { block: 'danger', mask: 'primary', log: 'info' } as const
const endpointText = (e: string) => ({ '/v1/chat/completions': '对话补全', '/v1/completions': '文本补全', '/v1/embeddings': '向量化' } as Record<string, string>)[e] ?? e

const { isSuper } = useAuth()
const { state, keyName, modelLabel } = useLookups()
const route = useRoute()
const range = ref(defaultRange('24h'))
const trRef = ref<InstanceType<typeof TimeRange>>()
const keyId = ref<number | undefined>(route.query.key_id ? Number(route.query.key_id) : undefined)
const modelId = ref<number>()
const status = ref(String(route.query.status ?? ''))
const q = ref('')
const rid = ref(String(route.query.request_id ?? ''))
const hitOnly = ref(route.query.filter_hit === 'true')

const { items, total, page, pageSize, loading, load, reset, search: debounced } = usePaged((p) => {
  const r = trRef.value?.current() ?? range.value
  return security.requestLogs({
    ...p, from: fmtDateTime(r.from), to: fmtDateTime(r.to), key_id: keyId.value, model_id: modelId.value, status: status.value,
    q: q.value.trim(), request_id: rid.value.trim(), filter_hit: hitOnly.value || undefined,
  })
})

const drawer = ref(false)
const currentId = ref<number>()
function open(id: number) { currentId.value = id; drawer.value = true }

const st = ref<GatewaySettings>()
const form = ref<GatewaySettings>()
const setVisible = ref(false)
const saving = ref(false)
function openSettings() { form.value = JSON.parse(JSON.stringify(st.value)); setVisible.value = true }
async function saveSettings() {
  saving.value = true
  try { st.value = await security.updateSettings(form.value!); ElMessage.success('设置已保存，约 5 秒内在所有网关实例生效'); setVisible.value = false } finally { saving.value = false }
}

onMounted(async () => { load(); st.value = await security.settings() })
</script>

<style scoped>
.strip { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; padding: 8px 12px; border: 1px solid #BBF7D0; background: var(--success-soft); color: #166534; border-radius: var(--radius-sm); font-size: 12.5px; }
.strip.off { border-color: #FDE68A; background: var(--warning-soft); color: #92400E; }
.filters { margin-bottom: 16px; }
.prompt { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; font-size: 13px; line-height: 1.5; }
.hitcell { display: flex; flex-wrap: wrap; gap: 4px; align-items: center; }
.code { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.sw { margin-left: 10px; }
:deep(.clickable) { cursor: pointer; }
</style>
