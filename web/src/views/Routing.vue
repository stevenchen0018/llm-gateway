<template>
  <div>
    <PageHeader title="模型调度" desc="业务携带模型名调用统一 API，网关经「厂商 / 供应商匹配 → 模型调度 → 供应商服务代理」实时路由；同一厂商的模型可由多家供应商提供，供应商之间自动容灾">
      <el-button v-if="canWrite" type="primary" :icon="Plus" @click="openCreate()">新增调度策略</el-button>
    </PageHeader>

    <Panel title="技术架构" :sub="trace ? '当前展示路由模拟结果' : '路由模拟后将高亮命中的链路'" class="arch">
      <ArchFlow :trace="trace" />
    </Panel>

    <Panel flush>
      <el-tabs v-model="tab" class="tabs">
        <el-tab-pane label="调度策略" name="policies" />
        <el-tab-pane label="供应商调度" name="suppliers" />
        <el-tab-pane label="路由模拟" name="simulate" />
      </el-tabs>

      <!-- ---- supplier dimension: per vendor, how its suppliers share traffic ---- -->
      <div v-if="tab === 'suppliers'" class="supplier-tab">
        <div class="toolbar">
          <el-input v-model="supKw" placeholder="搜索厂商" :prefix-icon="Search" clearable style="width: 200px" />
          <el-checkbox v-model="multiOnly">仅显示多供应商厂商</el-checkbox>
          <span class="spacer" />
          <span class="hint">未命中调度策略时，按厂商的供应商选择方式在其供应商间路由；停用的供应商不承接该厂商的任何流量（策略目标同样跳过）。</span>
        </div>
        <div class="vcards">
          <div v-for="v in supVendors" :key="v.id" class="vcard">
            <div class="vc-head">
              <VendorLogo :code="v.code" :size="26" /><b>{{ v.name }}</b><span class="muted">{{ v.suppliers.length }} 家供应商 · {{ v.model_count }} 个模型</span>
              <span class="spacer" />
              <el-radio-group :model-value="v.routing_strategy" size="small" :disabled="!isSuper" @change="(val: string | number | boolean | undefined) => setVendorRouting(v, String(val))">
                <el-radio-button v-for="(r, k) in VENDOR_ROUTING" :key="k" :value="k">{{ r.text }}</el-radio-button>
              </el-radio-group>
            </div>
            <div class="vc-rows">
              <div v-for="sp in orderedSuppliers(v)" :key="sp.id" class="vc-row" :class="{ off: sp.status === 'disabled' }">
                <span class="rank">{{ sp.status === 'disabled' ? '—' : rankOf(v, sp) }}</span>
                <VendorLogo :code="sp.provider?.code ?? ''" :size="18" />
                <span class="sname">{{ sp.provider?.name }}</span>
                <StatusBadge v-if="sp.provider" :text="SUPPLIER_TYPE[sp.provider.supplier_type]?.text ?? ''" :tone="SUPPLIER_TYPE[sp.provider.supplier_type]?.tone ?? 'info'" />
                <span class="num muted">{{ sp.provider && Number(sp.provider.discount_rate) < 1 ? discountText(sp.provider.discount_rate) : '无折扣' }}</span>
                <span class="spacer" />
                <template v-if="v.routing_strategy === 'supplier_weighted'">
                  <span class="muted">权重</span>
                  <el-input-number :model-value="sp.weight" :min="1" :max="10000" size="small" controls-position="right" style="width: 92px" :disabled="!isSuper" @change="(n: number | undefined) => saveSupply(sp, { weight: n ?? 1 })" />
                  <div class="bar"><i :style="{ width: weightShare(v, sp) + '%' }" /></div><span class="num share">{{ weightShare(v, sp) }}%</span>
                </template>
                <template v-else>
                  <span class="muted">优先级</span>
                  <el-input-number :model-value="sp.priority" :min="0" :max="9999" size="small" controls-position="right" style="width: 92px" :disabled="!isSuper" @change="(n: number | undefined) => saveSupply(sp, { priority: n ?? 0 })" />
                </template>
                <el-switch :model-value="sp.status === 'active'" size="small" :disabled="!isSuper" @change="(on: string | number | boolean) => saveSupply(sp, { status: on ? 'active' : 'disabled' })" />
              </div>
            </div>
          </div>
          <EmptyState v-if="!supVendors.length" text="没有符合条件的厂商" height="140px" />
        </div>
      </div>

      <!-- ---- policies ---- -->
      <template v-if="tab === 'policies'">
        <div class="list-toolbar">
          <el-select v-model="fApp" clearable placeholder="全部应用" style="width: 170px"><el-option v-for="a in state.apps" :key="a.id" :label="a.name" :value="a.id" /></el-select>
          <KeySelect v-model="fKey" placeholder="全部 API Key" width="200px" :special="{ value: 0, label: '全部 Key（全局策略）' }" />
          <el-input v-model="keyword" placeholder="搜索策略名称 / 源模型" :prefix-icon="Search" clearable style="width: 220px" />
          <span class="spacer" />
          <span class="muted">{{ filtered.length }} 条策略</span>
        </div>
        <el-table :data="lp.rows.value" v-loading="loading" empty-text="暂无调度策略">
          <el-table-column label="策略名称" min-width="150">
            <template #default="{ row }"><div class="cell-title">{{ row.name || row.alias }}</div><div class="cell-sub clamp2" :title="row.remark">{{ row.remark || '—' }}</div></template>
          </el-table-column>
          <el-table-column label="应用" width="120" show-overflow-tooltip><template #default="{ row }">{{ row.app_id ? appName(row.app_id) : '—' }}</template></el-table-column>
          <el-table-column label="API Key" width="130" show-overflow-tooltip><template #default="{ row }"><StatusBadge v-if="!row.api_key_id" text="全部 Key" tone="primary" /><span v-else>{{ keyName(row.api_key_id) }}</span></template></el-table-column>
          <el-table-column label="源模型" min-width="130">
            <template #default="{ row }">
              <StatusBadge :text="row.source_type === 'vendor' ? '厂商模型' : '自定义'" :tone="row.source_type === 'vendor' ? 'accent' : 'info'" />
              <div class="cell-sub mono" style="margin-top: 3px">{{ row.source_type === 'vendor' && row.source_vendor_id ? vendorName(row.source_vendor_id) + ' / ' : '' }}{{ row.alias }}</div>
            </template>
          </el-table-column>
          <el-table-column label="目标（供应商 / 模型，按顺序尝试）" min-width="280">
            <template #default="{ row }">
              <div class="targets">
                <span v-for="(r, i) in row.routes" :key="r.id" class="tgt" :class="{ off: !r.enabled || supplyOff(r.candidate_model_id) }" :title="supplyOff(r.candidate_model_id) ? '该供应商已对此厂商停用，路由时跳过' : ''"><b>{{ i + 1 }}</b><VendorLogo :code="providerCode(r.candidate_model_id)" :size="16" /><span class="tgt-text" :title="`${supplierOf(r.candidate_model_id)} / ${modelName(r.candidate_model_id)}`">{{ supplierOf(r.candidate_model_id) }}<span class="muted"> / </span><span class="mono">{{ modelName(r.candidate_model_id) }}</span></span></span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="策略" width="124"><template #default="{ row }"><StatusBadge :text="STRATEGY_TEXT[row.strategy as Strategy]" tone="accent" /></template></el-table-column>
          <el-table-column label="状态" width="80"><template #default="{ row }"><StatusBadge :text="row.enabled ? '启用' : '停用'" :tone="row.enabled ? 'success' : 'info'" /></template></el-table-column>
          <el-table-column label="操作" width="180" align="right" fixed="right" class-name="col-actions">
            <template #default="{ row }">
              <el-button link type="primary" @click="simulateFor(row)">模拟</el-button>
              <template v-if="row.api_key_id ? canWrite : isSuper">
              <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button link :type="row.enabled ? 'danger' : 'primary'" @click="toggleGroup(row, !row.enabled)">{{ row.enabled ? '停用' : '启用' }}</el-button>
              <el-popconfirm title="删除该策略及其全部目标？" @confirm="removeGroup(row)"><template #reference><el-button link type="danger">删除</el-button></template></el-popconfirm>
              </template>
            </template>
          </el-table-column>
        </el-table>
        <TablePager v-model:page="lp.page.value" v-model:page-size="lp.pageSize.value" :total="lp.total.value" />
      </template>

      <!-- ---- simulator ---- -->
      <div v-else-if="tab === 'simulate'" class="sim">
        <div class="toolbar">
          <KeySelect v-model="simKey" status="active" placeholder="API Key（不选则仅匹配全局策略）" width="260px" />
          <el-autocomplete v-model="simModel" :fetch-suggestions="suggest" placeholder="请求的模型名，如 deepseek-r1 或 aliyun/deepseek-r1" style="width: 360px" clearable @keyup.enter="runSim" />
          <el-button type="primary" :loading="simLoading" :disabled="!simModel.trim()" @click="runSim">模拟路由</el-button>
        </div>
        <p class="hint">模拟与真实请求使用同一套解析逻辑：先按模型名匹配厂商与供应商（<code>供应商编码/模型</code> 可指定供应商，如 <code>aliyun/deepseek-v3</code>），再取 Key 专属策略、全局策略，都没有时按厂商的供应商选择方式排序。<template v-if="trace?.pin_kind">本次请求已指定{{ trace.pin_kind === 'supplier' ? '供应商' : '厂商' }}「{{ trace.vendor }}」。</template></p>

        <el-alert v-if="trace && !resolved" type="error" :closable="false" show-icon title="无法路由" description="没有匹配到任何已上架模型或可用的调度目标；网关将返回「未配置路由」。" style="margin-bottom: 12px" />
        <el-table v-if="trace?.candidates?.length" :data="trace.candidates" size="small">
          <el-table-column label="顺序" width="70"><template #default="{ $index }"><span class="num">{{ $index + 1 }}</span></template></el-table-column>
          <el-table-column label="厂商" width="110"><template #default="{ row }">{{ row.model.vendor?.name ?? '—' }}</template></el-table-column>
          <el-table-column label="供应商" min-width="150"><template #default="{ row }"><div class="mcell"><VendorLogo :code="row.model.provider.code" :size="20" />{{ row.model.provider.name }}</div></template></el-table-column>
          <el-table-column label="模型" min-width="150"><template #default="{ row }"><span class="mono">{{ row.model.model_key }}</span></template></el-table-column>
          <el-table-column label="供应商优先级 / 权重" width="130" align="right"><template #default="{ row }"><span class="num">{{ row.model.supply ? `${row.model.supply.priority} / ${row.model.supply.weight}` : '—' }}</span></template></el-table-column>
          <el-table-column label="折扣" width="90" align="right"><template #default="{ row }"><span class="num">{{ discountText(row.model.provider.discount_rate) }}</span></template></el-table-column>
          <el-table-column label="折后均价 / 1K" width="140" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(effective(row.model)) }}</span></template></el-table-column>
          <el-table-column label="来源" min-width="200" show-overflow-tooltip><template #default="{ row }">{{ row.route.name || (trace?.scope === 'direct' ? `厂商供应商选择：${STRATEGY_TEXT[trace.strategy]}` : row.route.alias) }}</template></el-table-column>
        </el-table>
      </div>
    </Panel>

    <!-- ---- create / edit dialog (matches the console's policy form) ---- -->
    <el-dialog v-model="visible" :title="editing ? '编辑' : '新增调度策略'" width="680px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="96px" label-position="right">
        <el-form-item label="应用"><el-select v-model="form.app_id" clearable placeholder="选择应用" style="width: 100%"><el-option v-for="a in state.apps" :key="a.id" :label="a.name" :value="a.id" /></el-select></el-form-item>
        <el-form-item label="策略名称" prop="name"><el-input v-model="form.name" clearable placeholder="如 告警智能分析&模板索引优化_DeepSeek-R1" /></el-form-item>
        <el-form-item label="API Key" prop="api_key_id">
          <el-select v-if="editing" :model-value="form.api_key_id ? keyName(form.api_key_id) : '全部 Key（全局策略）'" disabled style="width: 100%" />
          <KeySelect v-else v-model="form.api_key_id" status="active" placeholder="搜索并选择 API Key" width="100%" :clearable="false" :special="isSuper ? { value: 0, label: '全部 Key（全局策略）' } : undefined" />
        </el-form-item>
        <el-form-item label="源模型" prop="alias">
          <div class="src">
            <el-select v-model="form.source_type" style="width: 128px"><el-option label="自定义" value="custom" /><el-option label="厂商模型" value="vendor" /></el-select>
            <el-select v-if="form.source_type === 'vendor'" v-model="form.source_vendor_id" filterable placeholder="厂商" style="width: 150px"><el-option v-for="v in state.vendors" :key="v.id" :label="v.name" :value="v.id" /></el-select>
            <el-select v-model="form.alias" filterable allow-create default-first-option placeholder="源模型名称（业务调用时传入的 model）" style="flex: 1">
              <el-option v-for="n in sourceNames" :key="n" :label="n" :value="n" />
            </el-select>
          </div>
        </el-form-item>
        <el-form-item label="目标模型" required>
          <div class="targets-edit">
            <div v-for="(t, i) in form.targets" :key="i" class="trow">
              <span class="ord">{{ i + 1 }}</span>
              <el-select v-model="t.provider_id" filterable placeholder="供应商" style="width: 160px" @change="t.model_id = undefined"><el-option v-for="p in state.providers" :key="p.id" :label="p.name" :value="p.id" /></el-select>
              <el-select v-model="t.model_id" filterable placeholder="模型" style="flex: 1"><el-option v-for="m in modelsOf(t.provider_id)" :key="m.id" :label="m.display_name" :value="m.id" /></el-select>
              <el-input-number v-if="form.strategy === 'round_robin'" v-model="t.weight" :min="1" :max="100" controls-position="right" style="width: 90px" title="权重" />
              <el-button v-if="form.targets.length > 1" link type="danger" :icon="Delete" @click="form.targets.splice(i, 1)" />
            </div>
            <div class="tactions">
              <el-button link type="primary" :icon="Plus" @click="form.targets.push({ provider_id: undefined, model_id: undefined, weight: 1 })">添加备用目标</el-button>
              <el-button link type="primary" :disabled="!form.alias" @click="fillSuppliers">按供应商填充「{{ form.alias || '源模型' }}」的全部供应商</el-button>
            </div>
          </div>
        </el-form-item>
        <el-form-item v-if="form.targets.length > 1" label="调度策略">
          <el-select v-model="form.strategy" style="width: 100%"><el-option v-for="(t, k) in STRATEGY_TEXT" :key="k" :label="t" :value="k" /></el-select>
          <div class="hint" style="width: 100%">{{ STRATEGY_HINT[form.strategy] }}</div>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="3" resize="none" placeholder="策略说明，如降本原因、适用场景" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">提交</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TablePager from '../components/TablePager.vue'
import { useLocalPage } from '../composables/usePaged'
import KeySelect from '../components/KeySelect.vue'
import { useAuth } from '../composables/useAuth'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance } from 'element-plus'
import { Delete, Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import StatusBadge from '../components/StatusBadge.vue'
import VendorLogo from '../components/VendorLogo.vue'
import ArchFlow from '../components/model/ArchFlow.vue'
import EmptyState from '../components/EmptyState.vue'
import { routes as routesApi, vendors as vendorsApi } from '../api'
import { STRATEGY_HINT, STRATEGY_TEXT, SUPPLIER_TYPE, VENDOR_ROUTING } from '../constants'
import { useLookups } from '../composables/useLookups'
import { fmtMoney } from '../utils'
import type { Model, Route, RouteTrace, Strategy, VendorSupplier, VendorView } from '../types'

const { isSuper, canWrite } = useAuth()

interface Group {
  gkey: string; name: string; alias: string; source_type: 'custom' | 'vendor'; source_vendor_id?: number | null
  app_id?: number | null; api_key_id?: number | null; strategy: Strategy; remark: string; enabled: boolean; routes: Route[]
}

const route = useRoute()
const router = useRouter()
const { state, keyName, appName, vendorName, modelName, modelById, reload } = useLookups()

const tab = ref('policies')
const loading = ref(false)
const rows = ref<Route[]>([])
const fApp = ref<number>()
const fKey = ref<number>()
const keyword = ref('')

async function load() {
  loading.value = true
  try { rows.value = await routesApi.list() } finally { loading.value = false }
}

const groups = computed<Group[]>(() => {
  const m = new Map<string, Route[]>()
  for (const r of rows.value) {
    const k = `${r.api_key_id ?? 0}|${r.alias.toLowerCase()}|${r.name}`
    m.set(k, [...(m.get(k) ?? []), r])
  }
  return [...m].map(([gkey, rs]) => {
    const sorted = [...rs].sort((a, b) => a.priority - b.priority || a.id - b.id)
    const h = sorted[0]
    return { gkey, name: h.name, alias: h.alias, source_type: h.source_type, source_vendor_id: h.source_vendor_id, app_id: h.app_id, api_key_id: h.api_key_id,
      strategy: h.strategy, remark: h.remark, enabled: sorted.some((r) => r.enabled), routes: sorted }
  }).sort((a, b) => (a.api_key_id ? 0 : 1) - (b.api_key_id ? 0 : 1) || a.name.localeCompare(b.name))
})

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  return groups.value.filter((g) =>
    (!fApp.value || g.app_id === fApp.value) &&
    (fKey.value === undefined || fKey.value === null || (fKey.value === 0 ? !g.api_key_id : g.api_key_id === fKey.value)) &&
    (!k || `${g.name} ${g.alias} ${g.remark}`.toLowerCase().includes(k)))
})

