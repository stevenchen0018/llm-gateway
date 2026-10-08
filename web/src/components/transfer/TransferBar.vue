<template>
  <div v-if="entity && (entity.can_import || entity.can_export)" class="tbar">
    <el-button v-if="entity.can_import" :icon="Upload" @click="importVisible = true">导入</el-button>
    <el-dropdown v-if="entity.can_export || entity.can_import" trigger="click" @command="onCommand">
      <el-button :icon="Download" :loading="busy">导出<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
      <template #dropdown>
        <el-dropdown-menu>
          <template v-if="entity.can_export">
            <el-dropdown-item command="xlsx" :icon="Document">导出 Excel{{ filtered ? '（当前筛选）' : '' }}</el-dropdown-item>
            <el-dropdown-item command="csv" :icon="Tickets">导出 CSV{{ filtered ? '（当前筛选）' : '' }}</el-dropdown-item>
          </template>
          <el-dropdown-item v-if="entity.can_import" command="template" :icon="Files" :divided="entity.can_export">下载导入模板</el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
    <ImportDialog v-if="entity.can_import" v-model:visible="importVisible" :entity="entity" @done="$emit('imported')" />
  </div>
</template>

<script setup lang="ts">
// TransferBar: the 导入 / 导出 buttons placed on list pages. Export applies the
// page's current filters (passed in `filters`), so what you see is what you get.
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ArrowDown, Document, Download, Files, Tickets, Upload } from '@element-plus/icons-vue'
import ImportDialog from './ImportDialog.vue'
import { transfer } from '../../api'
import { useTransferEntity } from '../../composables/useTransfer'

const props = defineProps<{ name: string; filters?: Record<string, unknown> }>()
defineEmits<{ imported: [] }>()

const entity = useTransferEntity(props.name)
const importVisible = ref(false)
const busy = ref(false)
const filtered = computed(() => Object.values(props.filters ?? {}).some((v) => v !== undefined && v !== null && v !== '' && v !== false))

async function onCommand(cmd: string) {
  busy.value = true
  try {
    if (cmd === 'template') {
      await transfer.template(props.name)
      ElMessage.success('模板已下载')
    } else {
      const r = await transfer.export(props.name, cmd as 'xlsx' | 'csv', props.filters)
      if (r.truncated) ElMessage.warning(`已导出前 ${r.rows.toLocaleString()} 行（超过上限部分已截断，请缩小筛选范围）`)
      else ElMessage.success(`已导出 ${r.rows.toLocaleString()} 行`)
    }
  } catch (e) {
    ElMessage.error((e as Error).message)
  } finally { busy.value = false }
}
</script>

<style scoped>
.tbar { display: inline-flex; gap: 8px; }
</style>
