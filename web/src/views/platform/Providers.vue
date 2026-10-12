<template>
  <div>
    <PageHeader title="供应商管理" desc="供应商是购买与调用模型的渠道：厂商官方 API、云平台、代理商或自建集群。一个厂商可由多家供应商提供，价格、折扣与容量按供应商核算；凭证只写入不回显">
      <TransferBar name="suppliers" :filters="{ supplier_type: fType, status: fStatus, q: keyword }" @imported="load" />
      <el-button :icon="DataAnalysis" @click="$router.push('/provider-tests')">可用性测试</el-button>
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
        <el-table-column label="接口协议 / Base URL" min-width="220">
          <template #default="{ row }"><div class="proto"><ProtocolTag :provider="row" /><span class="mono url" :title="row.base_url">{{ row.base_url }}</span></div><div class="cell-sub">{{ authOf(row) }}</div></template>
        </el-table-column>
        <el-table-column label="可用性（7 天）" width="150">
          <template #default="{ row }">
            <template v-if="health[row.id]?.last_run">
              <div class="health"><StatusBadge :text="TEST_RUN_STATUS[health[row.id].last_run!.status]?.text ?? ''" :tone="TEST_RUN_STATUS[health[row.id].last_run!.status]?.tone ?? 'info'" /><span v-if="health[row.id].availability >= 0" class="num muted">{{ (health[row.id].availability * 100).toFixed(health[row.id].availability === 1 ? 0 : 1) }}%</span></div>
              <div class="cell-sub">{{ fmtTime(health[row.id].last_run!.started_at) }}</div>
            </template>
            <span v-else class="faint">未测试</span>
          </template>
        </el-table-column>
        <el-table-column label="折扣" width="100"><template #default="{ row }"><StatusBadge :text="Number(row.discount_rate) < 1 ? discount(row.discount_rate) : '无折扣'" :tone="Number(row.discount_rate) < 1 ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="模型数" width="72" align="right"><template #default="{ row }"><span class="num">{{ modelCount(row.id) }}</span></template></el-table-column>
        <el-table-column label="商务联系人" width="150" show-overflow-tooltip><template #default="{ row }"><span :class="{ faint: !row.contact }">{{ row.contact || '—' }}</span></template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><StatusBadge :text="row.status === 'active' ? '启用' : '停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="操作" :width="isSuper ? 200 : 80" align="right" fixed="right" class-name="col-actions">
          <template #default="{ row }">
            <el-button v-if="isSuper" link type="primary" :disabled="row.status !== 'active'" @click="openTest(row)">测试</el-button>
            <el-button link type="primary" @click="openReport(row.id)">报告</el-button>
            <template v-if="isSuper">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link :type="row.status === 'active' ? 'danger' : 'primary'" @click="toggle(row, row.status !== 'active')">{{ row.status === 'active' ? '停用' : '启用' }}</el-button>
            </template>
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
          <el-form-item label="接口协议" class="span2">
            <el-radio-group v-model="form.protocol">
              <el-radio-button v-for="k in ['openai', 'anthropic', 'gemini']" :key="k" :value="k">{{ PROTOCOL[k].text }}</el-radio-button>
            </el-radio-group>
            <div class="hint">{{ PROTOCOL_HINT[form.protocol] }}。业务方无论用哪种协议调用网关，都会被转换为该供应商的协议</div>
          </el-form-item>
          <el-form-item label="Base URL" prop="base_url" class="span2">
            <el-input v-model="form.base_url" :placeholder="PROTOCOL[form.protocol].base" />
            <div class="hint">填 <span class="mono">mock://xxx</span> 使用内置 Mock 适配器（联调用，不发起真实请求）</div>
          </el-form-item>
          <el-form-item v-if="form.protocol === 'openai'" label="鉴权方式"><el-select v-model="form.auth_type"><el-option label="无" value="" /><el-option label="Bearer Token" value="bearer" /><el-option label="Api-Key Header" value="api_key_header" /></el-select></el-form-item>
          <el-form-item v-else label="鉴权方式"><el-input :model-value="`${PROTOCOL[form.protocol].auth} 请求头（协议原生）`" disabled /></el-form-item>
          <el-form-item label="凭证"><el-input v-model="form.auth_value" type="password" show-password :placeholder="editing ? '留空表示不修改' : 'API Key / Token'" /></el-form-item>
          <el-form-item label="折扣率"><el-input-number v-model="form.discount" :min="0.1" :max="1" :step="0.01" :precision="2" controls-position="right" /><div class="hint">折后价 = 标价 × 折扣率；1 表示无折扣</div></el-form-item>
          <el-form-item label="说明"><el-input v-model="form.description" /></el-form-item>
        </div>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存</el-button></template>
    </el-dialog>

    <ProviderTestDialog v-model:visible="testVisible" :provider="testing" @report="(p) => { testVisible = false; openReport(p.id) }" @done="loadHealth" />
    <ProviderReportDrawer v-model:visible="reportVisible" :provider-id="reportId" @test="(p) => openTest(p)" />
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import { useAuth } from '../../composables/useAuth'
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, type FormInstance } from 'element-plus'
import { DataAnalysis, Plus, Search } from '@element-plus/icons-vue'
import ProtocolTag from '../../components/provider/ProtocolTag.vue'
import ProviderTestDialog from '../../components/provider/ProviderTestDialog.vue'
import ProviderReportDrawer from '../../components/provider/ProviderReportDrawer.vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import VendorLogo from '../../components/VendorLogo.vue'
import { providerTests, providers } from '../../api'
import { useLookups } from '../../composables/useLookups'
import { PROTOCOL, SUPPLIER_TYPE, TEST_RUN_STATUS } from '../../constants'
import { fmtTime } from '../../utils'
import type { Provider, ProviderTestOverview } from '../../types'