// policies are grouped client-side from the route rows, so the groups page locally
const lp = useLocalPage(filtered)

const providerCode = (modelId: number) => modelById.value.get(modelId)?.provider?.code ?? ''
const supplierOf = (modelId: number) => modelById.value.get(modelId)?.provider?.name ?? ''
const supplyOff = (modelId: number) => modelById.value.get(modelId)?.supply?.status === 'disabled'
const modelsOf = (providerId?: number) => state.models.filter((m) => m.provider_id === providerId && m.status === 'active')
const sourceNames = computed(() => [...new Set([...state.models.map((m) => m.model_key), ...rows.value.map((r) => r.alias)])].sort())

// ---- form -----------------------------------------------------------------------
interface Target { routeId?: number; provider_id?: number; model_id?: number; weight: number }
const visible = ref(false)
const saving = ref(false)
const editing = ref<Group | null>(null)
const formRef = ref<FormInstance>()
const blank = () => ({
  app_id: undefined as number | undefined, name: '', api_key_id: undefined as number | undefined,
  source_type: 'custom' as 'custom' | 'vendor', source_vendor_id: undefined as number | undefined, alias: '',
  targets: [{ provider_id: undefined, model_id: undefined, weight: 1 }] as Target[],
  strategy: 'priority' as Strategy, remark: '',
})
const form = reactive(blank())
const rules = {
  name: [{ required: true, message: '请输入策略名称', trigger: 'blur' }],
  api_key_id: [{ required: true, message: '请选择 API Key', trigger: 'change' }],
  alias: [{ required: true, message: '请填写源模型', trigger: 'change' }],
}

