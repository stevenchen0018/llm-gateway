<template>
  <div>
    <PageHeader title="预算管理" desc="源头管控：预算需先申请并按额度分级审批（总监 / CTO）后才对 Key 生效；消耗追踪、阈值预警，耗尽后网关返回 402">
      <TransferBar name="budgets" :filters="{ approval_status: tab, q: keyword }" @imported="load" />
      <el-button v-if="canWrite" type="primary" :icon="Plus" @click="openCreate">申请预算</el-button>
    </PageHeader>

    <div class="row kpis4">
      <KpiCard label="待审批" :value="String(pendingCount)" unit="个" :danger="pendingCount > 0" color="#F59E0B" :hint="`总监 ${summary.pending_d} · CTO ${summary.pending_cto}`" />
      <KpiCard label="生效预算总额" :value="fmtMoney(totals.amount)" :hint="`${summary.approved} 个已通过`" />
      <KpiCard label="已消耗" :value="fmtMoney(totals.consumed)" color="#7C3AED" :hint="`整体使用率 ${totals.amount ? ((totals.consumed / totals.amount) * 100).toFixed(1) : '0.0'}%`" />
      <KpiCard label="需关注" :value="String(totals.attention)" unit="个" :danger="totals.attention > 0" color="#DC2626" hint="已耗尽或超过预警阈值" />
    </div>

    <Panel flush>
      <el-tabs v-model="tab" class="tabs">
        <el-tab-pane :label="`全部（${summary.total}）`" name="" />
        <el-tab-pane :label="`待审批（${pendingCount}）`" name="pending" />
        <el-tab-pane :label="`已通过（${summary.approved}）`" name="approved" />
        <el-tab-pane :label="`已驳回（${summary.rejected}）`" name="rejected" />
      </el-tabs>
      <div class="list-toolbar"><el-input v-model="keyword" placeholder="搜索 Key / 项目 / 申请人 / 理由" :prefix-icon="Search" clearable style="width: 260px" @input="pg.search" /></div>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无预算">
        <el-table-column label="绑定 Key / 项目" min-width="200"><template #default="{ row }"><div class="cell-title">{{ keyName(row.key_id) }}</div><div class="cell-sub">{{ row.project || '—' }} · 申请人 {{ row.applicant || '—' }}</div></template></el-table-column>
        <el-table-column label="周期" width="80"><template #default="{ row }">{{ periodText[row.period as Budget['period']] || '—' }}</template></el-table-column>
        <el-table-column label="额度" width="120" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(row.amount, sym(row)) }}</span></template></el-table-column>
        <el-table-column label="审批人级别" width="110">
          <template #default="{ row }"><StatusBadge :text="row.approver_level === 'CTO' ? 'CTO 审批' : row.approver_level === 'D' ? '总监审批' : '—'" :tone="row.approver_level === 'CTO' ? 'accent' : 'primary'" /></template>
        </el-table-column>
        <el-table-column label="审批状态" width="150">
          <template #default="{ row }">
            <StatusBadge :text="approvalText[row.approval_status as BudgetApproval]" :tone="approvalTone[row.approval_status as BudgetApproval]" />
            <div v-if="row.approval_status === 'rejected'" class="cell-sub" :title="row.reject_reason">{{ row.reject_reason || '—' }}</div>
            <div v-else-if="row.approval_status === 'approved'" class="cell-sub">{{ row.approver }}</div>
          </template>
        </el-table-column>
        <el-table-column label="使用率" min-width="200">
          <template #default="{ row }">
            <div v-if="row.approval_status === 'approved'" class="usage">
              <div class="track"><i :style="{ width: Math.min(ratio(row), 100) + '%', background: barColor(row) }" /><b :style="{ left: row.alert_threshold_pct + '%' }" title="预警阈值" /></div>
              <span class="num pct" :style="{ color: ratio(row) >= row.alert_threshold_pct ? barColor(row) : undefined }">{{ ratio(row).toFixed(1) }}%</span>
            </div>
            <span v-else class="faint">—</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100"><template #default="{ row }"><StatusBadge v-if="row.approval_status === 'approved'" :text="statusText[row.status as Budget['status']]" :tone="statusTone[row.status as Budget['status']]" /><span v-else class="faint">—</span></template></el-table-column>
        <el-table-column label="操作" width="140" align="right" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <span v-if="row.approval_status === 'pending' && !canDecide(row)" class="faint" :title="row.approver_level === 'CTO' ? '超过总监审批额度，需超级管理员（CTO）审批' : '需部门管理员审批'">待{{ row.approver_level === 'CTO' ? ' CTO ' : '总监' }}审批</span>
            <template v-else-if="row.approval_status === 'pending'">
              <el-popconfirm :title="`以「${row.approver_level === 'CTO' ? 'CTO' : '总监'}」身份批准该预算？`" @confirm="approve(row)"><template #reference><el-button link type="primary">通过</el-button></template></el-popconfirm>
              <el-button link type="danger" @click="openReject(row)">驳回</el-button>
            </template>
            <el-button v-else link type="primary" @click="detail = row">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <el-dialog v-model="visible" title="申请预算" width="560px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid">
          <el-form-item label="绑定 Key" prop="key_id" class="span2">
            <KeySelect v-model="form.key_id" status="active,pending" key-type="formal" placeholder="搜索尚未申请预算的正式 Key" width="100%" :clearable="false" :exclude-ids="budgetedKeyIds" />
          </el-form-item>
          <el-form-item label="所属项目" prop="project"><el-input v-model="form.project" placeholder="如 客服智能化二期" /></el-form-item>
          <el-form-item label="预算周期"><el-select v-model="form.period"><el-option v-for="(t, k) in periodText" :key="k" :label="t" :value="k" /></el-select></el-form-item>
          <el-form-item label="预算额度（元）" prop="amount">
            <el-input-number v-model="form.amount" :min="0.0001" :step="1000" :precision="4" controls-position="right" />
            <div class="hint">最小精度 0.0001</div>
          </el-form-item>
          <el-form-item label="预警阈值"><el-slider v-model="form.alert_threshold_pct" :min="10" :max="100" :step="5" /></el-form-item>
          <el-form-item label="申请事由" class="span2"><el-input v-model="form.reason" type="textarea" :rows="3" resize="none" placeholder="预算用途与预计规模" /></el-form-item>
          <div class="span2 route" :class="willBeCto ? 'cto' : 'd'">
            <el-icon><Stamp /></el-icon>
            <span>审批流程：<b>{{ willBeCto ? `额度超过 ${fmtNum(DIRECTOR_LIMIT)} 元，提交给 CTO 审批` : `额度不超过 ${fmtNum(DIRECTOR_LIMIT)} 元，提交给总监（D）审批` }}</b>。通过后自动绑定 Key 并开始管控。</span>
          </div>
        </div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">提交申请</el-button></template>
    </el-dialog>

    <el-dialog v-model="rejectVisible" title="驳回预算申请" width="440px" destroy-on-close>
      <el-input v-model="rejectReason" type="textarea" :rows="3" resize="none" placeholder="请填写驳回原因，申请人将看到该说明" />
      <template #footer><el-button @click="rejectVisible = false">取消</el-button><el-button type="danger" :loading="saving" @click="reject">确认驳回</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="预算详情" size="420px">
      <dl v-if="detail" class="dl">
        <div><dt>绑定 Key</dt><dd>{{ keyName(detail.key_id) }}</dd></div>
        <div><dt>项目</dt><dd>{{ detail.project || '—' }}</dd></div>
        <div><dt>申请人</dt><dd>{{ detail.applicant || '—' }}</dd></div>
        <div><dt>额度 / 已用</dt><dd class="num">{{ fmtMoney(detail.amount, sym(detail)) }} / {{ fmtMoney(detail.consumed, sym(detail)) }}</dd></div>
        <div><dt>审批级别</dt><dd>{{ detail.approver_level === 'CTO' ? 'CTO' : '总监（D）' }}</dd></div>
        <div><dt>审批人</dt><dd>{{ detail.approver || '—' }}</dd></div>
        <div><dt>审批时间</dt><dd>{{ fmtTime(detail.approved_at) }}</dd></div>
        <div class="full"><dt>申请事由</dt><dd>{{ detail.reason || '—' }}</dd></div>
        <div v-if="detail.reject_reason" class="full"><dt>驳回原因</dt><dd>{{ detail.reject_reason }}</dd></div>
      </dl>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../components/transfer/TransferBar.vue'
