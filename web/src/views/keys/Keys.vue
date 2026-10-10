<template>
  <div>
    <PageHeader title="Key 管理" desc="应用 Key 与员工个人编码 Key 的审批分发、配额、可用模型、IP 白名单与黑名单，生命周期全程留痕">
      <TransferBar name="keys" :filters="{ category, status: statusFilter, q: keyword }" @imported="load" />
      <el-button type="primary" :icon="Plus" @click="$router.push('/keys/apply')">申请 Key</el-button>
    </PageHeader>

    <Panel flush>
      <el-tabs v-model="category" class="tabs" @tab-change="reset">
        <el-tab-pane label="全部 Key" name="" />
        <el-tab-pane name="application"><template #label>应用 Key<span class="tab-hint">线上业务系统</span></template></el-tab-pane>
        <el-tab-pane name="personal"><template #label>个人编码 Key<span class="tab-hint">员工日常编码</span></template></el-tab-pane>
      </el-tabs>
      <div class="list-toolbar">
        <el-radio-group v-model="statusFilter" @change="reset">
          <el-radio-button value="">全部状态</el-radio-button>
          <el-radio-button v-for="(t, k) in statusText" :key="k" :value="k">{{ t }}</el-radio-button>
        </el-radio-group>
        <el-select v-if="category !== 'personal'" v-model="appFilter" clearable filterable placeholder="全部应用" style="width: 150px" @change="reset"><el-option v-for="a in state.apps" :key="a.id" :label="a.name" :value="a.id" /></el-select>
        <el-input v-model="keyword" :placeholder="category === 'personal' ? '姓名 / 邮箱 / 工号 / 前缀 / #ID' : '名称 / 负责人 / 前缀 / 应用 / #ID'" :prefix-icon="Search" clearable style="width: 250px" @input="onKeyword" />
        <span class="spacer" />
        <span class="muted">共 {{ total }} 个</span>
      </div>
      <el-table :data="rows" v-loading="loading" empty-text="暂无 Key">
        <el-table-column label="名称 / 场景" min-width="200">
          <template #default="{ row }"><div class="cell-title">{{ row.name }}</div><div class="cell-sub"><span v-if="hasSecret(row)" class="mono">{{ row.key_prefix }}…</span><span v-if="hasSecret(row) && row.scenario"> · </span>{{ row.scenario || (hasSecret(row) ? '' : '—') }}</div></template>
        </el-table-column>
        <el-table-column label="类型" width="132">
          <template #default="{ row }">
            <StatusBadge :text="KEY_CATEGORY[row.category].text" :tone="KEY_CATEGORY[row.category].tone" />
            <div v-if="row.category === 'application'" class="cell-sub">{{ row.key_type === 'trial' ? '试用' : '正式' }}<template v-if="row.expires_at"> · 至 {{ row.expires_at.slice(5, 10) }}</template></div>
            <div v-else-if="row.coding_tools?.length" class="cell-sub" :title="row.coding_tools.join('、')">{{ row.coding_tools.join('、') }}</div>
          </template>
        </el-table-column>
        <el-table-column label="归属" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <template v-if="row.category === 'personal'"><div>{{ row.owner }}<span v-if="row.employee_no" class="muted mono"> {{ row.employee_no }}</span></div><div class="cell-sub">{{ deptName(row.department_id) }} · {{ row.owner_email || '—' }}</div></template>
            <template v-else><div>{{ row.app_id ? appName(row.app_id) : '—' }}</div><div class="cell-sub">{{ deptName(row.department_id) }} · {{ row.owner }}</div></template>
          </template>
        </el-table-column>
        <el-table-column label="TPM / QPS" width="110" align="right"><template #default="{ row }"><span class="num">{{ row.tpm_quota ? compactN(row.tpm_quota) : '不限' }} / {{ row.qps_quota ? row.qps_quota : '不限' }}</span></template></el-table-column>
        <el-table-column label="可用模型" width="100">
          <template #default="{ row }">
            <el-tooltip v-if="row.allowed_models?.length" placement="top" :content="row.allowed_models.join('\n')" popper-class="pre-tip"><span class="chip-link">{{ row.allowed_models.length }} 个</span></el-tooltip>
            <span v-else class="faint">全部</span>
          </template>
        </el-table-column>
        <el-table-column label="IP" width="72">
          <template #default="{ row }">
            <el-tooltip v-if="row.ip_whitelist?.length" placement="top" :content="row.ip_whitelist.join('\n')" popper-class="pre-tip"><span class="chip-link"><el-icon><Lock /></el-icon>{{ row.ip_whitelist.length }}</span></el-tooltip>
            <span v-else class="faint">不限</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="96"><template #default="{ row }"><StatusBadge :text="statusLabel(row)" :tone="statusToneOf(row)" /></template></el-table-column>
        <el-table-column label="申请时间" width="160"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="操作" width="140" align="right" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button v-if="canManage(row) && row.status === 'pending'" link type="primary" @click="approve(row)">审批</el-button>
            <el-button v-if="canManage(row) && row.status === 'active'" link type="primary" @click="openQuota(row)">配额</el-button>
            <el-dropdown trigger="click" @command="(c: string) => onMore(c, row)">
              <el-button link type="primary">更多<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item v-if="canManage(row) && row.status !== 'blacklisted'" command="models">可用模型</el-dropdown-item>
                  <el-dropdown-item v-if="canManage(row) && row.status !== 'blacklisted'" command="ips">IP 白名单</el-dropdown-item>
                  <el-dropdown-item v-if="canManage(row) && row.status === 'active' && hasSecret(row)" command="rotate">重置密钥</el-dropdown-item>
                  <el-dropdown-item command="history">审计记录</el-dropdown-item>
                  <el-dropdown-item v-if="canManage(row) && row.status !== 'blacklisted'" command="blacklist" divided><span style="color: var(--danger)">加入黑名单</span></el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="page" v-model:page-size="pageSize" :total="total" />
    </Panel>

    <el-dialog v-model="modelsVisible" :title="`可用模型 · ${target?.name}`" width="560px" destroy-on-close>
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px" title="限制该 Key 能调用的模型名；留空表示可调用全部已上架模型。"
        description="按模型名匹配，与供应商无关（如 deepseek-v3 可由任一供应商提供）；调用名单外的模型网关返回 403 model_not_allowed。" />
      <el-select v-model="allowed" multiple filterable allow-create default-first-option placeholder="选择或输入模型名" style="width: 100%">
        <el-option-group v-for="g in modelGroups" :key="g.label" :label="g.label"><el-option v-for="m in g.names" :key="m" :label="m" :value="m" /></el-option-group>
      </el-select>
      <div class="presets"><el-button link type="primary" @click="allowed = [...DEFAULT_CODING_MODELS]">使用推荐编码模型</el-button><el-button link @click="allowed = []">清空（不限）</el-button></div>
      <template #footer><el-button @click="modelsVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveModels">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="ipVisible" :title="`IP 白名单 · ${target?.name}`" width="520px" destroy-on-close>
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px"
        title="仅允许名单内的来源地址调用该 Key；留空表示不限制。"
        description="每行一个 IPv4 / IPv6 地址或 CIDR 网段（如 10.20.0.0/16）。网关按 TCP 对端地址判断，经负载均衡接入时需在 server.trusted_proxies 中配置可信代理。" />
      <el-input v-model="ipText" type="textarea" :rows="7" resize="none" class="mono" placeholder="10.20.0.0/16&#10;192.168.10.25&#10;2001:db8::/32" />
      <div class="ip-foot">
        <span class="hint">{{ ipLines.length }} 条</span>
        <span v-if="ipInvalid.length" class="bad">格式错误：{{ ipInvalid.slice(0, 3).join('、') }}{{ ipInvalid.length > 3 ? ' 等' : '' }}</span>
      </div>
      <template #footer><el-button @click="ipVisible = false">取消</el-button><el-button type="primary" :loading="saving" :disabled="ipInvalid.length > 0" @click="saveIPs">保存</el-button></template>
    </el-dialog>

    <SecretDialog :secret="secret" @close="secret = ''" />

    <el-dialog v-model="blVisible" :title="`加入黑名单 · ${target?.name}`" width="460px" destroy-on-close>
      <el-alert type="error" :closable="false" show-icon title="拉黑后该 Key 立即失效，网关将拒绝其全部请求" style="margin-bottom: 12px" />
      <el-input v-model="blReason" type="textarea" :rows="3" resize="none" placeholder="请填写拉黑原因（将记入审计日志）" />
      <template #footer><el-button @click="blVisible = false">取消</el-button><el-button type="danger" :loading="saving" :disabled="!blReason.trim()" @click="blacklist">确认拉黑</el-button></template>
    </el-dialog>

    <el-dialog v-model="quotaVisible" :title="`调整配额 · ${target?.name}`" width="440px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="TPM 配额（每分钟 Token 数）"><el-input-number v-model="qTpm" :min="0" :step="10000" controls-position="right" style="width: 100%" /></el-form-item>
        <el-form-item label="QPS 配额（每秒请求数）"><el-input-number v-model="qQps" :min="0" controls-position="right" style="width: 100%" /></el-form-item>
        <div class="hint">0 表示不限；同时受部门配额与模型容量约束。调整后立即生效。</div>
      </el-form>
      <template #footer><el-button @click="quotaVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveQuota">保存</el-button></template>
    </el-dialog>

    <el-drawer v-model="historyVisible" :title="`审计记录 · ${target?.name}`" size="420px">
      <el-timeline style="padding-top: 8px">
        <el-timeline-item v-for="h in history" :key="h.id" :timestamp="fmtTime(h.created_at)" placement="top" :color="ACTION[h.action]?.color">
          <b>{{ ACTION[h.action]?.text ?? h.action }}</b><span class="muted">　操作人 {{ h.operator || '—' }}</span>
          <div v-if="h.detail" class="cell-sub">{{ h.detail }}</div>
        </el-timeline-item>
      </el-timeline>
      <EmptyState v-if="!history.length" text="暂无记录" height="160px" />
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Lock, Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import EmptyState from '../../components/EmptyState.vue'
import TablePager from '../../components/TablePager.vue'
import SecretDialog from '../../components/SecretDialog.vue'
import { keys } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { useLookups } from '../../composables/useLookups'
import { usePaged } from '../../composables/usePaged'
import { compact as compactN } from '../../charts'
import { DEFAULT_CODING_MODELS, KEY_CATEGORY } from '../../constants'
import { fmtTime, isIPOrCIDR } from '../../utils'
import type { ApiKey, AuditLog, KeyCategory, KeyStatus } from '../../types'

