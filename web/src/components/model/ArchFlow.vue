<template>
  <div class="flow">
    <div class="stage biz" :class="{ hot: !!trace }">
      <div class="no">业务域</div>
      <div class="ttl">调用统一 API</div>
      <div class="sub">携带模型名 <code>model</code></div>
      <div v-if="trace" class="live"><code>{{ trace.requested_name }}</code></div>
    </div>
    <div class="link"><i /><span>model</span></div>

    <div class="stage match" :class="{ hot: !!trace }">
      <div class="no">① 厂商 / 供应商匹配</div>
      <div class="ttl">模型名 → 厂商 → 供应商</div>
      <div class="sub">确定模型的研发厂商与提供它的全部供应商；<code>供应商/模型</code> 可指定渠道</div>
      <div v-if="trace" class="live">
        <template v-if="trace.matched.length">
          <span v-for="v in matchedVendors" :key="v" class="pill vend">{{ v }}</span>
          <span v-for="m in trace.matched" :key="m.id" class="pill"><VendorLogo :code="m.provider.code" :size="16" />{{ m.provider.name }}</span>
        </template>
        <span v-else class="miss">未匹配到已上架模型</span>
      </div>
    </div>
    <div class="link"><i /><span>model + 厂商<br />(源)</span></div>

    <div class="stage sched" :class="{ hot: !!trace }">
      <div class="no">② 模型调度</div>
      <div class="ttl">API Key + 模型 + 供应商</div>
      <div class="sub">Key 专属策略 &gt; 全局策略 &gt; 厂商的供应商选择（成本 / 优先级 / 权重）</div>
      <div v-if="trace" class="live">
        <StatusBadge :text="scopeText[trace.scope]" :tone="trace.scope === 'key' ? 'accent' : trace.scope === 'global' ? 'primary' : 'info'" />
        <span v-if="trace.policies?.length" class="pol">{{ policyNames }}</span>
        <span v-if="trace.candidates?.length" class="muted">· {{ STRATEGY_TEXT[trace.strategy] }}</span>
      </div>
    </div>
    <div class="link"><i /><span>model + 供应商<br />(目的)</span></div>

    <div class="stage proxy" :class="{ hot: !!trace }">
      <div class="no">③ 供应商服务代理</div>
      <div class="ttl">转发至选定的供应商</div>
      <div class="sub">失败自动切换下一候选，分钟级容灾</div>
      <ol v-if="trace?.candidates?.length" class="live cands">
        <li v-for="(c, i) in trace.candidates" :key="i"><b>{{ i + 1 }}</b><VendorLogo :code="c.model.provider.code" :size="16" />{{ c.model.provider.name }} / <span class="mono">{{ c.model.model_key }}</span></li>
      </ol>
    </div>

    <div class="vendors">
      <span class="vl">厂商</span>
      <span v-for="v in vendorList" :key="v.id" class="v" :class="{ on: activeVendorIds.has(v.id) }"><VendorLogo :code="v.code" :size="18" />{{ v.name }}<small>{{ v.suppliers.filter((s) => s.status === 'active').length }}</small></span>
    </div>
    <div class="vendors sup">
      <span class="vl">供应商</span>
      <span v-for="p in suppliers" :key="p.id" class="v" :class="{ on: activeSuppliers.has(p.code) }"><VendorLogo :code="p.code" :size="18" />{{ p.name }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import VendorLogo from '../VendorLogo.vue'
import StatusBadge from '../StatusBadge.vue'
import { STRATEGY_TEXT } from '../../constants'
import { useLookups } from '../../composables/useLookups'
import type { RouteTrace } from '../../types'

const props = defineProps<{ trace?: RouteTrace | null }>()
const { state } = useLookups()

const scopeText = { key: 'Key 专属策略', global: '全局策略', direct: '厂商供应商选择（无策略）' } as const
const vendorList = computed(() => state.vendors.filter((v) => v.status === 'active'))
const suppliers = computed(() => state.providers.filter((p) => p.status === 'active'))
const activeSuppliers = computed(() => new Set((props.trace?.candidates ?? []).map((c) => c.model.provider.code)))
const activeVendorIds = computed(() => new Set((props.trace?.matched ?? []).map((m) => m.vendor_id).filter(Boolean) as number[]))
const matchedVendors = computed(() => [...new Set((props.trace?.matched ?? []).map((m) => m.vendor?.name).filter(Boolean) as string[])])
const policyNames = computed(() => [...new Set((props.trace?.policies ?? []).map((p) => p.name || p.alias))].join('、'))
</script>

<style scoped>
.vendors .vl { font-size: 11.5px; color: var(--text-3); margin-right: 2px; min-width: 40px; }
.vendors .v small { margin-left: 3px; color: var(--text-3); font-size: 10.5px; }
.vendors.sup { margin-top: 6px !important; padding-top: 0 !important; border-top: none !important; }
.pill.vend { background: var(--accent-soft); color: var(--accent); border-color: transparent; font-weight: 500; }
.flow { display: grid; grid-template-columns: 1fr auto 1.15fr auto 1.15fr auto 1.35fr; align-items: stretch; gap: 0; }
.stage { padding: 12px 14px; border: 1px solid var(--border); border-radius: 10px; background: #FAFBFC; min-width: 0; }
.stage.hot { background: #fff; }
.biz.hot { border-color: #BFD0FA; }
.match { background: #FCF7FF; border-color: #E9D8FD; }
.sched { background: #FFF6F5; border-color: #F9D5D2; }
.proxy { background: #FFFBEB; border-color: #F8E6B0; }
.no { font-size: 12px; font-weight: 600; color: var(--text-2); margin-bottom: 4px; }
.ttl { font-size: 13.5px; font-weight: 600; }
.sub { margin-top: 3px; font-size: 12px; color: var(--text-2); line-height: 1.5; }
.sub code { padding: 0 4px; border-radius: 3px; background: rgba(0, 0, 0, .05); font-family: var(--mono); }
.live { margin-top: 10px; padding-top: 10px; border-top: 1px dashed rgba(0, 0, 0, .12); display: flex; flex-wrap: wrap; align-items: center; gap: 6px; font-size: 12.5px; }
.pill { display: inline-flex; align-items: center; gap: 5px; padding: 2px 8px 2px 4px; border-radius: 999px; background: #fff; border: 1px solid var(--border); }
.miss { color: var(--danger); }
.pol { font-weight: 500; }
.cands { list-style: none; margin: 10px 0 0; padding: 10px 0 0; display: grid; gap: 4px; }
.cands li { display: flex; align-items: center; gap: 6px; }
.cands b { display: inline-flex; align-items: center; justify-content: center; width: 16px; height: 16px; border-radius: 50%; background: var(--accent); color: #fff; font-size: 10px; }
code { font-family: var(--mono); font-size: 12px; }
.link { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; padding: 0 6px; min-width: 64px; font-size: 11px; color: var(--text-3); text-align: center; line-height: 1.3; }
.link i { display: block; width: 100%; height: 1px; background: #CBD5E1; position: relative; }
.link i::after { content: ''; position: absolute; right: 0; top: -3px; border: 3.5px solid transparent; border-left: 6px solid #CBD5E1; }
.vendors { grid-column: 1 / -1; display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; padding-top: 12px; border-top: 1px dashed var(--border); }
.v { display: inline-flex; align-items: center; gap: 6px; padding: 3px 10px 3px 5px; border-radius: 999px; border: 1px solid var(--border); font-size: 12px; color: var(--text-2); background: #fff; }
.v.on { border-color: #C4B5FD; background: var(--accent-soft); color: #5B21B6; font-weight: 500; }
@media (max-width: 1100px) { .flow { grid-template-columns: 1fr; gap: 8px; } .link { flex-direction: row; min-width: 0; padding: 0; } .link i { width: 18px; } }
</style>
