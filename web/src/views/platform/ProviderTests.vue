<template>
  <div>
    <PageHeader title="供应商可用性测试" desc="探测每家供应商的连通性与鉴权，并对其模型发起最小真实调用，统计成功率与耗时；支持手动、批量与定时巡检，异常时推送告警">
      <el-button :icon="Timer" @click="openSchedule">定时巡检{{ schedule?.enabled ? `：每 ${intervalText(schedule.interval_minutes)}` : '：未开启' }}</el-button>
      <el-button v-if="isSuper" type="primary" :icon="VideoPlay" :loading="runningAll" @click="runAll">测试全部供应商</el-button>
    </PageHeader>

    <div class="kpis">
      <KpiCard label="启用的供应商" :value="String(activeRows.length)" :hint="`共 ${rows.length} 家`" />
      <KpiCard label="最近测试全部通过" :value="String(count('success'))" hint="最近一次测试的结果" />
      <KpiCard label="存在异常" :value="String(count('partial') + count('failed'))" :hint="`失败 ${count('failed')} · 部分失败 ${count('partial')}`" :danger="count('failed') > 0" />
      <KpiCard label="平均可用率（7 天）" :value="avgAvail" :hint="`${untested} 家尚未测试`" />
    </div>

    <Panel title="供应商可用性" :count="filtered.length" flush>
      <template #toolbar>
        <el-radio-group v-model="fView"><el-radio-button value="">全部</el-radio-button><el-radio-button value="abnormal">异常</el-radio-button><el-radio-button value="untested">未测试</el-radio-button></el-radio-group>
        <el-select v-model="fProtocol" clearable placeholder="全部协议" style="width: 130px"><el-option v-for="k in ['openai', 'anthropic', 'gemini', 'mock']" :key="k" :label="PROTOCOL[k].text" :value="k" /></el-select>
        <el-input v-model="keyword" placeholder="搜索供应商 / 编码" :prefix-icon="Search" clearable style="width: 220px" />
        <span class="spacer" />
        <span v-if="anyRunning" class="muted"><el-icon class="is-loading"><Loading /></el-icon> 有测试正在进行，结果会自动刷新</span>
      </template>
      <el-table :data="lp.rows.value" v-loading="loading" empty-text="没有符合条件的供应商">
        <el-table-column label="供应商" min-width="200">
          <template #default="{ row }"><div class="mcell"><VendorLogo :code="row.provider.code" :size="28" /><div><div class="cell-title">{{ row.provider.name }}</div><div class="cell-sub mono">{{ row.provider.code }}</div></div></div></template>
        </el-table-column>
        <el-table-column label="协议" width="110"><template #default="{ row }"><ProtocolTag :provider="row.provider" /></template></el-table-column>
        <el-table-column label="最近一次测试" min-width="260">
          <template #default="{ row }">
            <template v-if="row.last_run">
              <div class="last"><StatusBadge :text="TEST_RUN_STATUS[row.last_run.status]?.text ?? row.last_run.status" :tone="TEST_RUN_STATUS[row.last_run.status]?.tone ?? 'info'" /><span class="cell-sub">{{ fmtTime(row.last_run.started_at) }} · {{ TEST_TRIGGER[row.last_run.trigger] }}</span></div>
              <div class="cell-sub sum" :title="row.last_run.summary">{{ row.last_run.summary || '测试进行中…' }}</div>
            </template>
            <span v-else class="faint">未测试</span>
          </template>
        </el-table-column>
        <el-table-column label="可用率（7 天）" width="160">
          <template #default="{ row }">
            <div v-if="row.availability >= 0" class="avail"><div class="track"><div class="fill" :class="tone(row.availability)" :style="{ width: `${row.availability * 100}%` }" /></div><span class="num">{{ pct(row.availability) }}</span></div>
            <span v-else class="faint">—</span>
          </template>
        </el-table-column>
        <el-table-column label="平均 / P95 耗时" width="176" align="right"><template #default="{ row }"><span class="num">{{ row.avg_latency_ms ? `${fmtDuration(row.avg_latency_ms)} / ${fmtDuration(row.p95_latency_ms)}` : '—' }}</span></template></el-table-column>
        <el-table-column label="状态" width="80"><template #default="{ row }"><StatusBadge :text="row.provider.status === 'active' ? '启用' : '停用'" :tone="row.provider.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="操作" :width="isSuper ? 110 : 70" align="right" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button v-if="isSuper" link type="primary" :disabled="row.provider.status !== 'active' || row.last_run?.status === 'running'" @click="openTest(row.provider)">测试</el-button>
            <el-button link type="primary" @click="openReport(row.provider.id)">报告</el-button>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="lp.page.value" v-model:page-size="lp.pageSize.value" :total="lp.total.value" />
    </Panel>

    <Panel title="测试记录" :count="runsPg.total.value" flush class="mt">
      <template #toolbar>
        <el-select v-model="rProvider" clearable filterable placeholder="全部供应商" style="width: 180px" @change="runsPg.reset"><el-option v-for="r in rows" :key="r.provider.id" :label="r.provider.name" :value="r.provider.id" /></el-select>
        <el-select v-model="rTrigger" clearable placeholder="全部方式" style="width: 120px" @change="runsPg.reset"><el-option v-for="(t, k) in TEST_TRIGGER" :key="k" :label="t" :value="k" /></el-select>
        <el-select v-model="rStatus" clearable placeholder="全部结果" style="width: 120px" @change="runsPg.reset"><el-option v-for="(t, k) in TEST_RUN_STATUS" :key="k" :label="t.text" :value="k" /></el-select>
      </template>
      <el-table :data="runsPg.items.value" v-loading="runsPg.loading.value" empty-text="暂无测试记录" class="runs" @row-click="openRun">
        <el-table-column label="时间" width="170"><template #default="{ row }"><span class="num">{{ fmtTime(row.started_at) }}</span></template></el-table-column>
        <el-table-column label="供应商" min-width="150"><template #default="{ row }">{{ providerName(row.provider_id) }}</template></el-table-column>
        <el-table-column label="协议" width="110"><template #default="{ row }"><ProtocolTag :protocol="row.protocol === 'mock' ? 'mock' : row.protocol" /></template></el-table-column>
        <el-table-column label="方式" width="90"><template #default="{ row }">{{ TEST_TRIGGER[row.trigger] ?? row.trigger }}</template></el-table-column>
        <el-table-column label="结果" width="112"><template #default="{ row }"><StatusBadge :text="TEST_RUN_STATUS[row.status]?.text ?? row.status" :tone="TEST_RUN_STATUS[row.status]?.tone ?? 'info'" /></template></el-table-column>
        <el-table-column label="通过 / 失败 / 跳过" width="140" align="right"><template #default="{ row }"><span class="num">{{ row.passed }} / <span :class="{ bad: row.failed }">{{ row.failed }}</span> / {{ row.skipped }}</span></template></el-table-column>
        <el-table-column label="摘要" min-width="260" show-overflow-tooltip prop="summary" />
        <el-table-column label="操作人" width="110" prop="operator" />
      </el-table>
      <TablePager v-model:page="runsPg.page.value" v-model:page-size="runsPg.pageSize.value" :total="runsPg.total.value" />
    </Panel>

    <el-dialog v-model="scheduleVisible" title="定时巡检" width="480px">
      <el-form label-position="top">
        <el-form-item label="开启定时巡检"><el-switch v-model="sForm.enabled" :disabled="!isSuper" /><div class="hint">开启后由主节点按间隔测试全部启用的供应商（集群内只执行一次）</div></el-form-item>
        <div class="form-grid">
          <el-form-item label="巡检间隔"><el-select v-model="sForm.interval_minutes" :disabled="!isSuper"><el-option v-for="m in [5, 10, 15, 30, 60, 120, 360, 720, 1440]" :key="m" :label="`每 ${intervalText(m)}`" :value="m" /></el-select></el-form-item>
          <el-form-item label="单项超时（秒）"><el-input-number v-model="sForm.timeout_seconds" :min="5" :max="120" :step="5" controls-position="right" :disabled="!isSuper" /></el-form-item>
          <el-form-item label="测试记录保留（天）"><el-input-number v-model="sForm.retention_days" :min="7" :max="365" controls-position="right" :disabled="!isSuper" /></el-form-item>
          <el-form-item label="失败告警"><el-switch v-model="sForm.alert_on_failure" :disabled="!isSuper" /><div class="hint">巡检未全部通过时写入告警中心并推送飞书</div></el-form-item>
        </div>
        <el-alert type="info" :closable="false" show-icon title="每次巡检会对每个模型发起一次最小调用（max_tokens=32），会产生极少量费用；图片 / 视频 / 语音类模型不做自动测试。" />
      </el-form>
      <template #footer><el-button @click="scheduleVisible = false">取消</el-button><el-button v-if="isSuper" type="primary" :loading="savingSchedule" @click="saveSchedule">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="runVisible" :title="`测试明细 #${runDetail?.id ?? ''} · ${runDetail ? providerName(runDetail.provider_id) : ''}`" width="860px">
      <div v-if="runDetail" class="rsum"><StatusBadge :text="TEST_RUN_STATUS[runDetail.status]?.text ?? runDetail.status" :tone="TEST_RUN_STATUS[runDetail.status]?.tone ?? 'info'" /><span>{{ runDetail.summary || '测试进行中…' }}</span></div>
      <TestResultTable v-if="runDetail?.results?.length" :results="runDetail.results" max-height="460" />
    </el-dialog>

    <ProviderTestDialog v-model:visible="testVisible" :provider="testing" @report="(p) => { testVisible = false; openReport(p.id) }" @done="refreshAll" />
    <ProviderReportDrawer v-model:visible="reportVisible" :provider-id="reportId" @test="(p) => openTest(p)" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading, Search, Timer, VideoPlay } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import KpiCard from '../../components/KpiCard.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import TablePager from '../../components/TablePager.vue'
