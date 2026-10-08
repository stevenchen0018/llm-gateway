<template>
  <el-dialog :model-value="visible" :title="`批量导入 · ${entity.title}`" width="820px" :close-on-click-modal="false" destroy-on-close
    @update:model-value="(v: boolean) => !v && close()">
    <el-steps :active="step" finish-status="success" simple class="steps">
      <el-step title="上传文件" />
      <el-step title="校验预览" />
      <el-step title="导入结果" />
    </el-steps>

    <!-- 1. upload -->
    <div v-if="step === 0" class="pane">
      <div class="tpl">
        <div>
          <b>第一步：下载模板并填写</b>
          <div class="hint">{{ entity.desc }}。模板含下拉选项与填写说明 Sheet，带 * 的列必填；也可直接使用「导出」的文件修改后再导入。</div>
        </div>
        <span class="tpl-btns">
          <el-button :icon="Download" @click="downloadTpl('xlsx')">Excel 模板</el-button>
          <el-button link type="primary" @click="downloadTpl('csv')">CSV 模板</el-button>
        </span>
      </div>
      <el-upload drag :auto-upload="false" :show-file-list="false" accept=".xlsx,.csv" :on-change="onFile" class="drop">
        <el-icon :size="34" class="up-ico"><UploadFilled /></el-icon>
        <div v-if="!file" class="up-t">将文件拖到此处，或<em>点击选择</em></div>
        <div v-else class="up-t"><el-icon><Document /></el-icon> <b>{{ file.name }}</b><span class="muted">（{{ (file.size / 1024).toFixed(1) }} KB）· 点击更换</span></div>
        <div class="up-h">支持 .xlsx / .csv（UTF-8 或 GBK），不超过 5 MB、5000 行</div>
      </el-upload>
      <div class="opts">
        <span class="lab">已存在的记录</span>
        <el-radio-group v-model="onConflict">
          <el-radio value="skip">跳过，保留原数据</el-radio>
          <el-radio value="update" :disabled="entity.name === 'keys'">用文件内容更新</el-radio>
        </el-radio-group>
        <span v-if="entity.name === 'keys'" class="hint">Key 导入只新建申请，不修改已有 Key</span>
      </div>
      <el-collapse class="cols">
        <el-collapse-item :title="`字段说明（${columns.length} 列）`">
          <el-table :data="columns" size="small" max-height="260">
            <el-table-column label="列名" width="140"><template #default="{ row }">{{ row.title }}<span v-if="row.required" class="req">*</span></template></el-table-column>
            <el-table-column label="说明" min-width="260"><template #default="{ row }"><span class="muted">{{ row.desc || '—' }}</span></template></el-table-column>
            <el-table-column label="可选值 / 示例" min-width="200" show-overflow-tooltip><template #default="{ row }"><span class="mono">{{ row.options?.join(' / ') || row.example || '—' }}</span></template></el-table-column>
          </el-table>
        </el-collapse-item>
      </el-collapse>
    </div>

    <!-- 2. validation preview -->
    <div v-else-if="step === 1 && result" class="pane">
      <div class="sum">
        <div><span>数据行</span><b class="num">{{ result.total }}</b></div>
        <div class="ok"><span>将新增</span><b class="num">{{ result.create }}</b></div>
        <div class="up"><span>将更新</span><b class="num">{{ result.update }}</b></div>
        <div><span>将跳过</span><b class="num">{{ result.skip }}</b></div>
        <div :class="{ bad: result.invalid > 0 }"><span>有错误</span><b class="num">{{ result.invalid }}</b></div>
      </div>
      <el-alert v-if="result.invalid === 0" type="success" :closable="false" show-icon :title="`全部 ${result.total} 行校验通过，确认后开始导入`" />
      <el-alert v-else type="warning" :closable="false" show-icon :title="`${result.invalid} 行存在错误`">
        <template #default>可下载错误报告修正后重新上传；或勾选「跳过错误行」，只导入校验通过的 {{ result.valid }} 行。</template>
      </el-alert>
      <ResultTables :result="result" />
      <div class="foot-opts">
        <el-checkbox v-if="result.invalid > 0" v-model="skipInvalid" :disabled="result.valid === 0">跳过错误行，只导入通过的 {{ result.valid }} 行</el-checkbox>
      </div>
    </div>

    <!-- 3. result -->
    <div v-else-if="step === 2 && result" class="pane">
      <el-result :icon="result.failed > 0 || result.invalid > 0 ? 'warning' : 'success'" :title="result.failed > 0 || result.invalid > 0 ? '导入完成（部分行未导入）' : '导入完成'">
        <template #sub-title>
          新增 <b>{{ result.created }}</b> · 更新 <b>{{ result.updated }}</b> · 跳过 <b>{{ result.skipped }}</b>
          <template v-if="result.failed + result.invalid"> · 未导入 <b class="bad">{{ result.failed + result.invalid }}</b></template>
        </template>
      </el-result>
      <ResultTables v-if="result.errors.length" :result="result" errors-only />
    </div>

    <template #footer>
      <div class="footer">
        <el-button v-if="result?.errors.length" link type="primary" :icon="Download" @click="downloadErrors">下载错误报告</el-button>
        <span class="spacer" />
        <template v-if="step === 0">
          <el-button @click="close">取消</el-button>
          <el-button type="primary" :loading="busy" :disabled="!file" @click="validate">下一步：校验</el-button>
        </template>
        <template v-else-if="step === 1">
          <el-button @click="step = 0">重新上传</el-button>
          <el-button type="primary" :loading="busy" :disabled="!canCommit" @click="commit">确认导入{{ result && result.create + result.update ? `（${result.create + result.update} 行）` : '' }}</el-button>
        </template>
        <el-button v-else type="primary" @click="close">完成</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, type PropType } from 'vue'
