<template>
  <el-dialog :model-value="!!secret" :title="title" width="540px" :close-on-click-modal="false" @update:model-value="(v: boolean) => !v && $emit('close')">
    <el-alert type="warning" :closable="false" show-icon title="请立即复制并妥善保管" description="系统只保存密钥的哈希；关闭此窗口后无法再次查看完整密钥，遗失只能重置。" />
    <div class="secret mono">{{ secret }}</div>
    <template #footer>
      <el-button :icon="CopyDocument" @click="copy">复制密钥</el-button>
      <el-button type="primary" @click="$emit('close')">我已保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
// SecretDialog shows a freshly issued API key secret exactly once.
import { ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'

const props = withDefaults(defineProps<{ secret: string; title?: string }>(), { title: 'Key 已分发' })
defineEmits<{ close: [] }>()
async function copy() { await navigator.clipboard.writeText(props.secret); ElMessage.success('已复制') }
</script>

<style scoped>
.secret { margin-top: 16px; padding: 12px 14px; background: #F9FAFB; border: 1px solid var(--border); border-radius: var(--radius-sm); word-break: break-all; font-size: 13px; user-select: all; }
</style>