import VendorLogo from '../../components/VendorLogo.vue'
import ProtocolTag from '../../components/provider/ProtocolTag.vue'
import ProviderTestDialog from '../../components/provider/ProviderTestDialog.vue'
import ProviderReportDrawer from '../../components/provider/ProviderReportDrawer.vue'
import TestResultTable from '../../components/provider/TestResultTable.vue'
import { providerTests } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { useLocalPage, usePaged } from '../../composables/usePaged'
import { PROTOCOL, TEST_RUN_STATUS, TEST_TRIGGER } from '../../constants'
import { fmtDuration, fmtTime } from '../../utils'
import type { Provider, ProviderTestOverview, ProviderTestRun, ProviderTestSchedule } from '../../types'

const { isSuper } = useAuth()

const rows = ref<ProviderTestOverview[]>([])
const loading = ref(false)
const fView = ref('')
const fProtocol = ref('')
const keyword = ref('')

const protocolOf = (p: Provider) => (p.base_url.startsWith('mock://') ? 'mock' : p.protocol || 'openai') // filter: Mock = local test suppliers
const activeRows = computed(() => rows.value.filter((r) => r.provider.status === 'active'))
const count = (status: string) => activeRows.value.filter((r) => r.last_run?.status === status).length
const untested = computed(() => activeRows.value.filter((r) => !r.last_run).length)
const avgAvail = computed(() => {
  const xs = activeRows.value.filter((r) => r.availability >= 0)
  return xs.length ? pct(xs.reduce((a, r) => a + r.availability, 0) / xs.length) : '—'
})
const anyRunning = computed(() => rows.value.some((r) => r.last_run?.status === 'running'))
const filtered = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return rows.value.filter((r) =>
    (!fView.value || (fView.value === 'untested' ? !r.last_run : r.last_run && (r.last_run.status === 'failed' || r.last_run.status === 'partial'))) &&
    (!fProtocol.value || protocolOf(r.provider) === fProtocol.value) &&
    (!q || r.provider.name.toLowerCase().includes(q) || r.provider.code.toLowerCase().includes(q)))
})
const lp = useLocalPage(filtered)

