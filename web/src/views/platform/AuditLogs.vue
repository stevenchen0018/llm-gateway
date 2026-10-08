<template>
  <div>
    <PageHeader title="审计日志" desc="API Key 全生命周期、配额与 IP 白名单变更的操作留痕，服务可管理、可追溯">
      <TransferBar name="audit_logs" :filters="{ action, key_id: keyId, q: keyword }" @imported="pg.load" />
      <el-button :icon="Refresh" :loading="pg.loading.value" @click="pg.load">刷新</el-button>
    </PageHeader>

    <Panel title="操作记录" :count="pg.total.value" flush>
      <template #toolbar>
        <KeySelect v-model="keyId" placeholder="全部 Key" width="190px" @change="pg.reset" />
        <el-select v-model="action" clearable placeholder="全部操作" style="width: 140px" @change="pg.reset"><el-option v-for="(v, k) in ACTION" :key="k" :label="v.text" :value="k" /></el-select>
        <el-input v-model="keyword" placeholder="搜索 Key 名称 / 操作人 / 详情" :prefix-icon="Search" clearable style="width: 240px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无记录">
        <el-table-column label="时间" width="180"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="操作" width="140"><template #default="{ row }"><StatusBadge :text="ACTION[row.action as Action].text" :tone="ACTION[row.action as Action].tone" /></template></el-table-column>
        <el-table-column label="API Key" min-width="180"><template #default="{ row }">{{ keyName(row.key_id) }}</template></el-table-column>
        <el-table-column label="操作人" width="140" prop="operator" />
        <el-table-column label="详情" min-width="280" show-overflow-tooltip prop="detail" />
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>
  </div>
</template>

<script setup lang="ts">
import TransferBar from '../../components/transfer/TransferBar.vue'
import { onMounted, ref } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import TablePager from '../../components/TablePager.vue'
import KeySelect from '../../components/KeySelect.vue'
import { keys } from '../../api'
import { useLookups } from '../../composables/useLookups'
import { usePaged } from '../../composables/usePaged'
import { fmtTime } from '../../utils'
import type { AuditLog } from '../../types'

type Action = AuditLog['action']
const ACTION: Record<Action, { text: string; tone: 'primary' | 'success' | 'danger' | 'warning' | 'accent' }> = {
  apply: { text: '提交申请', tone: 'primary' }, approve: { text: '审批分发', tone: 'success' }, blacklist: { text: '加入黑名单', tone: 'danger' },
  restore: { text: '移出黑名单', tone: 'warning' }, update_quota: { text: '调整配额', tone: 'accent' },
  update_ip_whitelist: { text: 'IP 白名单', tone: 'primary' },
  update_models: { text: '可用模型', tone: 'accent' }, claim: { text: '领取密钥', tone: 'success' }, rotate: { text: '重置密钥', tone: 'warning' },
}

const { keyName } = useLookups()
const action = ref('')
const keyword = ref('')
const keyId = ref<number>()
const pg = usePaged((p) => keys.auditPage({ ...p, action: action.value, key_id: keyId.value, q: keyword.value.trim() }))
onMounted(pg.load)
</script>