import TablePager from '../components/TablePager.vue'
import { usePaged } from '../composables/usePaged'
import KeySelect from '../components/KeySelect.vue'
import { useAuth } from '../composables/useAuth'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, type FormInstance } from 'element-plus'
import { Plus, Search, Stamp } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import KpiCard from '../components/KpiCard.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { budgets } from '../api'
import { DIRECTOR_LIMIT } from '../constants'
import { useLookups } from '../composables/useLookups'
import { fmtMoney, fmtNum, fmtTime } from '../utils'
import type { BudgetSummary, Budget, BudgetApproval } from '../types'

const { isSuper, canWrite } = useAuth()

const periodText: Record<Budget['period'], string> = { monthly: '月度', quarterly: '季度', none: '长期' }
const statusText: Record<Budget['status'], string> = { active: '正常', exhausted: '已耗尽', closed: '已关闭', pending: '待生效' }
const statusTone: Record<Budget['status'], 'success' | 'danger' | 'info' | 'warning'> = { active: 'success', exhausted: 'danger', closed: 'info', pending: 'warning' }
const approvalText: Record<BudgetApproval, string> = { pending: '待审批', approved: '已通过', rejected: '已驳回' }
const approvalTone: Record<BudgetApproval, 'warning' | 'success' | 'danger'> = { pending: 'warning', approved: 'success', rejected: 'danger' }