function openCreate(prefill?: { alias?: string; targetModelId?: number }) {
  editing.value = null
  Object.assign(form, blank())
  if (prefill?.alias) form.alias = prefill.alias
  if (prefill?.targetModelId) {
    const m = modelById.value.get(prefill.targetModelId)
    if (m) form.targets = [{ provider_id: m.provider_id, model_id: m.id, weight: 1 }]
  }
  visible.value = true
}

function openEdit(g: Group) {
  editing.value = g
  Object.assign(form, {
    app_id: g.app_id ?? undefined, name: g.name, api_key_id: g.api_key_id ?? 0, source_type: g.source_type,
    source_vendor_id: g.source_vendor_id ?? undefined, alias: g.alias, strategy: g.strategy, remark: g.remark,
    targets: g.routes.map((r) => ({ routeId: r.id, provider_id: modelById.value.get(r.candidate_model_id)?.provider_id, model_id: r.candidate_model_id, weight: r.weight })),
  })
  visible.value = true
}

async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  if (form.targets.some((t) => !t.model_id)) { ElMessage.warning('请为每个目标选择模型'); return }
  if (form.source_type === 'vendor' && !form.source_vendor_id) { ElMessage.warning('请选择源厂商'); return }
  saving.value = true
  const common = () => ({
    name: form.name, app_id: form.app_id, source_type: form.source_type,
    source_vendor_id: form.source_type === 'vendor' ? form.source_vendor_id : undefined,
    alias: form.alias.trim(), strategy: form.targets.length > 1 ? form.strategy : 'priority', remark: form.remark,
  })
  try {
    if (editing.value) {
      const keep = new Set(form.targets.filter((t) => t.routeId).map((t) => t.routeId))
      for (const r of editing.value.routes) if (!keep.has(r.id)) await routesApi.remove(r.id)
      for (const [i, t] of form.targets.entries()) {
        const body = { ...common(), candidate_model_id: t.model_id, priority: i, weight: t.weight }
        if (t.routeId) await routesApi.update(t.routeId, body)
        else await routesApi.create({ ...body, api_key_id: editing.value.api_key_id ?? undefined })
      }
    } else {
      for (const [i, t] of form.targets.entries()) {
        await routesApi.create({ ...common(), candidate_model_id: t.model_id, priority: i, weight: t.weight, api_key_id: form.api_key_id || undefined })
      }
    }
    ElMessage.success('已保存'); visible.value = false; await load()
  } finally { saving.value = false }
}

