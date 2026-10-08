export const fmtNum = (n: number | string) => Number(n).toLocaleString('en-US')

// Costs are tiny per call; show enough precision to be meaningful.
export const fmtMoney = (v: number | string, currency = '¥') => {
  const n = Number(v)
  const a = Math.abs(n)
  const digits = a === 0 ? 2 : a < 0.01 ? 6 : a < 1 ? 4 : 2
  return `${currency}${n.toLocaleString('en-US', { minimumFractionDigits: digits, maximumFractionDigits: digits })}`
}

export const fmtTime = (s?: string | null) => (s ? new Date(s).toLocaleString('zh-CN', { hour12: false }) : '-')

export const fmtDate = (d: Date) => {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

export const limitText = (n: number, unit: string) => (n > 0 ? `${fmtNum(n)} ${unit}` : '不限')

export const fmtPct = (n: number, digits = 2) => `${n.toFixed(digits)}%`

export const fmtDuration = (ms: number) => (ms >= 10_000 ? `${(ms / 1000).toFixed(1)} s` : `${fmtNum(Math.round(ms))} ms`)

// datetime-local style string the backend accepts (local time)
export const fmtDateTime = (d: Date) => `${fmtDate(d)}T${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:00`

export interface Range { from: Date; to: Date; preset: string }

const PRESET_MS: Record<string, number> = { '1h': 3600e3, '3h': 3 * 3600e3, '6h': 6 * 3600e3, '24h': 24 * 3600e3, '7d': 7 * 86400e3 }

export function defaultRange(preset = '3h'): Range {
  const to = new Date()
  return { preset, from: new Date(to.getTime() - PRESET_MS[preset]), to }
}

// Client-side pre-check for IP whitelist entries (the server validates too).
const V4 = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/
export function isIPOrCIDR(s: string): boolean {
  const [addr, bits, extra] = s.split('/')
  if (extra !== undefined || !addr) return false
  const v6 = addr.includes(':')
  if (v6 ? !/^[0-9a-fA-F:.]+$/.test(addr) || (addr.match(/::/g)?.length ?? 0) > 1 : !V4.test(addr)) return false
  if (bits === undefined) return true
  return /^\d{1,3}$/.test(bits) && Number(bits) <= (v6 ? 128 : 32)
}