const pct = (v: number) => `${(v * 100).toFixed(v === 1 ? 0 : 1)}%`
const tone = (v: number) => (v >= 0.99 ? 'good' : v >= 0.9 ? 'warn' : 'bad')
const intervalText = (m: number) => (m >= 60 && m % 60 === 0 ? `${m / 60} 小时` : `${m} 分钟`)
const providerName = (id: number) => rows.value.find((r) => r.provider.id === id)?.provider.name ?? `#${id}`

async function loadOverview() {
  loading.value = rows.value.length === 0
  try { rows.value = await providerTests.overview(7) } finally { loading.value = false }
}

// test history
const rProvider = ref<number>()
const rTrigger = ref('')
const rStatus = ref('')
const runsPg = usePaged((p) => providerTests.runs({ ...p, provider_id: rProvider.value, trigger: rTrigger.value, status: rStatus.value }))

const refreshAll = () => Promise.all([loadOverview(), runsPg.refresh()])

// while any run is in progress, refresh every 3 s
let poller: number | undefined
function watchRunning() {
  if (poller) return
  poller = window.setInterval(async () => {
    await refreshAll()
    if (!anyRunning.value) { clearInterval(poller); poller = undefined }
  }, 3000)
}
onBeforeUnmount(() => poller && clearInterval(poller))