async function toggleGroup(g: Group, on: boolean) {
  for (const r of g.routes) await routesApi.update(r.id, { enabled: on })
  ElMessage.success(on ? '已启用' : '已停用'); await load()
}
async function removeGroup(g: Group) {
  for (const r of g.routes) await routesApi.remove(r.id)
  ElMessage.success('已删除'); await load()
}

// fill targets with every supplier offering the source model, in supplier priority order
function fillSuppliers() {
  const name = form.alias.trim().toLowerCase()
  const offers = state.models.filter((m) => m.status === 'active' && (m.display_name.toLowerCase() === name || m.model_key.toLowerCase() === name) &&
    (form.source_type !== 'vendor' || !form.source_vendor_id || m.vendor_id === form.source_vendor_id) && m.supply?.status !== 'disabled')
  if (!offers.length) { ElMessage.warning(`没有找到提供「${form.alias}」的供应商`); return }
  offers.sort((a, b) => (a.supply?.priority ?? 100) - (b.supply?.priority ?? 100))
  form.targets = offers.map((m) => ({ provider_id: m.provider_id, model_id: m.id, weight: m.supply?.weight ?? 1 }))
  if (form.targets.length > 1 && form.strategy === 'priority') form.strategy = 'supplier_priority'
}

// ---- supplier dimension tab --------------------------------------------------------------
const supKw = ref('')
const multiOnly = ref(true)
const supVendors = computed(() => state.vendors.filter((v) => (!multiOnly.value || v.suppliers.length > 1) &&
  (!supKw.value.trim() || `${v.name} ${v.code}`.toLowerCase().includes(supKw.value.trim().toLowerCase()))))
