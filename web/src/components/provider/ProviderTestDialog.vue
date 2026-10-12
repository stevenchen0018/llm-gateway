<template>
  <el-dialog :model-value="visible" :title="`供应商测试 · ${provider?.name ?? ''}`" width="860px" destroy-on-close @update:model-value="emit('update:visible', $event)" @closed="reset">
    <!-- options -->
    <template v-if="!run">
      <div class="intro">
        <ProtocolTag :provider="provider" />
        <span class="mono muted">{{ provider?.base_url }}</span>
      </div>
      <ol class="steps">
        <li><b>连通性与鉴权</b>：请求供应商的模型列表接口，不消耗 Token</li>
        <li><b>模型调用</b>：对每个选中的模型发起一次最小请求（对话 max_tokens=32，向量输入 “ping”），统计成功率与耗时，会产生极少量费用</li>
      </ol>
      <el-form label-position="top" class="opts">
        <el-form-item :label="`测试模型（已选 ${selected.length} / ${testable.length}）`" class="span2">
          <div class="mlist">
            <el-checkbox :model-value="allChecked" :indeterminate="selected.length > 0 && !allChecked" @change="toggleAll">全选可测模型</el-checkbox>
            <div class="mgrid">
              <el-checkbox v-for="m in models" :key="m.id" v-model="selected" :value="m.id" :disabled="!isTestable(m)">
                <span class="mono">{{ m.model_key }}</span>
                <span v-if="!isTestable(m)" class="faint">（{{ categoryLabel(m.category) }}，不做自动测试）</span>
                <span v-else-if="m.status !== 'active'" class="faint">（已下架）</span>
              </el-checkbox>
              <span v-if="!models.length" class="faint">该供应商暂无模型，仅进行连通性与鉴权测试</span>
            </div>
          </div>
        </el-form-item>
        <el-form-item label="每个模型调用次数"><el-input-number v-model="rounds" :min="1" :max="5" controls-position="right" /><div class="hint">多次调用可得到更稳定的耗时（P95）</div></el-form-item>
        <el-form-item label="单项超时（秒）"><el-input-number v-model="timeout" :min="5" :max="120" :step="5" controls-position="right" /></el-form-item>
        <el-form-item label="测试提示词" class="span2"><el-input v-model="prompt" placeholder="请只回复：OK" maxlength="500" /></el-form-item>
      </el-form>
    </template>

    <!-- running / result -->
    <template v-else>
      <div class="rhead">
        <StatusBadge :text="TEST_RUN_STATUS[run.status]?.text ?? run.status" :tone="TEST_RUN_STATUS[run.status]?.tone ?? 'info'" />
        <span v-if="run.status === 'running'" class="muted"><el-icon class="is-loading"><Loading /></el-icon> 正在测试，已用时 {{ elapsed }} 秒…</span>
        <span v-else class="sum">{{ run.summary }}</span>
        <span class="spacer" />
        <span class="muted num">#{{ run.id }} · {{ fmtTime(run.started_at) }}</span>
      </div>
      <div v-if="run.status !== 'running'" class="kpis">
        <div><label>通过</label><b class="num ok">{{ run.passed }}</b></div>
        <div><label>失败</label><b class="num" :class="{ bad: run.failed }">{{ run.failed }}</b></div>
        <div><label>跳过</label><b class="num">{{ run.skipped }}</b></div>
        <div><label>平均耗时</label><b class="num">{{ run.passed > 1 || run.avg_latency_ms ? fmtDuration(run.avg_latency_ms) : '—' }}</b></div>
        <div><label>P95 耗时</label><b class="num">{{ run.passed > 1 || run.p95_latency_ms ? fmtDuration(run.p95_latency_ms) : '—' }}</b></div>
      </div>
      <TestResultTable v-if="run.results?.length" :results="run.results" max-height="380" />
      <el-skeleton v-else-if="run.status === 'running'" :rows="5" animated />
    </template>

    <template #footer>
      <template v-if="!run">
        <el-button @click="emit('update:visible', false)">取消</el-button>
        <el-button type="primary" :icon="VideoPlay" :loading="starting" @click="start">开始测试</el-button>
      </template>
      <template v-else>
        <el-button :disabled="run.status === 'running'" @click="run = null">重新测试</el-button>
        <el-button v-if="provider" @click="emit('report', provider)">查看测试报告</el-button>
        <el-button type="primary" @click="emit('update:visible', false)">{{ run.status === 'running' ? '后台运行' : '完成' }}</el-button>
      </template>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Loading, VideoPlay } from '@element-plus/icons-vue'