onMounted(async () => {
  await Promise.all([refreshAll(), loadSchedule()])
  if (anyRunning.value) watchRunning()
})

const runningAll = ref(false)
async function runAll() {
  runningAll.value = true
  try {
    const runs = await providerTests.runAll()
    ElMessage.success(runs.length ? `已开始测试 ${runs.length} 家供应商，结果会自动刷新` : '没有需要测试的供应商（可能都在测试中）')
    await refreshAll()
    watchRunning()
  } finally { runningAll.value = false }
}

// schedule
const schedule = ref<ProviderTestSchedule | null>(null)
const scheduleVisible = ref(false)
const savingSchedule = ref(false)
const sForm = reactive<ProviderTestSchedule>({ enabled: false, interval_minutes: 60, timeout_seconds: 30, alert_on_failure: true, retention_days: 90 })
async function loadSchedule() { try { schedule.value = await providerTests.schedule() } catch { /* optional */ } }
function openSchedule() { if (schedule.value) Object.assign(sForm, schedule.value); scheduleVisible.value = true }
async function saveSchedule() {
  savingSchedule.value = true
  try {
    schedule.value = await providerTests.saveSchedule({ ...sForm })
    ElMessage.success(sForm.enabled ? `已开启定时巡检：每 ${intervalText(sForm.interval_minutes)}（1 分钟内开始第一轮）` : '已关闭定时巡检')
    scheduleVisible.value = false
  } finally { savingSchedule.value = false }
}

// dialogs
const testVisible = ref(false)
const testing = ref<Provider | null>(null)
const reportVisible = ref(false)
const reportId = ref<number | null>(null)
const runVisible = ref(false)
const runDetail = ref<ProviderTestRun | null>(null)
function openTest(p: Provider) { testing.value = p; reportVisible.value = false; testVisible.value = true }
function openReport(id: number) { reportId.value = id; reportVisible.value = true }
async function openRun(row: ProviderTestRun) { runDetail.value = await providerTests.run(row.id); runVisible.value = true }
</script>

<style scoped>
.kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 16px; }
.mcell { display: flex; align-items: center; gap: 10px; }
.last { display: flex; align-items: center; gap: 8px; }
.sum { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.avail { display: flex; align-items: center; gap: 8px; }
.track { flex: 1; height: 6px; border-radius: 3px; background: #EEF0F3; overflow: hidden; }
.fill { height: 100%; border-radius: 3px; }
.fill.good { background: #16A34A; }
.fill.warn { background: #F59E0B; }
.fill.bad { background: #DC2626; }
.avail .num { width: 46px; text-align: right; font-size: 12px; }
.spacer { flex: 1; }
.runs :deep(tr) { cursor: pointer; }
.bad { color: var(--danger); }
.rsum { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; color: var(--text-2); }
.hint { font-size: 12px; color: var(--text-3); line-height: 18px; margin-top: 4px; width: 100%; }
.mt { margin-top: 16px; }
@media (max-width: 1100px) { .kpis { grid-template-columns: repeat(2, 1fr); } }
</style>