function orderedSuppliers(v: VendorView) {
  const key = (s: VendorView['suppliers'][number]) => (s.status === 'disabled' ? 1e9 : v.routing_strategy === 'supplier_weighted' ? -s.weight : v.routing_strategy === 'cost_first' ? Number(s.provider?.discount_rate ?? 1) : s.priority)
  return [...v.suppliers].sort((a, b) => key(a) - key(b))
}
const rankOf = (v: VendorView, s: VendorView['suppliers'][number]) => orderedSuppliers(v).filter((x) => x.status !== 'disabled').indexOf(s) + 1
function weightShare(v: VendorView, s: VendorView['suppliers'][number]) {
  if (s.status === 'disabled') return 0
  const total = v.suppliers.filter((x) => x.status === 'active').reduce((a, x) => a + x.weight, 0)
  return total ? Math.round((s.weight / total) * 100) : 0
}
async function saveSupply(s: VendorSupplier, patch: Partial<VendorSupplier>) {
  await vendorsApi.updateSupplier(s.id, patch)
  ElMessage.success(patch.status === 'disabled' ? '已停用：该供应商不再承接此厂商的流量' : '已更新，立即生效')
  await reload()
}
async function setVendorRouting(v: VendorView, strategy: string) {
  await vendorsApi.update(v.id, { name: v.name, description: v.description, website: v.website, status: v.status, routing_strategy: strategy })
  ElMessage.success(`${v.name} 的供应商选择方式已改为「${VENDOR_ROUTING[strategy].text}」`)
  await reload()
}

