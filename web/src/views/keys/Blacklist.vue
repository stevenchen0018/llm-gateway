<template>
  <div>
    <PageHeader title="黑名单" desc="被拉黑的 Key 会被网关立即拒绝；可查看原因、追溯操作，并在核实后移出黑名单">
      <TransferBar name="keys" :filters="{ status: 'blacklisted', q: keyword }" @imported="load" />
      <el-button v-if="canWrite" type="danger" plain :icon="CircleClose" @click="openAdd">加入黑名单</el-button>
    </PageHeader>

    <Panel title="黑名单 Key" :count="pg.total.value" flush>
      <template #toolbar><el-input v-model="keyword" placeholder="名称 / 负责人 / 前缀 / 应用 / #ID" :prefix-icon="Search" clearable style="width: 240px" @input="pg.search" /></template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="黑名单为空">
        <el-table-column label="Key" min-width="200"><template #default="{ row }"><div class="cell-title">{{ row.name }}</div><div class="cell-sub mono">{{ row.key_prefix.startsWith('sk-') ? row.key_prefix + '…' : '—' }}</div></template></el-table-column>
        <el-table-column label="应用" width="150" show-overflow-tooltip><template #default="{ row }">{{ row.app_id ? appName(row.app_id) : '—' }}</template></el-table-column>
        <el-table-column label="负责人" width="120" prop="owner" />
        <el-table-column label="拉黑原因" min-width="260"><template #default="{ row }"><span :title="row.blacklist_reason">{{ row.blacklist_reason || '—' }}</span></template></el-table-column>
        <el-table-column label="拉黑时间" width="170"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.blacklisted_at) }}</span></template></el-table-column>
        <el-table-column label="" width="130" align="right" fixed="right">
          <template v-if="canWrite" #default="{ row }"><el-popconfirm title="确认移出黑名单？恢复后 Key 立即可用（原本未审批的将回到待审批）。" width="260" @confirm="restore(row)"><template #reference><el-button link type="primary">移出黑名单</el-button></template></el-popconfirm></template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <el-dialog v-model="visible" title="加入黑名单" width="460px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="选择 Key"><KeySelect v-model="keyId" status="active,pending" placeholder="搜索要拉黑的 Key" width="100%" /></el-form-item>
        <el-form-item label="原因"><el-input v-model="reason" type="textarea" :rows="3" resize="none" placeholder="如 未经审批共享、异常调用、泄露风险" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="danger" :loading="saving" :disabled="!keyId || !reason.trim()" @click="add">确认加入</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import { useAuth } from '../../composables/useAuth'
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CircleClose, Search } from '@element-plus/icons-vue'
import KeySelect from '../../components/KeySelect.vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import { keys } from '../../api'
import { useLookups } from '../../composables/useLookups'
import { fmtTime } from '../../utils'
import type { ApiKey } from '../../types'

const { canWrite } = useAuth()

const { appName, reload } = useLookups()
const keyword = ref('')
const pg = usePaged((p) => keys.search({ ...p, status: 'blacklisted', q: keyword.value.trim() }))
const load = pg.refresh
onMounted(load)

async function restore(k: ApiKey) { await keys.restore(k.id); ElMessage.success('已移出黑名单'); await Promise.all([load(), reload()]) }

const visible = ref(false)
const saving = ref(false)
const keyId = ref<number>()
const reason = ref('')
function openAdd() { keyId.value = undefined; reason.value = ''; visible.value = true }
async function add() {
  saving.value = true
  try { await keys.blacklist(keyId.value!, reason.value.trim()); ElMessage.success('已加入黑名单'); visible.value = false; await Promise.all([load(), reload()]) } finally { saving.value = false }
}
</script>
