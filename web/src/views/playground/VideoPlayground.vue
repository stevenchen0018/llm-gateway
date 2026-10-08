<template>
  <div>
    <PageHeader title="模型体验 · 视频生成" desc="文生视频 / 图生视频：任务异步生成，可同时提交多个任务对比不同模型的效果与耗时" />
    <div class="layout">
      <Panel title="生成参数">
        <el-form label-position="top">
          <el-form-item label="模型"><ModelSelect v-model="modelId" :categories="['video_gen']" /></el-form-item>
          <el-form-item label="画面描述">
            <el-input v-model="prompt" type="textarea" :rows="5" resize="none" maxlength="800" show-word-limit placeholder="描述镜头、主体、动作与风格，如：航拍镜头缓缓推进，清晨雾气中的梯田，电影感" />
            <div class="chips"><button v-for="s in samples" :key="s" type="button" @click="prompt = s">{{ s }}</button></div>
          </el-form-item>
          <el-form-item label="首帧图片（可选，图生视频）">
            <div class="frame">
              <div v-if="image" class="thumb"><img :src="image" alt="首帧" /><el-button link type="danger" size="small" @click="image = ''">移除</el-button></div>
              <el-upload v-else :auto-upload="false" :show-file-list="false" accept="image/png,image/jpeg,image/webp" :on-change="onFile">
                <el-button :icon="Upload">上传图片</el-button>
              </el-upload>
              <span class="hint">PNG / JPG / WebP，≤ 5 MB</span>
            </div>
          </el-form-item>
          <div class="row2">
            <el-form-item label="画幅">
              <el-radio-group v-model="size">
                <el-radio-button v-for="s in SIZES" :key="s.value" :value="s.value">{{ s.label }}</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="时长">
              <el-radio-group v-model="seconds"><el-radio-button v-for="n in [5, 10]" :key="n" :value="n">{{ n }} 秒</el-radio-button></el-radio-group>
            </el-form-item>
          </div>
          <el-button type="primary" :loading="submitting" :disabled="!modelId || !prompt.trim()" @click="submit">提交生成任务</el-button>
          <div class="hint" style="margin-top: 8px">视频生成通常需要数十秒到数分钟，提交后可继续提交其他任务，结果会自动刷新。</div>
        </el-form>
      </Panel>

      <Panel title="生成任务" :sub="tasks.length ? `${running} 个进行中` : ''">
        <EmptyState v-if="!tasks.length" text="提交的视频生成任务将显示在这里" height="300px" />
        <div v-else class="tasks">
          <article v-for="t in tasks" :key="t.task.id" class="task">
            <div class="media" :class="{ wide: t.task.size !== '720x1280' }">
              <template v-if="t.task.status === 'succeeded' && t.url">
                <img v-if="t.url.startsWith('data:image')" :src="t.url" :alt="t.prompt" />
                <video v-else :src="t.url" controls playsinline preload="metadata" />
              </template>
              <div v-else-if="t.task.status === 'failed'" class="state fail"><el-icon :size="22"><CircleClose /></el-icon><span>{{ t.task.error || '生成失败' }}</span></div>
              <div v-else class="state">
                <el-progress type="circle" :percentage="t.task.progress" :width="64" :stroke-width="5" />
                <span>{{ t.task.status === 'queued' ? '排队中…' : '生成中…' }}</span>
              </div>
            </div>
            <div class="info">
              <div class="p">{{ t.prompt }}</div>
              <div class="m num muted">{{ t.model }} · {{ t.task.size }} · {{ t.task.seconds }}s<template v-if="t.hasImage"> · 图生视频</template> · {{ elapsed(t) }}</div>
              <div class="acts">
                <StatusBadge :text="STATUS[t.task.status].text" :tone="STATUS[t.task.status].tone" />
                <a v-if="t.url" :href="t.url" :download="`video-${t.task.id}.${t.url.startsWith('data:image/svg') ? 'svg' : 'mp4'}`">下载</a>
                <el-button link size="small" @click="remove(t.task.id)">移除</el-button>
              </div>
              <div v-if="t.url?.startsWith('data:image')" class="hint">Mock 厂商返回动态预览；接入真实厂商后此处播放 MP4。</div>
            </div>
          </article>
        </div>
      </Panel>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { ElMessage, type UploadFile } from 'element-plus'
import { CircleClose, Upload } from '@element-plus/icons-vue'
import PageHeader from '../../components/PageHeader.vue'
import Panel from '../../components/Panel.vue'
import EmptyState from '../../components/EmptyState.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import ModelSelect from '../../components/ModelSelect.vue'
import { playground } from '../../api'
import { useLookups } from '../../composables/useLookups'
import type { VideoTask } from '../../types'

