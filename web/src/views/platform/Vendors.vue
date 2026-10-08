<template>
  <div>
    <PageHeader title="厂商管理" desc="厂商是模型的研发方（DeepSeek、阿里通义、OpenAI …）。一个厂商可由多家供应商提供：在这里维护每个厂商由哪些供应商供货、启停，以及无调度策略时的供应商选择方式">
      <TransferBar name="vendors" :filters="{ status: fStatus, q: keyword }" @imported="refresh" />
      <el-button v-if="isSuper" type="primary" :icon="Plus" @click="openCreate">新增厂商</el-button>
    </PageHeader>

    <Panel title="厂商列表" :count="pg.total.value" flush>
      <template #toolbar>
        <el-radio-group v-model="fStatus" @change="pg.reset"><el-radio-button value="">全部</el-radio-button><el-radio-button value="active">启用</el-radio-button><el-radio-button value="disabled">停用</el-radio-button></el-radio-group>
        <el-input v-model="keyword" placeholder="搜索厂商名称 / 编码" :prefix-icon="Search" clearable style="width: 200px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" row-key="id" :expand-row-keys="expanded" empty-text="暂无厂商" @expand-change="onExpand">
        <el-table-column type="expand" width="36">
          <template #default="{ row }">
            <div class="sup">
              <div class="sup-head">
                <b>{{ row.name }} 的供应商</b>
                <span class="muted">无调度策略时按「{{ VENDOR_ROUTING[row.routing_strategy]?.text }}」选择：{{ VENDOR_ROUTING[row.routing_strategy]?.hint }}</span>
                <span class="spacer" />
                <el-button v-if="isSuper" size="small" :icon="Plus" @click="openAddSupplier(row)">添加供应商</el-button>
              </div>
              <el-table :data="row.suppliers" size="small" empty-text="尚无供应商">
                <el-table-column label="供应商" min-width="200"><template #default="{ row: s }"><div class="mcell"><VendorLogo :code="s.provider?.code ?? ''" :size="22" /><div><div>{{ s.provider?.name ?? `#${s.provider_id}` }}</div><div class="cell-sub mono">{{ s.provider?.code }}</div></div></div></template></el-table-column>
                <el-table-column label="类型" width="112"><template #default="{ row: s }"><StatusBadge v-if="s.provider" :text="SUPPLIER_TYPE[s.provider.supplier_type]?.text ?? '—'" :tone="SUPPLIER_TYPE[s.provider.supplier_type]?.tone ?? 'info'" /></template></el-table-column>
                <el-table-column label="折扣" width="80"><template #default="{ row: s }"><span class="num">{{ s.provider && Number(s.provider.discount_rate) < 1 ? discount(s.provider.discount_rate) : '无' }}</span></template></el-table-column>
                <el-table-column label="提供模型" width="84" align="right"><template #default="{ row: s }"><span class="num">{{ s.model_count }}</span></template></el-table-column>
                <el-table-column label="优先级" width="124"><template #default="{ row: s }"><el-input-number v-if="isSuper" :model-value="s.priority" :min="0" :max="9999" size="small" controls-position="right" style="width: 100px" @change="(v: number | undefined) => saveLink(s, { priority: v ?? 0 })" /><span v-else class="num">{{ s.priority }}</span></template></el-table-column>
                <el-table-column label="权重" width="124"><template #default="{ row: s }"><el-input-number v-if="isSuper" :model-value="s.weight" :min="1" :max="10000" size="small" controls-position="right" style="width: 100px" @change="(v: number | undefined) => saveLink(s, { weight: v ?? 1 })" /><span v-else class="num">{{ s.weight }}</span></template></el-table-column>
                <el-table-column label="流量占比" width="90" align="right"><template #default="{ row: s }"><span class="num muted">{{ share(row, s) }}</span></template></el-table-column>
                <el-table-column label="启用" width="70"><template #default="{ row: s }"><el-switch :model-value="s.status === 'active'" size="small" :disabled="!isSuper" @change="(v: string | number | boolean) => saveLink(s, { status: v ? 'active' : 'disabled' })" /></template></el-table-column>
                <el-table-column label="备注" min-width="160" show-overflow-tooltip><template #default="{ row: s }"><span :class="{ faint: !s.remark }">{{ s.remark || '—' }}</span></template></el-table-column>
                <el-table-column label="" width="70" align="right">
                  <template #default="{ row: s }"><el-popconfirm v-if="isSuper" title="移除该供应商？（仍在供货的请改为停用）" width="240" @confirm="removeLink(s)"><template #reference><el-button link type="danger" size="small">移除</el-button></template></el-popconfirm></template>
                </el-table-column>
              </el-table>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="厂商" min-width="200">
          <template #default="{ row }"><div class="mcell"><VendorLogo :code="row.code" :size="30" /><div><div class="cell-title">{{ row.name }}</div><div class="cell-sub mono">{{ row.code }}</div></div></div></template>
        </el-table-column>
        <el-table-column label="说明" min-width="220" show-overflow-tooltip><template #default="{ row }">{{ row.description || '—' }}</template></el-table-column>
        <el-table-column label="供应商" min-width="230">
          <template #default="{ row }">
            <div class="vlist">
              <span v-for="s in row.suppliers" :key="s.id" class="vtag" :class="{ off: s.status === 'disabled' }"><VendorLogo :code="s.provider?.code ?? ''" :size="16" />{{ s.provider?.name }}</span>
              <span v-if="!row.suppliers.length" class="faint">—</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="模型" width="64" align="right"><template #default="{ row }"><span class="num">{{ row.model_count }}</span></template></el-table-column>
        <el-table-column label="供应商选择" width="136"><template #default="{ row }"><StatusBadge :text="VENDOR_ROUTING[row.routing_strategy]?.text ?? row.routing_strategy" tone="primary" /></template></el-table-column>
        <el-table-column label="状态" width="80"><template #default="{ row }"><StatusBadge :text="row.status === 'active' ? '启用' : '停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="" width="150" align="right" fixed="right">
          <template v-if="isSuper" #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link :type="row.status === 'active' ? 'danger' : 'primary'" @click="toggle(row)">{{ row.status === 'active' ? '停用' : '启用' }}</el-button>
            <el-popconfirm title="删除该厂商？" @confirm="remove(row)"><template #reference><el-button link type="danger" :disabled="row.model_count > 0">删除</el-button></template></el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <el-dialog v-model="visible" :title="editing ? '编辑厂商' : '新增厂商'" width="560px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid">
          <el-form-item label="编码" prop="code"><el-input v-model="form.code" :disabled="!!editing" placeholder="如 deepseek" /></el-form-item>
          <el-form-item label="名称" prop="name"><el-input v-model="form.name" placeholder="如 DeepSeek" /></el-form-item>
          <el-form-item label="官网" class="span2"><el-input v-model="form.website" placeholder="https://" /></el-form-item>
          <el-form-item label="说明" class="span2"><el-input v-model="form.description" /></el-form-item>
          <el-form-item label="供应商选择方式（无调度策略时）" class="span2">
            <el-radio-group v-model="form.routing_strategy"><el-radio-button v-for="(r, k) in VENDOR_ROUTING" :key="k" :value="k">{{ r.text }}</el-radio-button></el-radio-group>
            <div class="hint">{{ VENDOR_ROUTING[form.routing_strategy]?.hint }}。已配置的调度策略优先于此设置。</div>
          </el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="supVisible" :title="`添加供应商 · ${supVendor?.name}`" width="480px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="供应商"><el-select v-model="sup.provider_id" filterable placeholder="选择供应商" style="width: 100%"><el-option v-for="p in supCandidates" :key="p.id" :label="p.name" :value="p.id"><span>{{ p.name }}</span><span class="faint" style="float: right; font-size: 12px">{{ SUPPLIER_TYPE[p.supplier_type]?.text }}</span></el-option></el-select></el-form-item>
        <div class="form-grid">
          <el-form-item label="优先级（小的先用）"><el-input-number v-model="sup.priority" :min="0" :max="9999" controls-position="right" /></el-form-item>
          <el-form-item label="权重"><el-input-number v-model="sup.weight" :min="1" :max="10000" controls-position="right" /></el-form-item>
        </div>
        <el-form-item label="备注"><el-input v-model="sup.remark" placeholder="如 合同编号、折扣到期时间" /></el-form-item>
        <div class="hint">添加后，在模型市场为该供应商上架此厂商的模型即可参与路由。</div>
      </el-form>
      <template #footer><el-button @click="supVisible = false">取消</el-button><el-button type="primary" :loading="saving" :disabled="!sup.provider_id" @click="addSupplier">添加</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, type FormInstance } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import VendorLogo from '../../components/VendorLogo.vue'
