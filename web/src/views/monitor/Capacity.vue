<template>
  <div>
    <PageHeader title="容量管理" :desc="`Key 与模型粒度的 TPM / QPS 容量：对比配额与近 ${view?.window_minutes ?? 60} 分钟内观测到的峰值，提前发现触顶风险`">
      <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
    </PageHeader>

    <div class="row kpis4">
      <KpiCard label="Key 数量" :value="String(view?.keys.length ?? 0)" :hint="`${keyStat.unlimited} 个未设置 TPM 配额`" />
      <KpiCard label="高负载（≥ 80%）" :value="String(keyStat.high + modelStat.high)" unit="个" color="#F59E0B" hint="Key + 模型，按 TPM 峰值利用率" />
      <KpiCard label="触顶（≥ 100%）" :value="String(keyStat.full + modelStat.full)" unit="个" :danger="keyStat.full + modelStat.full > 0" color="#DC2626" hint="峰值已超过配额，请求可能被限流" />
      <KpiCard label="模型数量" :value="String(view?.models.length ?? 0)" :hint="`${modelStat.unlimited} 个未设置 TPM 上限`" />
    </div>

    <Panel flush>
      <el-tabs v-model="tab" class="tabs"><el-tab-pane label="部门维度" name="departments" /><el-tab-pane label="Key 维度" name="keys" /><el-tab-pane v-if="isSuper" label="模型维度" name="models" /></el-tabs>

      <div class="charts">
        <div class="ctitle">TPM 峰值利用率 Top 10<span class="muted">（仅统计已设置配额的对象）</span></div>
        <RankBars :rows="topUtil" :fmt="(v) => v.toFixed(0) + '%'" series-name="TPM 利用率" height="260px" :color="C.primary" />
      </div>

      <el-table :data="lp.rows.value" v-loading="loading" empty-text="暂无数据">
        <el-table-column :label="{ keys: 'API Key', models: '模型', departments: '部门' }[tab]" min-width="200"><template #default="{ row }"><div class="cell-title">{{ row.name }}</div><div class="cell-sub">{{ row.sub }}</div></template></el-table-column>
        <el-table-column label="TPM 配额" width="110" align="right"><template #default="{ row }"><span class="num">{{ row.tpm_limit ? fmtNum(row.tpm_limit) : '不限' }}</span></template></el-table-column>
        <el-table-column label="QPS 配额" width="100" align="right"><template #default="{ row }"><span class="num">{{ row.qps_limit ? fmtNum(row.qps_limit) : '不限' }}</span></template></el-table-column>
        <el-table-column label="峰值 TPM" width="110" align="right"><template #default="{ row }"><span class="num">{{ fmtNum(row.peak_tpm) }}</span></template></el-table-column>
        <el-table-column label="峰值 RPM" width="100" align="right"><template #default="{ row }"><span class="num">{{ fmtNum(row.peak_rpm) }}</span></template></el-table-column>
        <el-table-column label="TPM 利用率" min-width="200">
          <template #default="{ row }">
            <div v-if="row.tpm_limit" class="usage"><div class="track"><i :style="{ width: Math.min(row.tpm_util, 100) + '%', background: colorOf(row.tpm_util) }" /></div><span class="num pct" :style="{ color: row.tpm_util >= 80 ? colorOf(row.tpm_util) : undefined }">{{ row.tpm_util.toFixed(0) }}%</span></div>
            <span v-else class="faint">未设置配额</span>
          </template>
        </el-table-column>
        <el-table-column label="QPS 利用率" width="110" align="right"><template #default="{ row }"><span v-if="row.qps_limit" class="num" :style="{ color: row.qps_util >= 80 ? colorOf(row.qps_util) : undefined }">{{ row.qps_util.toFixed(0) }}%</span><span v-else class="faint">—</span></template></el-table-column>
        <el-table-column label="状态" width="100"><template #default="{ row }"><StatusBadge v-bind="badge(row)" /></template></el-table-column>
        <el-table-column label="操作" width="110" align="right" class-name="col-actions"><template #default="{ row }"><el-button v-if="tab === 'keys' ? canWrite : isSuper" link type="primary" @click="openEdit(row)">调整配额</el-button></template></el-table-column>
      </el-table>
      <TablePager v-model:page="lp.page.value" v-model:page-size="lp.pageSize.value" :total="lp.total.value" />
    </Panel>

    <el-dialog v-model="visible" :title="`调整配额 · ${editing?.name}`" width="440px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="TPM 配额（每分钟 Token 数）"><el-input-number v-model="qTpm" :min="0" :step="10000" controls-position="right" style="width: 100%" /></el-form-item>
        <el-form-item label="QPS 配额（每秒请求数）"><el-input-number v-model="qQps" :min="0" controls-position="right" style="width: 100%" /></el-form-item>
        <div class="hint">0 表示不限。近 {{ view?.window_minutes }} 分钟峰值：TPM {{ fmtNum(editing?.peak_tpm ?? 0) }}，RPM {{ fmtNum(editing?.peak_rpm ?? 0) }}。调整后立即生效并记入审计。</div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TablePager from '../../components/TablePager.vue'
