<template>
  <div>
    <PageHeader title="模型市场" desc="集中纳管内外部模型：按厂商、类型、上下文与标签筛选，查看详情、获取接入说明或立即体验">
      <TransferBar name="models" :filters="{ category: f.category, q: keyword }" @imported="onModelsImported" />
      <el-input v-model="keyword" placeholder="搜索模型名称 / 描述" :prefix-icon="Search" clearable style="width: 240px" />
      <el-button v-if="canPlatform" type="primary" :icon="Plus" @click="openCreate">新增模型</el-button>
    </PageHeader>

    <Panel class="filters">
      <VendorFilter v-model="f.vendors" label="模型厂商" :items="state.vendors" :counts="countByVendor" />
      <VendorFilter v-model="f.providers" label="供应商" :items="state.providers" :counts="countByProvider" :top-n="6" />
      <FilterRow v-model="f.category" label="模型类型" :options="categoryOptions" />
      <FilterRow v-model="f.ctx" label="上下文长度" :options="CONTEXT_BUCKETS.map((b) => ({ value: b.value, label: b.label }))" />
      <FilterRow v-model="f.tag" label="标签" :options="tagOptions" />
    </Panel>

    <div class="bar">
      <span class="muted">共 <b class="num">{{ filtered.length }}</b> 个模型<template v-if="f.vendors.length">，{{ f.vendors.length }} 家厂商</template><template v-if="f.providers.length">，{{ f.providers.length }} 家供应商</template></span>
      <span class="bar-r">
        <el-select v-model="sortBy" size="small" style="width: 130px"><el-option v-for="o in SORTS" :key="o.value" :label="o.label" :value="o.value" /></el-select>
        <el-radio-group v-model="groupByVendor" size="small"><el-radio-button :value="false">平铺</el-radio-button><el-radio-button :value="true">按厂商分组</el-radio-button></el-radio-group>
      </span>
    </div>

    <template v-if="!groupByVendor">
      <div v-loading="loading" class="grid">
        <ModelCard v-for="m in paged" :key="m.id" :model="m" :mine="mineIds.has(m.id)" :supplier-count="offerCount(m)"
          @detail="openDetail" @guide="openGuide" @try="tryModel" @toggle-mine="toggleMine" />
      </div>
      <div v-if="filtered.length > pageSize" class="pager">
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :page-sizes="[12, 24, 48]" :total="filtered.length" layout="total, sizes, prev, pager, next" background />
      </div>
    </template>
    <div v-else v-loading="loading" class="vgroups">
      <section v-for="g in vendorGroups" :key="g.id" class="vgroup">
        <header>
          <VendorLogo :code="g.code" :size="22" /><b>{{ g.name }}</b><span class="muted num">{{ g.models.length }} 个模型</span>
          <el-button v-if="g.models.length > GROUP_PREVIEW" link type="primary" size="small" @click="toggleGroup(g.id)">{{ expanded.has(g.id) ? '收起' : `展开全部 ${g.models.length}` }}</el-button>
        </header>
        <div class="grid">
          <ModelCard v-for="m in expanded.has(g.id) ? g.models : g.models.slice(0, GROUP_PREVIEW)" :key="m.id" :model="m" :mine="mineIds.has(m.id)" :supplier-count="offerCount(m)"
            @detail="openDetail" @guide="openGuide" @try="tryModel" @toggle-mine="toggleMine" />
        </div>
      </section>
    </div>
    <EmptyState v-if="!loading && !filtered.length" text="没有符合条件的模型" hint="调整筛选条件，或点击右上角新增模型" />

    <ModelDetailDrawer v-model:visible="detailVisible" :model="current" :mine="!!current && mineIds.has(current.id)" :manage="canPlatform" :offers="current ? offersOf(current) : []" @select="openDetail"
      @edit="openEdit" @guide="openGuide" @try="tryModel" @toggle-mine="toggleMine" @toggle-status="toggleStatus" />
    <ModelGuideDialog v-model:visible="guideVisible" :model="guideModel" />

    <el-dialog v-model="formVisible" :title="editing ? '编辑模型' : '新增模型'" width="640px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid">
          <el-form-item label="厂商（模型研发方）" prop="vendor_id"><el-select v-model="form.vendor_id" filterable placeholder="选择厂商"><el-option v-for="v in state.vendors" :key="v.id" :label="v.name" :value="v.id" /></el-select></el-form-item>
          <el-form-item label="供应商（调用渠道）" prop="provider_id">
            <el-select v-model="form.provider_id" :disabled="!!editing" filterable placeholder="选择供应商">
              <el-option-group v-for="g in supplierOptions" :key="g.label" :label="g.label"><el-option v-for="p in g.items" :key="p.id" :label="p.name" :value="p.id" /></el-option-group>
            </el-select>
          </el-form-item>
          <el-form-item label="模型类型" prop="category"><el-select v-model="form.category"><el-option v-for="c in CATEGORIES" :key="c.value" :label="c.label" :value="c.value" /></el-select></el-form-item>
          <el-form-item label="厂商模型名" prop="model_key"><el-input v-model="form.model_key" :disabled="!!editing" placeholder="如 qwen-max" /></el-form-item>
          <el-form-item label="展示名称" prop="display_name"><el-input v-model="form.display_name" /></el-form-item>
          <el-form-item label="调用形态"><el-radio-group v-model="form.type" :disabled="!!editing"><el-radio value="chat">对话 / 生成</el-radio><el-radio value="embedding">向量 / 重排</el-radio></el-radio-group></el-form-item>
          <el-form-item label="上下文长度（K）"><el-input-number v-model="form.context_k" :min="0" :step="8" controls-position="right" /></el-form-item>
          <el-form-item label="标签" class="span2"><el-select v-model="form.tags" multiple filterable allow-create default-first-option placeholder="输入后回车添加，如 深度思考 / 开源" /></el-form-item>
          <el-form-item label="模型描述" class="span2"><el-input v-model="form.description" type="textarea" :rows="3" resize="none" /></el-form-item>
          <el-form-item label="输入价 / 1K"><el-input-number v-model="form.input" :min="0" :step="0.001" :precision="6" controls-position="right" /></el-form-item>
          <el-form-item label="输出价 / 1K"><el-input-number v-model="form.output" :min="0" :step="0.001" :precision="6" controls-position="right" /></el-form-item>
          <el-form-item label="TPM 上限"><el-input-number v-model="form.tpm_limit" :min="0" :step="1000" controls-position="right" /></el-form-item>
          <el-form-item label="QPS 上限"><el-input-number v-model="form.qps_limit" :min="0" controls-position="right" /></el-form-item>
          <el-form-item label="发布时间"><el-date-picker v-model="form.released_at" type="date" value-format="YYYY-MM-DD" placeholder="选择日期" style="width: 100%" /></el-form-item>
          <div class="hint" style="align-self: end; margin-bottom: 20px">0 表示不限；模型自身的低限流阈值与 Key 配额取较小值生效。</div>
        </div>
      </el-form>
      <template #footer><el-button @click="formVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../components/transfer/TransferBar.vue'