const { state, keyName, reload } = useLookups()
// D-level: department admin (or super); CTO-level: super admin only
const canDecide = (b: Budget) => (b.approver_level === 'CTO' ? isSuper.value : canWrite.value)
const tab = ref('')
const keyword = ref('')
const pg = usePaged((p) => budgets.page({ ...p, approval_status: tab.value, q: keyword.value.trim() }))
watch(tab, () => pg.reset())

// KPI cards and tab counts cover every budget in scope, not just this page
const summary = ref<BudgetSummary>({ total: 0, pending: 0, pending_d: 0, pending_cto: 0, approved: 0, rejected: 0, amount: 0, consumed: 0, attention: 0, open_key_ids: [] })
async function load() { await Promise.all([pg.refresh(), budgets.summary().then((s) => { summary.value = s })]) }
onMounted(load)

const pendingCount = computed(() => summary.value.pending)
const sym = (b: Budget) => (b.currency === 'CNY' ? '¥' : b.currency + ' ')
const ratio = (b: Budget) => (Number(b.amount) ? (Number(b.consumed) / Number(b.amount)) * 100 : 0)
const barColor = (b: Budget) => (b.status === 'exhausted' || ratio(b) >= 100 ? '#DC2626' : ratio(b) >= b.alert_threshold_pct ? '#F59E0B' : '#2563EB')
const totals = computed(() => ({ amount: summary.value.amount, consumed: summary.value.consumed, attention: summary.value.attention }))
// keys that already have an open (pending/approved) budget are shown disabled
const budgetedKeyIds = computed(() => summary.value.open_key_ids)

// ---- apply ---------------------------------------------------------------------------------
const visible = ref(false)
const saving = ref(false)
const formRef = ref<FormInstance>()
const blank = () => ({ key_id: undefined as number | undefined, project: '', amount: 5000, period: 'monthly', alert_threshold_pct: 80, reason: '' })
const form = reactive(blank())
const rules = {
  key_id: [{ required: true, message: '请选择 Key', trigger: 'change' }],
  project: [{ required: true, message: '请填写所属项目', trigger: 'blur' }],
  amount: [{ required: true, message: '请输入额度', trigger: 'blur' }],
}
const willBeCto = computed(() => form.amount > DIRECTOR_LIMIT)

function openCreate() { Object.assign(form, blank()); visible.value = true }
async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    await budgets.create({ key_id: form.key_id, amount: form.amount.toFixed(4), period: form.period, alert_threshold_pct: form.alert_threshold_pct, project: form.project, reason: form.reason })
    ElMessage.success(`预算申请已提交，等待${willBeCto.value ? ' CTO ' : '总监'}审批`); visible.value = false; await load()
  } finally { saving.value = false }
}

// ---- approve / reject ------------------------------------------------------------------------
async function approve(b: Budget) { await budgets.approve(b.id); ElMessage.success('已通过并绑定到 Key'); await Promise.all([load(), reload()]) }
const rejectVisible = ref(false)
const rejectReason = ref('')
const rejecting = ref<Budget | null>(null)
function openReject(b: Budget) { rejecting.value = b; rejectReason.value = ''; rejectVisible.value = true }
async function reject() {
  saving.value = true
  try { await budgets.reject(rejecting.value!.id, rejectReason.value); ElMessage.success('已驳回'); rejectVisible.value = false; await load() } finally { saving.value = false }
}

const detail = ref<Budget | null>(null)
const detailVisible = computed({ get: () => !!detail.value, set: (v) => { if (!v) detail.value = null } })
</script>

<style scoped>
.row.kpis4 { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 960px) { .row.kpis4 { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.tabs { padding: 0 16px; }
.tabs :deep(.el-tabs__header) { margin: 0; }
.tabs :deep(.el-tabs__nav-wrap::after) { height: 1px; background: var(--border-soft); }
.usage { display: flex; align-items: center; gap: 12px; }
.track { position: relative; flex: 1; height: 6px; border-radius: 3px; background: #EEF0F3; }
.track i { display: block; height: 100%; border-radius: 3px; }
.track b { position: absolute; top: -3px; width: 2px; height: 12px; background: #9CA3AF; border-radius: 1px; }
.pct { width: 64px; text-align: right; font-weight: 500; white-space: nowrap; }
.route { display: flex; align-items: center; gap: 8px; padding: 10px 12px; border-radius: 8px; font-size: 12.5px; line-height: 1.5; }
.route.d { background: var(--primary-soft); color: #1E40AF; }
.route.cto { background: var(--accent-soft); color: #5B21B6; }
.dl { margin: 0; display: grid; grid-template-columns: 1fr 1fr; gap: 14px 16px; }
.dl .full { grid-column: 1 / -1; }
dt { font-size: 12px; color: var(--text-2); }
dd { margin: 3px 0 0; font-size: 13.5px; }
</style>