import TablePager from '../../components/TablePager.vue'
import { vendors } from '../../api'
import { useAuth } from '../../composables/useAuth'
import { useLookups } from '../../composables/useLookups'
import { usePaged } from '../../composables/usePaged'
import { SUPPLIER_TYPE, VENDOR_ROUTING } from '../../constants'
import type { VendorRouting, VendorSupplier, VendorView } from '../../types'

const { isSuper } = useAuth()
const { state, reload } = useLookups()
const discount = (v: string) => `${(Number(v) * 10).toFixed(1).replace(/\.0$/, '')} 折`

const fStatus = ref('')
const keyword = ref('')
const pg = usePaged((p) => vendors.page({ ...p, status: fStatus.value, q: keyword.value.trim() }))
onMounted(pg.load)
async function refresh() { await Promise.all([pg.refresh(), reload()]) }

// keep expanded rows open across reloads
const expanded = ref<number[]>([])
function onExpand(row: VendorView, rows: VendorView[]) { expanded.value = rows.map((r) => r.id); void row }

// expected share of first-choice traffic under weighted selection
function share(v: VendorView, s: VendorSupplier) {
  if (v.routing_strategy !== 'supplier_weighted' || s.status !== 'active') return s.status === 'active' ? '—' : '0%'
  const total = v.suppliers.filter((x) => x.status === 'active').reduce((a, x) => a + x.weight, 0)
  return total ? `${Math.round((s.weight / total) * 100)}%` : '—'
}