import { useAuth } from '../composables/useAuth'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, type FormInstance } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../components/PageHeader.vue'
import Panel from '../components/Panel.vue'
import FilterRow from '../components/FilterRow.vue'
import VendorFilter from '../components/VendorFilter.vue'
import VendorLogo from '../components/VendorLogo.vue'
import EmptyState from '../components/EmptyState.vue'
import ModelCard from '../components/model/ModelCard.vue'
import ModelDetailDrawer from '../components/model/ModelDetailDrawer.vue'
import ModelGuideDialog from '../components/model/ModelGuideDialog.vue'
import { models } from '../api'
import { CATEGORIES, CONTEXT_BUCKETS, categoryLabel, playgroundPath } from '../constants'
import { useLookups } from '../composables/useLookups'
import type { Model } from '../types'

const { canPlatform } = useAuth()

const router = useRouter()
const { state, reload } = useLookups()

const loading = ref(false)
const all = ref<Model[]>([])
const mineIds = ref(new Set<number>())
const keyword = ref('')
const f = reactive({ vendors: [] as number[], providers: [] as number[], category: '', ctx: '', tag: '' })
const page = ref(1)
const pageSize = ref(12)
const SORTS = [
  { value: 'default', label: '默认排序' },
  { value: 'newest', label: '最新发布' },
  { value: 'price', label: '输入价从低到高' },
  { value: 'context', label: '上下文从长到短' },
  { value: 'name', label: '名称 A→Z' },
]
const sortBy = ref('default')
const groupByVendor = ref(false)
const GROUP_PREVIEW = 4
const expanded = ref(new Set<number>())
function toggleGroup(id: number) {
  const s = new Set(expanded.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expanded.value = s
}

async function load() {
  loading.value = true
  try {
    const [m, mine] = await Promise.all([models.list(), models.mine()])
    all.value = m
    mineIds.value = new Set(mine.map((x) => x.id))
  } finally { loading.value = false }
}
onMounted(load)

const countByVendor = computed(() => {
  const c = new Map<number, number>()
  for (const m of all.value) if (m.vendor_id) c.set(m.vendor_id, (c.get(m.vendor_id) ?? 0) + 1)
  return c
})
// the same model (by name) offered by several suppliers
const offersByName = computed(() => {
  const by = new Map<string, Model[]>()
  for (const m of all.value) by.set(m.display_name, [...(by.get(m.display_name) ?? []), m])
  return by
})
const offersOf = (m: Model) => offersByName.value.get(m.display_name) ?? [m]
const offerCount = (m: Model) => offersOf(m).length
const countByProvider = computed(() => {
  const c = new Map<number, number>()
  for (const m of all.value) c.set(m.provider_id, (c.get(m.provider_id) ?? 0) + 1)
  return c
})
const categoryOptions = computed(() => {
  const present = new Set(all.value.map((m) => m.category))
  return [{ value: '', label: '全部' }, ...CATEGORIES.filter((c) => present.has(c.value)).map((c) => ({ value: c.value, label: c.label }))]
})
const tagOptions = computed(() => {
  const count = new Map<string, number>()
  for (const m of all.value) for (const t of m.tags) count.set(t, (count.get(t) ?? 0) + 1)
  const top = [...count].sort((a, b) => b[1] - a[1]).slice(0, 10).map(([t]) => ({ value: t, label: t }))
  return [{ value: '', label: '全部' }, ...top]
})

const filtered = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  const bucket = CONTEXT_BUCKETS.find((b) => b.value === f.ctx)!
  const list = all.value.filter((m) =>
    (!f.vendors.length || (!!m.vendor_id && f.vendors.includes(m.vendor_id))) &&
    (!f.providers.length || f.providers.includes(m.provider_id)) &&
    (!f.category || m.category === f.category) &&
    bucket.test(m.context_length) &&
    (!f.tag || m.tags.includes(f.tag)) &&
    (!k || `${m.display_name} ${m.model_key} ${m.description} ${m.provider?.name ?? ''} ${m.vendor?.name ?? ''}`.toLowerCase().includes(k)),
  )
  const by: Record<string, (a: Model, b: Model) => number> = {
    newest: (a, b) => (b.released_at ?? '').localeCompare(a.released_at ?? ''),
    price: (a, b) => Number(a.input_price_per_1k) - Number(b.input_price_per_1k),
    context: (a, b) => b.context_length - a.context_length,
    name: (a, b) => a.display_name.localeCompare(b.display_name),
  }
  return by[sortBy.value] ? [...list].sort(by[sortBy.value]) : list
})
watch([filtered, pageSize], () => { page.value = 1 })
const paged = computed(() => filtered.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const vendorGroups = computed(() => {
  const by = new Map<number, Model[]>()
  for (const m of filtered.value) by.set(m.vendor_id ?? 0, [...(by.get(m.vendor_id ?? 0) ?? []), m])
  return [...by].map(([id, models]) => {
    const v = state.vendors.find((x) => x.id === id)
    return { id, code: v?.code ?? '', name: v?.name ?? '未归属厂商', models }
  }).sort((a, b) => b.models.length - a.models.length)
})

async function onModelsImported() { await load(); await reload() }

// ---- detail / guide / actions ---------------------------------------------------
const detailVisible = ref(false)
const current = ref<Model | null>(null)
function openDetail(m: Model) { current.value = m; detailVisible.value = true }

const guideVisible = ref(false)
const guideModel = ref<Model | null>(null)
function openGuide(m: Model) { guideModel.value = m; guideVisible.value = true }

function tryModel(m: Model) { router.push({ path: playgroundPath(m.category), query: { model: m.id } }) }

async function toggleMine(m: Model) {
  if (mineIds.value.has(m.id)) { await models.removeMine(m.id); mineIds.value.delete(m.id); ElMessage.success('已移出我的模型') }
  else { await models.addMine(m.id); mineIds.value.add(m.id); ElMessage.success('已加入我的模型') }
  mineIds.value = new Set(mineIds.value)
}

async function toggleStatus(m: Model) {
  const on = m.status !== 'active'
  const updated = await models.update(m.id, { status: on ? 'active' : 'disabled' })
  ElMessage.success(on ? '已上架' : '已下架')
  await load(); await reload()
  current.value = all.value.find((x) => x.id === updated.id) ?? null
}

// ---- create / edit ------------------------------------------------------------------
const formVisible = ref(false)
const saving = ref(false)
const editing = ref<Model | null>(null)
const formRef = ref<FormInstance>()
const blank = () => ({ vendor_id: undefined as number | undefined, provider_id: undefined as number | undefined, model_key: '', display_name: '', type: 'chat', category: 'text', context_k: 32, tags: [] as string[], description: '', input: 0, output: 0, tpm_limit: 0, qps_limit: 0, released_at: '' as string | null })
const form = reactive(blank())
const rules = {
  vendor_id: [{ required: true, message: '请选择厂商', trigger: 'change' }],
  provider_id: [{ required: true, message: '请选择供应商', trigger: 'change' }],
  model_key: [{ required: true, message: '请输入厂商模型名', trigger: 'blur' }],
  display_name: [{ required: true, message: '请输入展示名称', trigger: 'blur' }],
  category: [{ required: true, message: '请选择模型类型', trigger: 'change' }],
}

// suppliers already supplying the chosen vendor first
const supplierOptions = computed(() => {
  const v = state.vendors.find((x) => x.id === form.vendor_id)
  const linked = new Set(v?.suppliers.map((s) => s.provider_id) ?? [])
  const first = state.providers.filter((p) => linked.has(p.id)), rest = state.providers.filter((p) => !linked.has(p.id))
  return [{ label: v ? `已为 ${v.name} 供货` : '全部供应商', items: v ? first : rest }, ...(v ? [{ label: '其他供应商（保存后自动建立供货关系）', items: rest }] : [])].filter((g) => g.items.length)
})

function openCreate() { editing.value = null; Object.assign(form, blank()); formVisible.value = true }
function openEdit(m: Model) {
  editing.value = m
  Object.assign(form, {
    vendor_id: m.vendor_id ?? undefined, provider_id: m.provider_id, model_key: m.model_key, display_name: m.display_name, type: m.type, category: m.category,
    context_k: Math.round(m.context_length / 1024), tags: [...m.tags], description: m.description,
    input: Number(m.input_price_per_1k), output: Number(m.output_price_per_1k), tpm_limit: m.tpm_limit, qps_limit: m.qps_limit,
    released_at: m.released_at ? m.released_at.slice(0, 10) : '',
  })
  formVisible.value = true
}

async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  const body = {
    display_name: form.display_name, vendor_id: form.vendor_id, category: form.category, context_length: form.context_k * 1024, tags: form.tags,
    description: form.description, released_at: form.released_at || '',
    input_price_per_1k: String(form.input), output_price_per_1k: String(form.output), tpm_limit: form.tpm_limit, qps_limit: form.qps_limit,
  }
  try {
    if (editing.value) await models.update(editing.value.id, body)
    else await models.create({ ...body, provider_id: form.provider_id, model_key: form.model_key, type: form.type })
    ElMessage.success('已保存'); formVisible.value = false
    await load(); await reload()
    if (editing.value) current.value = all.value.find((x) => x.id === editing.value!.id) ?? null
  } finally { saving.value = false }
}

defineExpose({ categoryLabel })
</script>

<style scoped>
.filters { margin-bottom: 12px; }
.filters :deep(.panel-body) { padding: 10px 16px; }
.bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; gap: 12px; flex-wrap: wrap; }
.bar-r { display: flex; gap: 8px; align-items: center; }
.vgroups { display: grid; gap: 18px; min-height: 120px; }
.vgroup header { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.vgroup header b { font-weight: 600; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(290px, 1fr)); gap: 12px; min-height: 120px; }
.pager { display: flex; justify-content: flex-end; margin-top: 16px; }
</style>