import StatusBadge from '../StatusBadge.vue'
import ProtocolTag from './ProtocolTag.vue'
import TestResultTable from './TestResultTable.vue'
import { providerTests } from '../../api'
import { useLookups } from '../../composables/useLookups'
import { TEST_RUN_STATUS, categoryLabel } from '../../constants'
import { fmtDuration, fmtTime } from '../../utils'
import type { Model, Provider, ProviderTestRun } from '../../types'

const props = defineProps<{ visible: boolean; provider: Provider | null }>()
const emit = defineEmits<{ 'update:visible': [boolean]; report: [Provider]; done: [ProviderTestRun] }>()

const { state } = useLookups()
const UNTESTED = ['image_gen', 'video_gen', 'speech_asr', 'speech_tts', 'rerank']
const isTestable = (m: Model) => !UNTESTED.includes(m.category)
const models = computed(() => state.models.filter((m) => m.provider_id === props.provider?.id).sort((a, b) => a.model_key.localeCompare(b.model_key)))
const testable = computed(() => models.value.filter(isTestable))

const selected = ref<number[]>([])
const rounds = ref(1)
const timeout = ref(30)
const prompt = ref('')
const starting = ref(false)
const run = ref<ProviderTestRun | null>(null)
const elapsed = ref(0)
let timer: number | undefined

const allChecked = computed(() => testable.value.length > 0 && selected.value.length === testable.value.length)
const toggleAll = (on: unknown) => { selected.value = on ? testable.value.map((m) => m.id) : [] }

watch(() => [props.visible, props.provider?.id], ([v]) => {
  if (v) selected.value = testable.value.filter((m) => m.status === 'active').map((m) => m.id)
}, { immediate: true })

async function start() {
  if (!props.provider) return
  starting.value = true
  try {
    // no model selected: probe only
    const ids = selected.value.length ? selected.value : [-1]
    run.value = await providerTests.start(props.provider.id, { model_ids: ids, rounds: rounds.value, timeout_seconds: timeout.value, prompt: prompt.value })
    poll()
  } finally { starting.value = false }
}

function poll() {
  stop()
  const began = Date.now()
  timer = window.setInterval(async () => {
    elapsed.value = Math.round((Date.now() - began) / 1000)
    if (!run.value || run.value.status !== 'running') return stop()
    try {
      const r = await providerTests.run(run.value.id)
      if (r.status !== 'running') { run.value = r; stop(); emit('done', r) }
    } catch { stop() }
  }, 1000)
}
function stop() { if (timer) { clearInterval(timer); timer = undefined } }
function reset() { stop(); run.value = null; elapsed.value = 0; prompt.value = '' }
onBeforeUnmount(stop)
</script>

<style scoped>
.intro { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.steps { margin: 0 0 14px; padding: 10px 14px 10px 30px; background: #F8FAFC; border: 1px solid var(--border-soft); border-radius: var(--radius-sm); color: var(--text-2); font-size: 12.5px; line-height: 22px; }
.steps b { color: var(--text); font-weight: 500; }
.opts { display: grid; grid-template-columns: 1fr 1fr; column-gap: 16px; }
.opts .span2 { grid-column: 1 / -1; }
.mlist { width: 100%; border: 1px solid var(--border); border-radius: var(--radius-sm); padding: 6px 12px; }
.mgrid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: 12px; max-height: 200px; overflow: auto; border-top: 1px solid var(--border-soft); margin-top: 4px; padding-top: 4px; }
.mgrid .el-checkbox { margin-right: 0; min-width: 0; }
.mgrid :deep(.el-checkbox__label) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hint { width: 100%; font-size: 12px; color: var(--text-3); line-height: 18px; margin-top: 4px; }
.rhead { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.rhead .sum { color: var(--text-2); }
.spacer { flex: 1; }
.kpis { display: grid; grid-template-columns: repeat(5, 1fr); gap: 10px; margin-bottom: 14px; }
.kpis div { border: 1px solid var(--border); border-radius: var(--radius-sm); padding: 8px 12px; }
.kpis label { display: block; font-size: 12px; color: var(--text-2); }
.kpis b { font-size: 18px; font-weight: 600; }
.kpis .ok { color: #15803D; }
.kpis .bad { color: var(--danger); }
</style>