async function saveLink(s: VendorSupplier, patch: Partial<VendorSupplier>) {
  await vendors.updateSupplier(s.id, patch)
  ElMessage.success(patch.status === 'disabled' ? '已停用：该供应商不再承接此厂商模型的流量' : '已更新，立即生效')
  await refresh()
}
async function removeLink(s: VendorSupplier) { await vendors.removeSupplier(s.id); ElMessage.success('已移除'); await refresh() }

// ---- add supplier -----------------------------------------------------------------------------
const supVisible = ref(false)
const supVendor = ref<VendorView | null>(null)
const sup = reactive({ provider_id: undefined as number | undefined, priority: 100, weight: 100, remark: '' })
const supCandidates = computed(() => state.providers.filter((p) => !supVendor.value?.suppliers.some((s) => s.provider_id === p.id)))
function openAddSupplier(v: VendorView) { supVendor.value = v; Object.assign(sup, { provider_id: undefined, priority: 100, weight: 100, remark: '' }); supVisible.value = true }
async function addSupplier() {
  saving.value = true
  try { await vendors.addSupplier(supVendor.value!.id, { ...sup }); ElMessage.success('已添加'); supVisible.value = false; await refresh() } finally { saving.value = false }
}

// ---- vendor create / edit -----------------------------------------------------------------------
const visible = ref(false)
const saving = ref(false)
const editing = ref<VendorView | null>(null)
const formRef = ref<FormInstance>()
const blank = () => ({ code: '', name: '', website: '', description: '', routing_strategy: 'cost_first' as VendorRouting })
const form = reactive(blank())
const rules = {
  code: [{ required: true, pattern: /^[a-z][a-z0-9_-]{1,31}$/, message: '小写字母开头，2–32 位字母数字 - _', trigger: 'blur' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
}
function openCreate() { editing.value = null; Object.assign(form, blank()); visible.value = true }
function openEdit(v: VendorView) { editing.value = v; Object.assign(form, { code: v.code, name: v.name, website: v.website, description: v.description, routing_strategy: v.routing_strategy }); visible.value = true }
async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    if (editing.value) await vendors.update(editing.value.id, { ...form, status: editing.value.status })
    else await vendors.create({ ...form })
    ElMessage.success('已保存'); visible.value = false; await refresh()
  } finally { saving.value = false }
}
async function toggle(v: VendorView) {
  const on = v.status !== 'active'
  await vendors.update(v.id, { name: v.name, description: v.description, website: v.website, routing_strategy: v.routing_strategy, status: on ? 'active' : 'disabled' })
  ElMessage.success(on ? '已启用' : '已停用：该厂商的全部模型不再参与路由'); await refresh()
}
async function remove(v: VendorView) { await vendors.remove(v.id); ElMessage.success('已删除'); await refresh() }
</script>

<style scoped>
.mcell { display: flex; align-items: center; gap: 10px; }
.vlist { display: flex; flex-wrap: wrap; gap: 4px; }
.vtag { display: inline-flex; align-items: center; gap: 4px; padding: 1px 6px 1px 2px; border: 1px solid var(--border); border-radius: 999px; font-size: 12px; white-space: nowrap; }
.vtag.off { opacity: .5; text-decoration: line-through; }
.sup { padding: 4px 16px 12px 52px; background: #FAFBFC; }
.sup-head { display: flex; align-items: center; gap: 10px; padding: 8px 0; font-size: 13px; }
.spacer { flex: 1; }
</style>
