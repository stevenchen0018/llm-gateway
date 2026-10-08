import { computed, ref, watch, type Ref } from 'vue'
import type { PageResult } from '../types'

// The page size is a per-viewer preference shared by every table.
const SIZE_KEY = 'llmgw_page_size'
export const PAGE_SIZES = [10, 20, 50, 100]
function savedSize(): number {
  try {
    const n = Number(localStorage.getItem(SIZE_KEY))
    return PAGE_SIZES.includes(n) ? n : 20
  } catch { return 20 }
}
function saveSize(n: number) { try { localStorage.setItem(SIZE_KEY, String(n)) } catch { /* private mode */ } }

/**
 * usePaged drives a server-paged table: call `load()` to (re)fetch the current
 * page, `reset()` after a filter change (back to page 1), `refresh()` after a
 * mutation (stays on the page, steps back if the page became empty).
 */
export function usePaged<T>(fetcher: (p: { page: number; page_size: number }) => Promise<PageResult<T>>) {
  const items = ref([]) as Ref<T[]>
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(savedSize())
  const loading = ref(false)
  let seq = 0

  async function load() {
    const my = ++seq
    loading.value = true
    try {
      const res = await fetcher({ page: page.value, page_size: pageSize.value })
      if (my !== seq) return // a newer request superseded this one
      items.value = res.items
      total.value = res.total
      // deleting the last row of the last page: step back one page
      if (!res.items.length && res.total > 0 && page.value > 1) {
        page.value = Math.ceil(res.total / pageSize.value)
        await load()
      }
    } finally { if (my === seq) loading.value = false }
  }
  function reset() { page.value = 1; return load() }
  const refresh = load

  watch(pageSize, (n) => { saveSize(n); page.value = 1; load() })
  watch(page, () => load())

  // debounced reset for keyword inputs
  let timer: ReturnType<typeof setTimeout> | undefined
  function search() { clearTimeout(timer); timer = setTimeout(reset, 300) }

  return { items, total, page, pageSize, loading, load, reset, refresh, search }
}

/**
 * useLocalPage pages an in-memory list (for views that must hold the whole
 * set anyway, e.g. grouped routing policies or computed capacity rows).
 */
export function useLocalPage<T>(list: Ref<T[]>) {
  const page = ref(1)
  const pageSize = ref(savedSize())
  watch(pageSize, (n) => { saveSize(n); page.value = 1 })
  watch(() => list.value.length, () => {
    const last = Math.max(1, Math.ceil(list.value.length / pageSize.value))
    if (page.value > last) page.value = last
  })
  const rows = computed(() => list.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
  const total = computed(() => list.value.length)
  return { page, pageSize, rows, total }
}
