<template>
  <el-drawer :model-value="visible" size="480px" :with-header="false" @update:model-value="$emit('update:visible', $event)">
    <div v-if="model" class="dwr">
      <div class="head">
        <VendorLogo :code="model.vendor?.code ?? model.provider?.code ?? ''" :size="44" />
        <div class="ttl">
          <h3>{{ model.display_name }}</h3>
          <div class="muted mono">{{ model.model_key }}</div>
        </div>
        <StatusBadge :text="model.status === 'active' ? '已上架' : '已下架'" :tone="model.status === 'active' ? 'success' : 'info'" />
      </div>
      <div class="tags">
        <span class="tag cat">{{ categoryLabel(model.category) }}</span>
        <span v-for="t in extraTags" :key="t" class="tag">{{ t }}</span>
      </div>
      <p class="desc">{{ model.description || '暂无描述' }}</p>

      <h5>基本信息</h5>
      <dl class="grid">
        <div><dt>厂商</dt><dd>{{ model.vendor?.name ?? '—' }}</dd></div>
        <div><dt>当前供应商</dt><dd>{{ model.provider?.name }}<span v-if="model.provider" class="muted">（{{ SUPPLIER_TYPE[model.provider.supplier_type]?.text }}）</span></dd></div>
        <div><dt>类型</dt><dd>{{ model.type === 'chat' ? '对话 / 生成' : '向量 / 重排' }}</dd></div>
        <div><dt>上下文长度</dt><dd class="num">{{ fmtContext(model.context_length) }}</dd></div>
        <div><dt>发布时间</dt><dd>{{ model.released_at ? model.released_at.slice(0, 10) : '—' }}</dd></div>
      </dl>

      <h5>定价（每 1K tokens）</h5>
      <dl class="grid">
        <div><dt>输入标价</dt><dd class="num">{{ fmtMoney(model.input_price_per_1k) }}</dd></div>
        <div><dt>输出标价</dt><dd class="num">{{ fmtMoney(model.output_price_per_1k) }}</dd></div>
        <div><dt>供应商折扣</dt><dd class="num">{{ discountText }}</dd></div>
        <div><dt>折后输入 / 输出</dt><dd class="num">{{ fmtMoney(Number(model.input_price_per_1k) * rate) }} / {{ fmtMoney(Number(model.output_price_per_1k) * rate) }}</dd></div>
      </dl>

      <template v-if="(offers?.length ?? 0) > 1">
        <h5>供应商对比 <span class="muted" style="font-weight: 400">· {{ offers!.length }} 家供应商提供 {{ model.display_name }}<template v-if="model.vendor">；无调度策略时按「{{ VENDOR_ROUTING[model.vendor.routing_strategy]?.text }}」选择</template></span></h5>
        <el-table :data="offers" size="small" class="offers" @row-click="(r: Model) => $emit('select', r)">
          <el-table-column label="供应商" min-width="130"><template #default="{ row }"><span :class="{ cur: row.id === model.id }">{{ row.provider?.name }}</span><div class="cell-sub">{{ SUPPLIER_TYPE[row.provider?.supplier_type]?.text }}<template v-if="row.supply"> · 优先级 {{ row.supply.priority }}</template></div></template></el-table-column>
          <el-table-column label="折后 输入/输出" min-width="140" align="right"><template #default="{ row }"><span class="num">{{ fmtMoney(effective(row, 'in')) }} / {{ fmtMoney(effective(row, 'out')) }}</span></template></el-table-column>
          <el-table-column label="TPM" width="70" align="right"><template #default="{ row }"><span class="num">{{ row.tpm_limit ? compact(row.tpm_limit) : '不限' }}</span></template></el-table-column>
          <el-table-column label="状态" width="72"><template #default="{ row }"><StatusBadge :text="offerState(row).text" :tone="offerState(row).tone" /></template></el-table-column>
        </el-table>
      </template>

      <h5>容量阈值</h5>
      <dl class="grid">
        <div><dt>TPM 上限</dt><dd class="num">{{ model.tpm_limit ? fmtNum(model.tpm_limit) : '不限' }}</dd></div>
        <div><dt>QPS 上限</dt><dd class="num">{{ model.qps_limit ? fmtNum(model.qps_limit) : '不限' }}</dd></div>
      </dl>

      <h5>近 7 日表现</h5>
      <div v-loading="loading" class="stats">
        <template v-if="stat">
          <div><span>调用量</span><b class="num">{{ fmtNum(stat.requests) }}</b></div>
          <div><span>Token</span><b class="num">{{ compact(stat.total_tokens) }}</b></div>
          <div><span>失败率</span><b class="num" :class="{ bad: stat.failure_rate > 5 }">{{ stat.failure_rate.toFixed(2) }}%</b></div>
          <div><span>平均 RT</span><b class="num">{{ fmtDuration(stat.avg_latency_ms) }}</b></div>
          <div><span>成本</span><b class="num">{{ fmtMoney(stat.cost) }}</b></div>
        </template>
        <div v-else-if="!loading" class="faint">近 7 日暂无调用</div>
      </div>

      <div class="actions">
        <el-button type="primary" :icon="VideoPlay" @click="$emit('try', model)">立即体验</el-button>
        <el-button :icon="Document" @click="$emit('guide', model)">接入说明</el-button>
        <el-button :icon="mine ? StarFilled : Star" @click="$emit('toggle-mine', model)">{{ mine ? '已加入我的模型' : '加入我的模型' }}</el-button>
      </div>
      <div v-if="manage" class="admin">
        <span class="muted">模型管理</span>
        <el-button link type="primary" @click="$emit('edit', model)">编辑</el-button>
        <el-button link :type="model.status === 'active' ? 'danger' : 'primary'" @click="$emit('toggle-status', model)">{{ model.status === 'active' ? '下架' : '上架' }}</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Document, Star, StarFilled, VideoPlay } from '@element-plus/icons-vue'