import { ElMessage, ElTable, ElTableColumn, ElTag, type UploadFile } from 'element-plus'
import { Document, Download, UploadFilled } from '@element-plus/icons-vue'
import { transfer } from '../../api'
import type { ImportResult, TransferEntity } from '../../types'

const props = defineProps<{ visible: boolean; entity: TransferEntity }>()
const emit = defineEmits<{ 'update:visible': [v: boolean]; done: [] }>()

const step = ref(0)
const file = ref<File | null>(null)
const onConflict = ref('skip')
const skipInvalid = ref(false)
const busy = ref(false)
const result = ref<ImportResult | null>(null)
const columns = computed(() => props.entity.columns.filter((c) => !c.export_only))
const canCommit = computed(() => !!result.value && (result.value.invalid === 0 || skipInvalid.value) && result.value.create + result.value.update > 0)

function onFile(f: UploadFile) {
  if (!f.raw) return
  if (f.raw.size > 5 * 1024 * 1024) { ElMessage.error('文件不能超过 5 MB'); return }
  file.value = f.raw
}
async function downloadTpl(format: 'xlsx' | 'csv') {
  try { await transfer.template(props.entity.name, format) } catch (e) { ElMessage.error((e as Error).message) }
}
async function run(mode: 'validate' | 'commit') {
  busy.value = true
  try { return await transfer.import(props.entity.name, file.value!, { mode, on_conflict: onConflict.value, skip_invalid: skipInvalid.value }) } finally { busy.value = false }
}
async function validate() {
  result.value = await run('validate')
  skipInvalid.value = false
  step.value = 1
}
async function commit() {
  const r = await run('commit')
  result.value = r
  if (!r.committed) { ElMessage.warning(r.message || '未导入'); return }
  step.value = 2
  emit('done')
}
function close() {
  emit('update:visible', false)
  setTimeout(() => { step.value = 0; file.value = null; result.value = null; skipInvalid.value = false }, 200)
}
function downloadErrors() {
  const rows = [['行号', '列', '问题'], ...result.value!.errors.map((e) => [String(e.row), e.column, e.message])]
  const csv = '﻿' + rows.map((r) => r.map((c) => `"${c.replace(/"/g, '""')}"`).join(',')).join('\r\n')
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
  a.download = `${props.entity.title}_导入错误报告.csv`
  a.click()
}