const { canWrite, state: auth } = useAuth()
const route = useRoute()

const statusText: Record<KeyStatus, string> = { pending: '待审批', active: '生效中', blacklisted: '已拉黑' }
const ACTION: Record<AuditLog['action'], { text: string; color: string }> = {
  apply: { text: '提交申请', color: '#2563EB' }, approve: { text: '审批通过', color: '#16A34A' }, blacklist: { text: '加入黑名单', color: '#DC2626' },
  restore: { text: '移出黑名单', color: '#F59E0B' }, update_quota: { text: '调整配额', color: '#7C3AED' },
  update_ip_whitelist: { text: '修改 IP 白名单', color: '#0EA5E9' }, update_models: { text: '修改可用模型', color: '#7C3AED' },
  claim: { text: '持有人领取密钥', color: '#16A34A' }, rotate: { text: '重置密钥', color: '#F59E0B' },
}

const { state, appName, deptName, reload } = useLookups()
const category = ref<KeyCategory | ''>((route.query.category as KeyCategory) ?? '')
const statusFilter = ref<KeyStatus | ''>('')
const appFilter = ref<number>()
const keyword = ref('')
const saving = ref(false)
const target = ref<ApiKey | null>(null)

const hasSecret = (k: ApiKey) => k.key_prefix.startsWith('sk-')
// an approved personal key whose holder has not claimed the secret yet
const unclaimed = (k: ApiKey) => k.status === 'active' && !hasSecret(k)
const statusLabel = (k: ApiKey) => (unclaimed(k) ? '待领取' : statusText[k.status])
const statusToneOf = (k: ApiKey) => (unclaimed(k) ? 'info' : ({ pending: 'warning', active: 'success', blacklisted: 'danger' } as const)[k.status])
const canManage = (k: ApiKey) => canWrite.value && (auth.me?.permissions.super || k.department_id === auth.me?.department_id)

