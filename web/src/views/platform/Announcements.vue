<template>
  <div>
    <PageHeader title="公告管理" desc="启用的公告会在控制台顶部滚动展示，用于变更通知、降本提醒与维护公告">
      <el-button type="primary" :icon="Plus" @click="openCreate">新增公告</el-button>
    </PageHeader>

    <Panel title="公告列表" :count="pg.total.value" flush>
      <template #toolbar>
        <el-radio-group v-model="fLevel" @change="pg.reset"><el-radio-button value="">全部</el-radio-button><el-radio-button value="warning">重要</el-radio-button><el-radio-button value="info">通知</el-radio-button></el-radio-group>
        <el-input v-model="keyword" placeholder="搜索公告内容" :prefix-icon="Search" clearable style="width: 200px" @input="pg.search" />
      </template>
      <el-table :data="pg.items.value" v-loading="pg.loading.value" empty-text="暂无公告">
        <el-table-column label="内容" min-width="420"><template #default="{ row }">{{ row.content }}</template></el-table-column>
        <el-table-column label="级别" width="100"><template #default="{ row }"><StatusBadge :text="row.level === 'warning' ? '重要' : '通知'" :tone="row.level === 'warning' ? 'warning' : 'primary'" /></template></el-table-column>
        <el-table-column label="展示" width="90"><template #default="{ row }"><el-switch :model-value="row.active" @change="(v: boolean) => toggle(row, v)" /></template></el-table-column>
        <el-table-column label="发布时间" width="170"><template #default="{ row }"><span class="num muted">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="" width="130" align="right" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-popconfirm title="删除该公告？" @confirm="remove(row)"><template #reference><el-button link type="danger">删除</el-button></template></el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
      <TablePager v-model:page="pg.page.value" v-model:page-size="pg.pageSize.value" :total="pg.total.value" />
    </Panel>

    <el-dialog v-model="visible" :title="editing ? '编辑公告' : '新增公告'" width="520px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="公告内容"><el-input v-model="form.content" type="textarea" :rows="4" resize="none" maxlength="300" show-word-limit /></el-form-item>
        <el-form-item label="级别"><el-radio-group v-model="form.level"><el-radio value="info">通知</el-radio><el-radio value="warning">重要</el-radio></el-radio-group></el-form-item>
        <el-form-item label="立即展示"><el-switch v-model="form.active" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" :disabled="!form.content.trim()" @click="save">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import TablePager from '../../components/TablePager.vue'
import { usePaged } from '../../composables/usePaged'
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import { platform } from '../../api'
import { fmtTime } from '../../utils'
import type { Announcement } from '../../types'

const fLevel = ref('')
const keyword = ref('')
const pg = usePaged((p) => platform.announcementsPage({ ...p, level: fLevel.value, q: keyword.value.trim() }))
const visible = ref(false)
const saving = ref(false)
const editing = ref<Announcement | null>(null)
const form = reactive({ content: '', level: 'info', active: true })

const load = pg.refresh
onMounted(load)

function openCreate() { editing.value = null; Object.assign(form, { content: '', level: 'info', active: true }); visible.value = true }
function openEdit(a: Announcement) { editing.value = a; Object.assign(form, { content: a.content, level: a.level, active: a.active }); visible.value = true }
async function save() {
  saving.value = true
  try {
    if (editing.value) await platform.updateAnnouncement(editing.value.id, { ...form })
    else await platform.createAnnouncement({ ...form })
    ElMessage.success('已保存'); visible.value = false; await load()
  } finally { saving.value = false }
}
async function toggle(a: Announcement, on: boolean) { await platform.updateAnnouncement(a.id, { content: a.content, level: a.level, active: on }); await load() }
async function remove(a: Announcement) { await platform.deleteAnnouncement(a.id); ElMessage.success('已删除'); await load() }
</script>