const OP = { create: { text: '新增', type: 'success' }, update: { text: '更新', type: 'primary' }, skip: { text: '跳过', type: 'info' } } as const
// error list + row preview, shared by steps 2 and 3
const ResultTables = defineComponent({
  props: { result: { type: Object as PropType<ImportResult>, required: true }, errorsOnly: Boolean },
  setup(p) {
    return () => h('div', { class: 'tables' }, [
      p.result.errors.length ? h('div', [
        h('div', { class: 'tt' }, [`错误明细（${p.result.errors.length}${p.result.errors_truncated ? '+' : ''}）`]),
        h(ElTable, { data: p.result.errors, size: 'small', maxHeight: 220 }, () => [
          h(ElTableColumn, { label: '行号', width: 70, prop: 'row' }),
          h(ElTableColumn, { label: '列', width: 130, prop: 'column' }),
          h(ElTableColumn, { label: '问题', minWidth: 300, prop: 'message' }),
        ]),
      ]) : null,
      !p.errorsOnly && p.result.preview.length ? h('div', [
        h('div', { class: 'tt' }, [`执行预览（前 ${p.result.preview.length} 行）`]),
        h(ElTable, { data: p.result.preview, size: 'small', maxHeight: 220 }, () => [
          h(ElTableColumn, { label: '行号', width: 70, prop: 'row' }),
          h(ElTableColumn, { label: '操作', width: 80 }, { default: ({ row }: { row: ImportResult['preview'][number] }) => h(ElTag, { type: OP[row.op].type, size: 'small', disableTransitions: true }, () => OP[row.op].text) }),
          h(ElTableColumn, { label: '内容', minWidth: 300, prop: 'summary' }),
        ]),
      ]) : null,
    ])
  },
})
</script>

<style scoped>
.steps { margin-bottom: 16px; }
.pane { display: grid; gap: 14px; }
.tpl { display: flex; justify-content: space-between; align-items: center; gap: 16px; padding: 12px 14px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: #FAFBFC; }
.tpl-btns { display: flex; align-items: center; gap: 10px; flex: none; }
.drop :deep(.el-upload-dragger) { padding: 22px 16px; border-radius: var(--radius); }
.up-ico { color: var(--primary); }
.up-t { margin-top: 6px; font-size: 13.5px; display: inline-flex; align-items: center; gap: 4px; }
.up-t em { color: var(--primary); font-style: normal; margin-left: 2px; }
.up-h { margin-top: 4px; font-size: 12px; color: var(--text-3); }
.opts { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.lab { font-size: 13px; color: var(--text-2); }
.req { color: var(--danger); margin-left: 2px; }
.cols :deep(.el-collapse-item__header) { font-size: 13px; color: var(--text-2); height: 38px; }
.sum { display: grid; grid-template-columns: repeat(5, 1fr); gap: 10px; }
.sum > div { padding: 10px 12px; border: 1px solid var(--border); border-radius: var(--radius-sm); }
.sum span { display: block; font-size: 12px; color: var(--text-2); }
.sum b { font-size: 20px; font-weight: 600; }
.sum .ok b { color: var(--success); }
.sum .up b { color: var(--primary); }
.sum .bad { border-color: #FCA5A5; background: var(--danger-soft); }
.sum .bad b, .bad { color: var(--danger); }
:deep(.tables) { display: grid; gap: 12px; }
:deep(.tt) { font-size: 12.5px; font-weight: 600; color: var(--text-2); margin-bottom: 6px; }
.footer { display: flex; align-items: center; gap: 8px; }
.spacer { flex: 1; }
</style>