const { modelName } = useLookups()
const SIZES = [{ value: '1280x720', label: '16:9' }, { value: '720x1280', label: '9:16' }, { value: '960x960', label: '1:1' }]
const STATUS = {
  queued: { text: '排队中', tone: 'info' }, running: { text: '生成中', tone: 'primary' },
  succeeded: { text: '已完成', tone: 'success' }, failed: { text: '失败', tone: 'danger' },
} as const
const samples = ['航拍镜头缓缓推进，清晨雾气中的梯田，电影感', '一只橘猫在窗台上伸懒腰，午后阳光，慢动作', '赛博朋克城市夜景，霓虹灯下的雨夜街道，镜头横移']

const modelId = ref<number>()
const prompt = ref('')
const size = ref('1280x720')
const seconds = ref(5)
const image = ref('')
const submitting = ref(false)

interface Job { task: VideoTask; modelId: number; model: string; prompt: string; hasImage: boolean; url?: string; startedAt: number; doneAt?: number }
const tasks = ref<Job[]>([])
const running = computed(() => tasks.value.filter((t) => t.task.status === 'queued' || t.task.status === 'running').length)
const now = ref(Date.now())

function onFile(f: UploadFile) {
  if (!f.raw) return
  if (f.raw.size > 5 * 1024 * 1024) { ElMessage.error('图片不能超过 5 MB'); return }
  const r = new FileReader()
  r.onload = () => { image.value = String(r.result) }
  r.readAsDataURL(f.raw)
}

async function submit() {
  submitting.value = true
  try {
    const task = await playground.submitVideo({ model_id: modelId.value!, prompt: prompt.value.trim(), size: size.value, seconds: seconds.value, image: image.value || undefined })
    tasks.value.unshift({ task, modelId: modelId.value!, model: modelName(modelId.value), prompt: prompt.value.trim(), hasImage: !!image.value, startedAt: Date.now() })
    ElMessage.success('任务已提交')
  } finally { submitting.value = false }
}

// poll every unfinished task; stop polling a task once it is done
async function poll() {
  now.value = Date.now()
  for (const t of tasks.value) {
    if (t.task.status === 'succeeded' || t.task.status === 'failed') continue
    try {
      const next = await playground.video(t.modelId, t.task.id)
      t.task = next
      if (next.status === 'succeeded') {
        t.doneAt = Date.now()
        t.url = next.content_proxied ? await playground.videoBlobUrl(t.modelId, next.id) : next.video_url
      } else if (next.status === 'failed') t.doneAt = Date.now()
    } catch { /* transient: retry on the next tick */ }
  }
}
const timer = setInterval(poll, 2000)
onBeforeUnmount(() => {
  clearInterval(timer)
  for (const t of tasks.value) if (t.url?.startsWith('blob:')) URL.revokeObjectURL(t.url)
})

function remove(id: string) {
  const t = tasks.value.find((x) => x.task.id === id)
  if (t?.url?.startsWith('blob:')) URL.revokeObjectURL(t.url)
  tasks.value = tasks.value.filter((x) => x.task.id !== id)
}
const elapsed = (t: Job) => `${Math.round(((t.doneAt ?? now.value) - t.startedAt) / 1000)}s`
</script>

<style scoped>
.layout { display: grid; grid-template-columns: minmax(0, 4fr) minmax(0, 7fr); gap: 16px; align-items: start; }
@media (max-width: 960px) { .layout { grid-template-columns: 1fr; } }
.chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
.chips button { padding: 3px 10px; border: 1px solid var(--border); border-radius: 999px; background: #fff; font: inherit; font-size: 12px; color: var(--text-2); cursor: pointer; text-align: left; }
.chips button:hover { border-color: var(--primary); color: var(--primary); }
.frame { display: flex; align-items: center; gap: 12px; }
.thumb { display: flex; align-items: center; gap: 8px; }
.thumb img { width: 72px; height: 48px; object-fit: cover; border-radius: 6px; border: 1px solid var(--border); }
.row2 { display: flex; gap: 24px; flex-wrap: wrap; }
.tasks { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 14px; }
.task { border: 1px solid var(--border); border-radius: 10px; overflow: hidden; background: #fff; }
.media { aspect-ratio: 9 / 16; max-height: 360px; margin: 0 auto; background: #0F172A; display: flex; align-items: center; justify-content: center; }
.media.wide { aspect-ratio: 16 / 9; max-height: none; }
.media img, .media video { width: 100%; height: 100%; object-fit: contain; display: block; }
.state { display: flex; flex-direction: column; align-items: center; gap: 10px; color: #CBD5E1; font-size: 12.5px; padding: 12px; text-align: center; }
.state :deep(.el-progress__text) { color: #E2E8F0; }
.state.fail { color: #FCA5A5; }
.info { padding: 10px 12px; font-size: 12.5px; display: grid; gap: 4px; }
.p { line-height: 1.5; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.m { font-size: 11.5px; }
.acts { display: flex; align-items: center; gap: 12px; }
</style>