import VendorLogo from '../VendorLogo.vue'
import StatusBadge from '../StatusBadge.vue'
import { monitor } from '../../api'
import { SUPPLIER_TYPE, VENDOR_ROUTING, categoryLabel, fmtContext } from '../../constants'
import { compact } from '../../charts'
import { fmtDateTime, fmtDuration, fmtMoney, fmtNum } from '../../utils'
import type { Model, RankRow } from '../../types'

const props = defineProps<{ visible: boolean; model: Model | null; mine?: boolean; manage?: boolean; offers?: Model[] }>()
defineEmits<{
  'update:visible': [v: boolean]; edit: [m: Model]; guide: [m: Model]; try: [m: Model]
  'toggle-mine': [m: Model]; 'toggle-status': [m: Model]; select: [m: Model]
}>()

const effective = (m: Model, side: 'in' | 'out') => Number(side === 'in' ? m.input_price_per_1k : m.output_price_per_1k) * (Number(m.provider?.discount_rate ?? 1) || 1)
const offerState = (m: Model) => (m.status !== 'active' ? { text: '下架', tone: 'info' as const }
  : m.provider?.status === 'disabled' || m.supply?.status === 'disabled' ? { text: '停用', tone: 'warning' as const } : { text: '可用', tone: 'success' as const })

const extraTags = computed(() => (props.model?.tags ?? []).filter((t) => t !== categoryLabel(props.model?.category ?? '')))
const rate = computed(() => Number(props.model?.provider?.discount_rate ?? 1) || 1)
const discountText = computed(() => (rate.value >= 1 ? '无折扣' : `${(rate.value * 10).toFixed(1).replace(/\.0$/, '')} 折`))

const stat = ref<RankRow | null>(null)
const loading = ref(false)
watch(() => [props.visible, props.model?.id], async () => {
  if (!props.visible || !props.model) return
  loading.value = true
  stat.value = null
  try {
    const to = new Date(), from = new Date(to.getTime() - 7 * 86400e3)
    const rows = await monitor.ranking({ group_by: 'model', model_id: props.model.id, from: fmtDateTime(from), to: fmtDateTime(to) })
    stat.value = rows[0] ?? null
  } finally { loading.value = false }
}, { immediate: true })
</script>

<style scoped>
.offers { border: 1px solid var(--border-soft); border-radius: 8px; }
.offers :deep(tr) { cursor: pointer; }
.cur { font-weight: 600; color: var(--primary); }
.dwr { padding: 4px 4px 24px; }
.head { display: flex; align-items: center; gap: 12px; }
.ttl { flex: 1; min-width: 0; }
h3 { margin: 0; font-size: 17px; }
.tags { display: flex; flex-wrap: wrap; gap: 4px; margin: 14px 0 6px; }
.tag { padding: 0 8px; height: 22px; line-height: 22px; border-radius: 4px; background: #F3F4F6; color: var(--text-2); font-size: 12px; }
.tag.cat { background: var(--primary-soft); color: #1D4ED8; }
.desc { color: var(--text-2); line-height: 1.7; margin: 8px 0 4px; }
h5 { margin: 18px 0 8px; font-size: 13px; font-weight: 600; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 16px; margin: 0; padding: 12px 14px; background: #FAFBFC; border: 1px solid var(--border-soft); border-radius: 8px; }
.grid div { min-width: 0; }
dt { font-size: 12px; color: var(--text-2); }
dd { margin: 2px 0 0; font-size: 13px; }
.stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; min-height: 56px; }
.stats > div { padding: 10px 12px; border: 1px solid var(--border-soft); border-radius: 8px; }
.stats span { display: block; font-size: 12px; color: var(--text-2); }
.stats b { font-size: 16px; font-weight: 600; }
.stats b.bad { color: var(--danger); }
.actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 22px; padding-top: 16px; border-top: 1px solid var(--border-soft); }
.admin { display: flex; align-items: center; gap: 12px; margin-top: 12px; font-size: 13px; }
.grid dd { text-align: left; }
</style>