// ---- simulator ------------------------------------------------------------------------
const simKey = ref<number>()
const simModel = ref('')
const simLoading = ref(false)
const trace = ref<RouteTrace | null>(null)
const resolved = ref(true)

const suggest = (q: string, cb: (r: { value: string }[]) => void) => {
  const all = [...new Set([...state.models.map((m) => m.model_key), ...rows.value.map((r) => r.alias)])]
  cb(all.filter((n) => n.toLowerCase().includes(q.toLowerCase())).slice(0, 15).map((value) => ({ value })))
}

async function runSim() {
  simLoading.value = true
  try {
    const res = await routesApi.simulate({ api_key_id: simKey.value ?? 0, model: simModel.value.trim() })
    trace.value = res.trace
    resolved.value = res.resolved
  } finally { simLoading.value = false }
}

function simulateFor(g: Group) {
  tab.value = 'simulate'
  simKey.value = g.api_key_id ?? undefined
  simModel.value = g.alias
  runSim()
}

const rateOf = (v: string) => Number(v) || 1
const discountText = (v: string) => (rateOf(v) >= 1 ? '无' : `${(rateOf(v) * 10).toFixed(1).replace(/\.0$/, '')} 折`)
const effective = (m: Model) => (Number(m.input_price_per_1k) + Number(m.output_price_per_1k)) * rateOf(m.provider.discount_rate)