const { isSuper } = useAuth()

const authText: Record<string, string> = { bearer: 'Bearer Token', api_key_header: 'Api-Key Header' }
const PROTOCOL_HINT: Record<string, string> = {
  openai: 'OpenAI Chat Completions：OpenAI 及所有 OpenAI 兼容接口（DeepSeek、通义、豆包、vLLM 等），Base URL 通常以 /v1 结尾',
  anthropic: 'Anthropic Messages API：Claude 官方及兼容渠道，Base URL 如 https://api.anthropic.com（可带或不带 /v1）',
  gemini: 'Gemini generateContent：Google AI Studio / Gemini API，Base URL 如 https://generativelanguage.googleapis.com',
}
const authOf = (p: Provider) => (p.base_url.startsWith('mock://') ? '内置 Mock 适配器' : p.protocol && p.protocol !== 'openai' ? `${PROTOCOL[p.protocol]?.auth} 鉴权` : authText[p.auth_type] || '无鉴权')

// availability (latest test + 7-day pass rate) per supplier
const health = ref<Record<number, ProviderTestOverview>>({})
async function loadHealth() {
  try { health.value = Object.fromEntries((await providerTests.overview(7)).map((o) => [o.provider.id, o])) } catch { /* optional column */ }
}
const testVisible = ref(false)
const testing = ref<Provider | null>(null)
const reportVisible = ref(false)
const reportId = ref<number | null>(null)
function openTest(p: Provider) { testing.value = p; reportVisible.value = false; testVisible.value = true }
function openReport(id: number) { reportId.value = id; reportVisible.value = true }
const { state, reload } = useLookups()
const rows = ref<Provider[]>([])
const loading = ref(false)
const visible = ref(false)
const saving = ref(false)
const editing = ref<Provider | null>(null)
const formRef = ref<FormInstance>()
const form = reactive({ code: '', name: '', base_url: '', protocol: 'openai', auth_type: 'bearer', auth_value: '', discount: 1, description: '', supplier_type: 'cloud', contact: '' })
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
onMounted(() => { load(); loadHealth() })

function openCreate() { editing.value = null; Object.assign(form, { code: '', name: '', base_url: '', protocol: 'openai', auth_type: 'bearer', auth_value: '', discount: 1, description: '', supplier_type: 'cloud', contact: '' }); visible.value = true }
function openEdit(p: Provider) { editing.value = p; Object.assign(form, { code: p.code, name: p.name, base_url: p.base_url, protocol: p.protocol || 'openai', auth_type: p.auth_type, auth_value: '', discount: Number(p.discount_rate) || 1, description: p.description, supplier_type: p.supplier_type || 'cloud', contact: p.contact }); visible.value = true }

async function save() {
  if (!(await formRef.value?.validate().catch(() => false))) return
  saving.value = true
  try {
    const common = { name: form.name, base_url: form.base_url, protocol: form.protocol, auth_type: form.auth_type, auth_value: form.auth_value, discount_rate: String(form.discount), description: form.description, supplier_type: form.supplier_type, contact: form.contact }
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
.proto { display: flex; align-items: center; gap: 8px; min-width: 0; }
.proto .url { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.health { display: flex; align-items: center; gap: 6px; }
</style>
