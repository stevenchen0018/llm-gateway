// Shared chart styling: recessive grid/axes, thin marks, one hue per single-series
// chart, and a fixed categorical palette for multi-series charts.
import type { Series, SeriesPoint, SeriesResult } from './types'

export const C = {
  primary: '#2563EB',
  accent: '#7C3AED',
  danger: '#DC2626',
  text: '#111827',
  text2: '#6B7280',
  grid: '#EEF0F3',
  axis: '#D1D5DB',
  surface: '#FFFFFF',
}

export const compact = (n: number) =>
  Math.abs(n) >= 1e9 ? (n / 1e9).toFixed(1).replace(/\.0$/, '') + 'B' : Math.abs(n) >= 1e6 ? (n / 1e6).toFixed(1).replace(/\.0$/, '') + 'M' : Math.abs(n) >= 1e3 ? (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'k' : String(Math.round(n * 100) / 100)

export const tooltipBase = {
  backgroundColor: '#FFFFFF',
  borderColor: '#E5E7EB',
  borderWidth: 1,
  padding: [8, 10],
  textStyle: { color: C.text, fontSize: 12 },
  extraCssText: 'box-shadow:0 4px 12px rgba(17,24,39,.08);border-radius:8px;font-variant-numeric:tabular-nums;',
}

export const tipRow = (color: string, name: string, value: string) =>
  `<div style="display:flex;align-items:center;gap:8px;justify-content:space-between;min-width:160px">` +
  `<span style="display:inline-flex;align-items:center;gap:6px;color:${C.text2}"><i style="width:8px;height:8px;border-radius:2px;background:${color}"></i>${name}</span>` +
  `<b style="font-weight:600">${value}</b></div>`

export const xAxis = (data: string[]) => ({
  type: 'category' as const,
  data,
  axisTick: { show: false },
  axisLine: { lineStyle: { color: C.axis } },
  axisLabel: { color: C.text2, fontSize: 11, margin: 10 },
})

export const yAxis = (fmt: (v: number) => string = compact) => ({
  type: 'value' as const,
  axisLabel: { color: C.text2, fontSize: 11, formatter: fmt },
  axisLine: { show: false },
  axisTick: { show: false },
  splitLine: { lineStyle: { color: C.grid } },
})

// ---------------------------------------------------------------------------
// Categorical palette (validated reference set: adjacent-pair CVD ΔE ≥ 8).
// 7 entity slots + a neutral "其他" — an 8th+ series is folded, never given a
// generated hue.
// ---------------------------------------------------------------------------
export const SERIES_PALETTE = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#4a3aa7']
export const OTHER_COLOR = '#9CA3AF'
export const OTHER_KEY = '__other__'
export const MAX_SERIES = SERIES_PALETTE.length

// Color follows the entity: once an entity holds a slot it keeps it across
// filter changes; newcomers take the lowest slot not used by a current peer.
const slotOf = new Map<string, number>()

export function assignColors(keys: string[]): Record<string, string> {
  const used = new Set<number>()
  const out: Record<string, string> = {}
  for (const k of keys) {
    const s = slotOf.get(k)
    if (s !== undefined && !used.has(s)) used.add(s)
  }
  const taken = new Set<number>()
  for (const k of keys) {
    let s = slotOf.get(k)
    if (s === undefined || taken.has(s)) {
      s = 0
      while (used.has(s) || taken.has(s)) s++
      if (s >= MAX_SERIES) s = MAX_SERIES - 1
      slotOf.set(k, s)
    }
    taken.add(s)
    used.add(s)
    out[k] = SERIES_PALETTE[s]
  }
  return out
}

// ---------------------------------------------------------------------------
// Time-series metrics
// ---------------------------------------------------------------------------
export type MetricKey = 'rpm' | 'tpm' | 'requests' | 'failed' | 'failure_rate' | 'rt' | 'tokens' | 'tokens_prompt' | 'tokens_completion'

interface MetricDef {
  label: string
  // returns null for "no data in this bucket" (ratio metrics), else a number
  value: (p: SeriesPoint, stepMin: number) => number | null
  fmt: (v: number) => string
  axis?: (v: number) => string
  zeroFill: boolean
}

const ms = (v: number) => (v >= 10_000 ? `${(v / 1000).toFixed(1)} s` : `${Math.round(v).toLocaleString('en-US')} ms`)

export const METRICS: Record<MetricKey, MetricDef> = {
  rpm: { label: '每分钟调用次数(RPM)', value: (p, m) => p.requests / m, fmt: (v) => Math.round(v).toLocaleString('en-US'), zeroFill: true },
  tpm: { label: '每分钟消耗 Token 数(TPM)', value: (p, m) => p.total_tokens / m, fmt: (v) => Math.round(v).toLocaleString('en-US'), zeroFill: true },
  requests: { label: '总调用量', value: (p) => p.requests, fmt: (v) => Math.round(v).toLocaleString('en-US'), zeroFill: true },
  failed: { label: '失败数量', value: (p) => p.failed, fmt: (v) => Math.round(v).toLocaleString('en-US'), zeroFill: true },
  failure_rate: { label: '失败率', value: (p) => (p.requests ? (p.failed / p.requests) * 100 : null), fmt: (v) => v.toFixed(2) + '%', axis: (v) => v.toFixed(1) + '%', zeroFill: false },
  rt: { label: '平均响应时间(RT)', value: (p) => (p.requests ? p.latency_sum_ms / p.requests : null), fmt: ms, axis: (v) => compactMs(v), zeroFill: false },
  tokens: { label: '总量', value: (p) => p.total_tokens, fmt: (v) => Math.round(v).toLocaleString('en-US'), zeroFill: true },
  tokens_prompt: { label: '输入', value: (p) => p.prompt_tokens, fmt: (v) => Math.round(v).toLocaleString('en-US'), zeroFill: true },
  tokens_completion: { label: '输出', value: (p) => p.completion_tokens, fmt: (v) => Math.round(v).toLocaleString('en-US'), zeroFill: true },
}

function compactMs(v: number) {
  return v >= 1000 ? `${(v / 1000).toFixed(v >= 10_000 ? 0 : 1)}s` : `${Math.round(v)}ms`
}

const sumPoints = (a: SeriesPoint, b: SeriesPoint): SeriesPoint => ({
  ts: a.ts,
  requests: a.requests + b.requests,
  failed: a.failed + b.failed,
  prompt_tokens: a.prompt_tokens + b.prompt_tokens,
  completion_tokens: a.completion_tokens + b.completion_tokens,
  total_tokens: a.total_tokens + b.total_tokens,
  latency_sum_ms: a.latency_sum_ms + b.latency_sum_ms,
  cost: String(Number(a.cost) + Number(b.cost)),
})

// Keep the busiest MAX_SERIES entities and fold the tail into one "其他" series.
export function foldSeries(series: Series[], max = MAX_SERIES): Series[] {
  const weight = (s: Series) => s.points.reduce((t, p) => t + p.requests, 0)
  const sorted = [...series].sort((a, b) => weight(b) - weight(a))
  if (sorted.length <= max) return sorted
  const head = sorted.slice(0, max - 1)
  const byTs = new Map<string, SeriesPoint>()
  for (const s of sorted.slice(max - 1)) {
    for (const p of s.points) byTs.set(p.ts, byTs.has(p.ts) ? sumPoints(byTs.get(p.ts)!, p) : p)
  }
  const other: Series = { group_key: OTHER_KEY, points: [...byTs.values()].sort((a, b) => +new Date(a.ts) - +new Date(b.ts)) }
  return [...head, other]
}

const pad = (n: number) => String(n).padStart(2, '0')
const fmtTs = (t: number) => {
  const d = new Date(t)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export interface TsOptionInput {
  result: SeriesResult
  metric: MetricKey
  names: Record<string, string> // group_key -> display name
  area?: boolean
  fold?: boolean
}

// Build an ECharts option for a multi-series time chart: gap-filled time axis,
// thin lines, scrollable legend, hover tooltip listing series by value.
export function buildTsOption({ result, metric, names, area = false, fold = true }: TsOptionInput) {
  const def = METRICS[metric]
  const step = result.step_seconds
  const stepMin = step / 60
  const series = fold ? foldSeries(result.series) : result.series
  const realKeys = series.filter((s) => s.group_key !== OTHER_KEY).map((s) => s.group_key)
  const colors = assignColors(realKeys)
  const colorOf = (k: string) => (k === OTHER_KEY ? OTHER_COLOR : colors[k])
  const nameOf = (k: string) => (k === OTHER_KEY ? '其他' : names[k] ?? `#${k}`)

  const start = Math.floor(new Date(result.from).getTime() / 1000 / step) * step
  const end = new Date(result.to).getTime() / 1000
  const ticks: number[] = []
  for (let t = start; t < end && ticks.length < 4000; t += step) ticks.push(t * 1000)

  const seriesOpt = series.map((s) => {
    const byTs = new Map(s.points.map((p) => [Math.floor(new Date(p.ts).getTime() / 1000) * 1000, p]))
    const data = ticks.map((t) => {
      const p = byTs.get(t)
      const v = p ? def.value(p, stepMin) : def.zeroFill ? 0 : null
      return [t, v] as [number, number | null]
    })
    return {
      name: nameOf(s.group_key),
      type: 'line' as const,
      data,
      showSymbol: false,
      symbolSize: 7,
      lineStyle: { width: 1.6, color: colorOf(s.group_key) },
      itemStyle: { color: colorOf(s.group_key), borderColor: C.surface, borderWidth: 2 },
      areaStyle: area ? { color: colorOf(s.group_key), opacity: 0.06 } : undefined,
      emphasis: { focus: 'series' as const, lineStyle: { width: 2.2 } },
      connectNulls: false,
    }
  })

  return {
    grid: { left: 58, right: 16, top: series.length > 1 ? 38 : 16, bottom: 28 },
    legend: {
      type: 'scroll' as const, top: 0, left: 0, right: 8, itemWidth: 14, itemHeight: 3, itemGap: 14,
      textStyle: { color: C.text2, fontSize: 11 }, pageIconSize: 10, pageTextStyle: { color: C.text2 },
      show: series.length > 1,
    },
    tooltip: {
      ...tooltipBase, trigger: 'axis' as const, axisPointer: { type: 'line' as const, lineStyle: { color: C.axis } },
      formatter: (ps: { seriesName: string; value: [number, number | null]; color: string; axisValue: number }[]) => {
        const rows = ps.filter((p) => p.value?.[1] != null).sort((a, b) => (b.value[1] as number) - (a.value[1] as number)).slice(0, 10)
        if (!rows.length) return ''
        return `<div style="color:${C.text2};margin-bottom:4px">${fmtTs(ps[0].axisValue)}</div>` + rows.map((r) => tipRow(r.color, r.seriesName, def.fmt(r.value[1] as number))).join('')
      },
    },
    xAxis: {
      type: 'time' as const, min: ticks[0], max: ticks[ticks.length - 1],
      axisTick: { show: false }, axisLine: { lineStyle: { color: C.axis } },
      axisLabel: { color: C.text2, fontSize: 11, hideOverlap: true },
      splitLine: { show: false },
    },
    yAxis: yAxis(def.axis ?? ((v: number) => compact(v))),
    series: seriesOpt,
  }
}

// ---------------------------------------------------------------------------
// Horizontal ranking bars (failure rate / RT by model). One hue; the value is
// labelled at the bar end; mouse-wheel scrolls through rows beyond maxRows.
// ---------------------------------------------------------------------------
export interface RankInput {
  rows: { name: string; value: number }[]
  fmt: (v: number) => string
  color?: string
  maxRows?: number
  seriesName: string
}

export function buildRankOption({ rows, fmt, color = C.primary, maxRows = 12, seriesName }: RankInput) {
  const sorted = [...rows].sort((a, b) => b.value - a.value)
  const data = [...sorted].reverse() // echarts draws the first category at the bottom
  const visible = Math.min(maxRows, data.length)
  const start = Math.max(0, 100 - (visible / Math.max(data.length, 1)) * 100)
  return {
    grid: { left: 8, right: 64, top: 8, bottom: 8, containLabel: true },
    tooltip: {
      ...tooltipBase, trigger: 'axis' as const, axisPointer: { type: 'shadow' as const, shadowStyle: { color: 'rgba(17,24,39,.04)' } },
      formatter: (p: { name: string; value: number }[]) => `<div style="color:${C.text2};margin-bottom:4px">${p[0].name}</div>` + tipRow(color, seriesName, fmt(p[0].value)),
    },
    xAxis: { type: 'value' as const, axisLabel: { color: C.text2, fontSize: 11, formatter: (v: number) => (v >= 1000 ? compact(v) : String(v)) }, splitLine: { lineStyle: { color: C.grid } }, axisLine: { show: false } },
    yAxis: {
      type: 'category' as const, data: data.map((r) => r.name), axisTick: { show: false }, axisLine: { lineStyle: { color: C.axis } },
      axisLabel: { color: C.text2, fontSize: 11, width: 130, overflow: 'truncate' as const },
    },
    dataZoom: [{ type: 'inside' as const, yAxisIndex: 0, start, end: 100, zoomOnMouseWheel: false, moveOnMouseWheel: true, moveOnMouseMove: false, filterMode: 'empty' as const }],
    series: [{
      name: seriesName, type: 'bar' as const, data: data.map((r) => r.value), barMaxWidth: 12,
      itemStyle: { color, borderRadius: [0, 4, 4, 0] },
      label: { show: true, position: 'right' as const, color: C.text2, fontSize: 11, formatter: (p: { value: number }) => fmt(p.value) },
    }],
  }
}