onMounted(async () => {
  await Promise.all([load(), reload()])
  // arriving from the model-switch suggestions: /routing?new=1&source=..&target=<model id>
  if (route.query.new) {
    openCreate({ alias: String(route.query.source ?? ''), targetModelId: Number(route.query.target) || undefined })
    router.replace({ path: '/routing' })
  }
})
</script>

<style scoped>
.supplier-tab { padding: 12px 16px 16px; }
.supplier-tab .toolbar { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; flex-wrap: wrap; }
.supplier-tab .toolbar .hint { max-width: 560px; }
.vcards { display: grid; grid-template-columns: repeat(auto-fill, minmax(520px, 1fr)); gap: 12px; }
.vcard { border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.vc-head { display: flex; align-items: center; gap: 8px; padding: 10px 12px; background: #FAFBFC; border-bottom: 1px solid var(--border-soft); }
.vc-rows { display: grid; }
.vc-row { display: flex; align-items: center; gap: 8px; padding: 7px 12px; border-bottom: 1px solid var(--border-soft); font-size: 13px; }
.vc-row:last-child { border-bottom: none; }
.vc-row.off { opacity: .5; }
.vc-row .rank { width: 18px; height: 18px; border-radius: 50%; background: var(--primary-soft); color: var(--primary); font-size: 11px; font-weight: 600; display: inline-flex; align-items: center; justify-content: center; }
.vc-row .sname { min-width: 110px; }
.vc-row .bar { width: 70px; height: 6px; border-radius: 3px; background: #EEF0F3; overflow: hidden; }
.vc-row .bar i { display: block; height: 100%; background: var(--accent); }
.vc-row .share { width: 36px; text-align: right; }
.tactions { display: flex; gap: 16px; flex-wrap: wrap; }
.arch { margin-bottom: 16px; }
.tabs { padding: 0 16px; }
.tabs :deep(.el-tabs__header) { margin: 0; }
.tabs :deep(.el-tabs__nav-wrap::after) { height: 1px; background: var(--border-soft); }
.pad { padding: 12px 16px; }
.spacer { flex: 1; }
.targets { display: flex; flex-direction: column; gap: 4px; }
.tgt { display: flex; align-items: center; gap: 6px; font-size: 12.5px; min-width: 0; white-space: nowrap; }
.tgt > b, .tgt > .logo { flex: none; }
.tgt-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.clamp2 { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.tgt b { display: inline-flex; align-items: center; justify-content: center; width: 16px; height: 16px; border-radius: 50%; background: var(--accent-soft); color: var(--accent); font-size: 10px; font-weight: 600; }
.tgt.off { opacity: .45; text-decoration: line-through; }
.src { display: flex; gap: 8px; width: 100%; }
.targets-edit { width: 100%; display: grid; gap: 8px; }
.trow { display: flex; align-items: center; gap: 8px; }
.ord { flex: none; width: 18px; height: 18px; border-radius: 50%; background: var(--accent-soft); color: var(--accent); font-size: 11px; font-weight: 600; display: inline-flex; align-items: center; justify-content: center; }
.sim { padding: 16px; }
:deep(.el-table .cell .el-button + .el-button) { margin-left: 8px; }
:deep(.el-table__fixed-right .cell) { white-space: nowrap; }
.mcell { display: flex; align-items: center; gap: 8px; }
</style>
