<template>
  <div>
    <PageHeader title="供应商管理" desc="供应商是购买与调用模型的渠道：厂商官方 API、云平台、代理商或自建集群。一个厂商可由多家供应商提供，价格、折扣与容量按供应商核算；凭证只写入不回显">
      <TransferBar name="suppliers" :filters="{ supplier_type: fType, status: fStatus, q: keyword }" @imported="load" />
      <el-button v-if="isSuper" type="primary" :icon="Plus" @click="openCreate">接入供应商</el-button>
    </PageHeader>

    <Panel title="供应商列表" :count="pg.total.value" flush>
      <template #toolbar>
        <el-select v-model="fType" clearable placeholder="全部类型" style="width: 120px" @change="pg.reset"><el-option v-for="(t, k) in SUPPLIER_TYPE" :key="k" :label="t.text" :value="k" /></el-select>
        <el-radio-group v-model="fStatus" @change="pg.reset"><el-radio-button value="">全部</el-radio-button><el-radio-button value="active">启用</el-radio-button><el-radio-button value="disabled">停用</el-radio-button></el-radio-group>
        <el-input v-model="keyword" placeholder="搜索供应商 / 编码 / 地址 / 联系人" :prefix-icon="Search" clearable style="width: 240px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无供应商，点击右上角接入">
        <el-table-column label="供应商" min-width="190"><template #default="{ row }"><div class="mcell"><VendorLogo :code="row.code" :size="30" /><div><div class="cell-title">{{ row.name }}</div><div class="cell-sub mono">{{ row.code }}</div></div></div></template></el-table-column>
        <el-table-column label="类型" width="112"><template #default="{ row }"><StatusBadge :text="SUPPLIER_TYPE[row.supplier_type]?.text ?? row.supplier_type" :tone="SUPPLIER_TYPE[row.supplier_type]?.tone ?? 'info'" /></template></el-table-column>
        <el-table-column label="供应的厂商" min-width="200">
          <template #default="{ row }">
            <div class="vlist">
              <el-tooltip v-for="v in suppliedVendors(row.id)" :key="v.id" :content="v.link.status === 'disabled' ? `${v.name}：已停用该供应商` : `${v.name}：优先级 ${v.link.priority} · 权重 ${v.link.weight}`" placement="top">
                <span class="vtag" :class="{ off: v.link.status === 'disabled' }"><VendorLogo :code="v.code" :size="16" />{{ v.name }}</span>
              </el-tooltip>
              <span v-if="!suppliedVendors(row.id).length" class="faint">—</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Base URL" min-width="200" show-overflow-tooltip><template #default="{ row }"><span class="mono">{{ row.base_url }}</span><div class="cell-sub">{{ row.base_url.startsWith('mock://') ? 'Mock 适配器' : 'OpenAI 兼容' }} · {{ authText[row.auth_type] || '无鉴权' }}</div></template></el-table-column>
        <el-table-column label="折扣" width="100"><template #default="{ row }"><StatusBadge :text="Number(row.discount_rate) < 1 ? discount(row.discount_rate) : '无折扣'" :tone="Number(row.discount_rate) < 1 ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="模型数" width="72" align="right"><template #default="{ row }"><span class="num">{{ modelCount(row.id) }}</span></template></el-table-column>
        <el-table-column label="商务联系人" width="150" show-overflow-tooltip><template #default="{ row }"><span :class="{ faint: !row.contact }">{{ row.contact || '—' }}</span></template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><StatusBadge :text="row.status === 'active' ? '启用' : '停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="操作" width="130" align="right" fixed="right" class-name="col-actions">
          <template v-if="isSuper" #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link :type="row.status === 'active' ? 'danger' : 'primary'" @click="toggle(row, row.status !== 'active')">{{ row.status === 'active' ? '停用' : '启用' }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <el-dialog v-model="visible" :title="editing ? '编辑供应商' : '接入供应商'" width="560px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid">
          <el-form-item label="编码" prop="code"><el-input v-model="form.code" :disabled="!!editing" placeholder="aliyun / deepseek" /></el-form-item>
          <el-form-item label="名称" prop="name"><el-input v-model="form.name" placeholder="如 阿里云百炼" /></el-form-item>
          <el-form-item label="供应商类型"><el-select v-model="form.supplier_type"><el-option v-for="(t, k) in SUPPLIER_TYPE" :key="k" :label="t.text" :value="k" /></el-select></el-form-item>
          <el-form-item label="商务联系人"><el-input v-model="form.contact" placeholder="如 张敏 · 大客户经理" /></el-form-item>
          <el-form-item label="Base URL" prop="base_url" class="span2">
            <el-input v-model="form.base_url" placeholder="https://api.example.com/v1" />
            <div class="hint">OpenAI 兼容地址；填 <span class="mono">mock://xxx</span> 使用内置 Mock 适配器（联调用）</div>
          </el-form-item>
          <el-form-item label="鉴权方式"><el-select v-model="form.auth_type"><el-option label="无" value="" /><el-option label="Bearer Token" value="bearer" /><el-option label="Api-Key Header" value="api_key_header" /></el-select></el-form-item>
          <el-form-item label="凭证"><el-input v-model="form.auth_value" type="password" show-password :placeholder="editing ? '留空表示不修改' : 'API Key / Token'" /></el-form-item>
          <el-form-item label="折扣率"><el-input-number v-model="form.discount" :min="0.1" :max="1" :step="0.01" :precision="2" controls-position="right" /><div class="hint">折后价 = 标价 × 折扣率；1 表示无折扣</div></el-form-item>
          <el-form-item label="说明"><el-input v-model="form.description" /></el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import { useAuth } from '../../composables/useAuth'
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, type FormInstance } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import VendorLogo from '../../components/VendorLogo.vue'
import { providers } from '../../api'
import { useLookups } from '../../composables/useLookups'
import { SUPPLIER_TYPE } from '../../constants'
import type { Provider } from '../../types'