// server-side search & paging: stays fast however many keys exist
const { items: rows, total, page, pageSize, loading, load, reset, search: onKeyword } = usePaged((p) =>
  keys.search({ ...p, q: keyword.value.trim(), category: category.value || undefined, status: statusFilter.value || undefined, app_id: category.value === 'personal' ? undefined : appFilter.value }))
onMounted(load)

function onMore(cmd: string, k: ApiKey) {
  target.value = k
  if (cmd === 'models') openModels(k)
  else if (cmd === 'ips') openIPs(k)
  else if (cmd === 'rotate') rotate(k)
  else if (cmd === 'history') openHistory(k)
  else if (cmd === 'blacklist') openBlacklist(k)
}

// ---- approve / rotate ----------------------------------------------------------------
const secret = ref('')
async function approve(k: ApiKey) {
  const res = await keys.approve(k.id)
  if (res.claim_required) ElMessageBox.alert(`已审批通过。密钥由持有人「${k.owner}」登录控制台后在「我的 Key」中自行领取，审批人不会看到密钥。`, '审批通过', { type: 'success' })
  else secret.value = res.secret ?? ''
  await Promise.all([load(), reload()])
}
async function rotate(k: ApiKey) {
  await ElMessageBox.confirm(`重置后旧密钥立即失效，使用该 Key 的${k.category === 'personal' ? '编码工具' : '应用'}需要更新配置。确认重置「${k.name}」？`, '重置密钥', { type: 'warning' })
  const res = await keys.rotate(k.id)
  if (res.claim_required) ElMessage.success(`旧密钥已作废，持有人「${k.owner}」需在「我的 Key」重新领取`)
  else secret.value = res.secret ?? ''
  await load()
}