import { useLocalPage } from '../../composables/usePaged'
import { useAuth } from '../../composables/useAuth'
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import RankBars from '../../components/RankBars.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { keys as keysApi, models as modelsApi, monitor, tenancy } from '../../api'
import { C } from '../../charts'
import { useLookups } from '../../composables/useLookups'
import { fmtNum } from '../../utils'
import type { CapacityRow, CapacityView } from '../../types'

const { isSuper, canWrite } = useAuth()

const { state, reload } = useLookups()
const view = ref<CapacityView | null>(null)
const loading = ref(false)
const tab = ref<'departments' | 'keys' | 'models'>('departments')

async function load() {
  loading.value = true
  try { view.value = await monitor.capacity(60) } finally { loading.value = false }
}
onMounted(load)

const rows = computed(() => (view.value ? view.value[tab.value] : []))
const lp = useLocalPage(rows)
watch(tab, () => { lp.page.value = 1 })
const stat = (list: CapacityRow[]) => ({
  unlimited: list.filter((r) => !r.tpm_limit).length,
  high: list.filter((r) => r.tpm_limit && r.tpm_util >= 80 && r.tpm_util < 100).length,
  full: list.filter((r) => r.tpm_limit && r.tpm_util >= 100).length,
})
const keyStat = computed(() => stat(view.value?.keys ?? []))
const modelStat = computed(() => stat(view.value?.models ?? []))
const topUtil = computed(() => rows.value.filter((r) => r.tpm_limit > 0).map((r) => ({ name: r.name, value: r.tpm_util })).sort((a, b) => b.value - a.value).slice(0, 10))

const colorOf = (u: number) => (u >= 100 ? '#DC2626' : u >= 80 ? '#F59E0B' : '#2563EB')
function badge(r: CapacityRow) {
  if (!r.tpm_limit && !r.qps_limit) return { text: '未限流', tone: 'info' as const }
  const u = Math.max(r.tpm_util, r.qps_util)
  if (u >= 100) return { text: '已触顶', tone: 'danger' as const }
  if (u >= 80) return { text: '高负载', tone: 'warning' as const }
  return { text: '正常', tone: 'success' as const }
}

const visible = ref(false)
const saving = ref(false)
const editing = ref<CapacityRow | null>(null)
const qTpm = ref(0)
const qQps = ref(0)
function openEdit(r: CapacityRow) { editing.value = r; qTpm.value = r.tpm_limit; qQps.value = r.qps_limit; visible.value = true }

async function save() {
  saving.value = true
  try {
    if (tab.value === 'keys') await keysApi.quota(editing.value!.id, qTpm.value, qQps.value)
    else if (tab.value === 'departments') {
      const d = state.depts.find((x) => x.id === editing.value!.id)!
      await tenancy.updateDepartment(d.id, { ...d, tpm_quota: qTpm.value, qps_quota: qQps.value })
    } else await modelsApi.update(editing.value!.id, { tpm_limit: qTpm.value, qps_limit: qQps.value })
    ElMessage.success('配额已更新'); visible.value = false
    await Promise.all([load(), reload()])
  } finally { saving.value = false }
}
</script>

<style scoped>
.row.kpis4 { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 960px) { .row.kpis4 { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.tabs { padding: 0 16px; }
.tabs :deep(.el-tabs__header) { margin: 0; }
.tabs :deep(.el-tabs__nav-wrap::after) { height: 1px; background: var(--border-soft); }
.charts { padding: 12px 16px 4px; border-bottom: 1px solid var(--border-soft); }
.ctitle { font-size: 13.5px; font-weight: 600; }
.ctitle .muted { font-size: 12px; font-weight: 400; margin-left: 8px; }
.usage { display: flex; align-items: center; gap: 12px; }
.track { flex: 1; height: 6px; border-radius: 3px; background: #EEF0F3; overflow: hidden; }
.track i { display: block; height: 100%; border-radius: 3px; }
.pct { width: 44px; text-align: right; font-weight: 500; }
</style>