const { isSuper } = useAuth()

const authText: Record<string, string> = { bearer: 'Bearer Token', api_key_header: 'Api-Key Header' }
const { state, reload } = useLookups()
const rows = ref<Provider[]>([])
const loading = ref(false)
const visible = ref(false)
const saving = ref(false)
const editing = ref<Provider | null>(null)
const formRef = ref<FormInstance>()
const form = reactive({ code: '', name: '', base_url: '', auth_type: 'bearer', auth_value: '', discount: 1, description: '', supplier_type: 'cloud', contact: '' })
const rules = {
  code: [{ required: true, message: '请输入编码', trigger: 'blur' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  base_url: [{ required: true, message: '请输入 Base URL', trigger: 'blur' }],
}
const discount = (v: string) => `${(Number(v) * 10).toFixed(1).replace(/\.0$/, '')} 折`
const modelCount = (id: number) => state.models.filter((m) => m.provider_id === id).length
// vendors this supplier supplies (from the vendor→supplier links)
const suppliedVendors = (providerId: number) => state.vendors.flatMap((v) => v.suppliers.filter((s) => s.provider_id === providerId).map((link) => ({ id: v.id, code: v.code, name: v.name, link })))

const fStatus = ref('')
const keyword = ref('')
const fType = ref('')
const pg = usePaged((p) => providers.page({ ...p, status: fStatus.value, supplier_type: fType.value, q: keyword.value.trim() }))
const load = pg.refresh
onMounted(load)

function openCreate() { editing.value = null; Object.assign(form, { code: '', name: '', base_url: '', auth_type: 'bearer', auth_value: '', discount: 1, description: '', supplier_type: 'cloud', contact: '' }); visible.value = true }
function openEdit(p: Provider) { editing.value = p; Object.assign(form, { code: p.code, name: p.name, base_url: p.base_url, auth_type: p.auth_type, auth_value: '', discount: Number(p.discount_rate) || 1, description: p.description, supplier_type: p.supplier_type || 'cloud', contact: p.contact }); visible.value = true }

async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    const common = { name: form.name, base_url: form.base_url, auth_type: form.auth_type, auth_value: form.auth_value, discount_rate: String(form.discount), description: form.description, supplier_type: form.supplier_type, contact: form.contact }
    if (editing.value) await providers.update(editing.value.id, common)
    else await providers.create({ ...common, code: form.code })
    ElMessage.success('已保存'); visible.value = false
    await Promise.all([load(), reload()])
  } finally { saving.value = false }
}

async function toggle(p: Provider, on: boolean) {
  await providers.update(p.id, { status: on ? 'active' : 'disabled' })
  ElMessage.success(on ? '已启用' : '已停用（该供应商的全部模型不再参与路由）')
  await Promise.all([load(), reload()])
}
</script>

<style scoped>
.vlist { display: flex; flex-wrap: wrap; gap: 4px; }
.vtag { display: inline-flex; align-items: center; gap: 4px; padding: 1px 6px 1px 2px; border: 1px solid var(--border); border-radius: 999px; font-size: 12px; }
.vtag.off { opacity: .5; text-decoration: line-through; }
.mcell { display: flex; align-items: center; gap: 10px; }
</style>