// ---- allowed models ------------------------------------------------------------------
const modelsVisible = ref(false)
const allowed = ref<string[]>([])
const modelGroups = computed(() => {
  const coding = new Set<string>(), other = new Set<string>()
  for (const m of state.models) if (m.status === 'active') (m.category === 'code' || DEFAULT_CODING_MODELS.includes(m.display_name) ? coding : other).add(m.display_name)
  return [{ label: '编码推荐', names: [...coding].sort() }, { label: '其他模型', names: [...other].filter((n) => !coding.has(n)).sort() }]
})
function openModels(k: ApiKey) { allowed.value = [...(k.allowed_models ?? [])]; modelsVisible.value = true }
async function saveModels() {
  saving.value = true
  try { await keys.allowedModels(target.value!.id, allowed.value); ElMessage.success(allowed.value.length ? '可用模型已更新' : '已取消模型限制'); modelsVisible.value = false; await load() } finally { saving.value = false }
}

// ---- IP whitelist ----------------------------------------------------------------
const ipVisible = ref(false)
const ipText = ref('')
const ipLines = computed(() => ipText.value.split(/[\n,，;；\s]+/).map((x) => x.trim()).filter(Boolean))
const ipInvalid = computed(() => ipLines.value.filter((x) => !isIPOrCIDR(x)))
function openIPs(k: ApiKey) { ipText.value = (k.ip_whitelist ?? []).join('\n'); ipVisible.value = true }
async function saveIPs() {
  saving.value = true
  try { await keys.ipWhitelist(target.value!.id, ipLines.value); ElMessage.success(ipLines.value.length ? 'IP 白名单已更新，立即生效' : '已取消 IP 限制'); ipVisible.value = false; await load() } finally { saving.value = false }
}

const blVisible = ref(false)
const blReason = ref('')
function openBlacklist(k: ApiKey) { target.value = k; blReason.value = ''; blVisible.value = true }
async function blacklist() {
  saving.value = true
  try { await keys.blacklist(target.value!.id, blReason.value.trim()); ElMessage.success('已加入黑名单'); blVisible.value = false; await Promise.all([load(), reload()]) } finally { saving.value = false }
}

const quotaVisible = ref(false)
const qTpm = ref(0)
const qQps = ref(0)
function openQuota(k: ApiKey) { target.value = k; qTpm.value = k.tpm_quota; qQps.value = k.qps_quota; quotaVisible.value = true }
async function saveQuota() {
  saving.value = true
  try { await keys.quota(target.value!.id, qTpm.value, qQps.value); ElMessage.success('配额已更新'); quotaVisible.value = false; await Promise.all([load(), reload()]) } finally { saving.value = false }
}

const historyVisible = ref(false)
const history = ref<AuditLog[]>([])
async function openHistory(k: ApiKey) { history.value = await keys.history(k.id); historyVisible.value = true }
</script>

<style scoped>
.tabs { padding: 0 16px; }
.tabs :deep(.el-tabs__header) { margin: 0; }
.tab-hint { margin-left: 6px; font-size: 11.5px; color: var(--text-3); font-weight: 400; }
.pad { padding: 12px 16px; }
.spacer { flex: 1; }
.chip-link { display: inline-flex; align-items: center; gap: 3px; color: var(--primary); font-size: 12.5px; cursor: default; }
.presets { margin-top: 8px; display: flex; gap: 12px; }
.ip-foot { display: flex; justify-content: space-between; margin-top: 6px; font-size: 12px; }
.ip-foot .bad { color: var(--danger); }
</style>
