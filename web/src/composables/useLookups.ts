import { computed, reactive } from 'vue'
import { keys as keysApi, models as modelsApi, platform, providers as providersApi, tenancy, vendors as vendorsApi } from '../api'
import type { ApiKey, Application, DepartmentView, Model, Provider, VendorView } from '../types'

// Session-wide cache of the reference data every screen needs to turn ids into names.
const state = reactive({
  providers: [] as Provider[],
  models: [] as Model[],
  keys: [] as ApiKey[],
  apps: [] as Application[],
  depts: [] as DepartmentView[],
  vendors: [] as VendorView[],
  loaded: false,
  loading: null as Promise<void> | null,
})

export async function loadLookups(force = false) {
  if (state.loaded && !force) return
  if (state.loading) return state.loading
  state.loading = (async () => {
    const [p, m, k, a, d, v] = await Promise.all([providersApi.list(), modelsApi.list(), keysApi.list(), platform.applications(), tenancy.departments(), vendorsApi.list()])
    state.vendors = v
    state.providers = p
    state.models = m
    state.keys = k
    state.apps = a
    state.depts = d
    state.loaded = true
  })().finally(() => { state.loading = null })
  return state.loading
}

export function useLookups() {
  loadLookups()

  const providerById = computed(() => new Map(state.providers.map((p) => [p.id, p])))
  const modelById = computed(() => new Map(state.models.map((m) => [m.id, m])))
  const keyById = computed(() => new Map(state.keys.map((k) => [k.id, k])))
  const appById = computed(() => new Map(state.apps.map((a) => [a.id, a])))

  // the same model name is often served by several vendors: disambiguate with the vendor
  const dupNames = computed(() => {
    const c = new Map<string, number>()
    for (const m of state.models) c.set(m.display_name, (c.get(m.display_name) ?? 0) + 1)
    return new Set([...c].filter(([, n]) => n > 1).map(([k]) => k))
  })

  const vendorById = computed(() => new Map(state.vendors.map((v) => [v.id, v])))
  const vendorName = (id?: number | null) => (id ? vendorById.value.get(id)?.name ?? `#${id}` : '—')
  const providerName = (id?: number | null) => (id ? providerById.value.get(id)?.name ?? `#${id}` : '—')
  const keyName = (id?: number | null) => (id ? keyById.value.get(id)?.name ?? `#${id}` : '—')
  const deptName = (id?: number | null) => (id ? state.depts.find((d) => d.id === id)?.name ?? `#${id}` : '—')
  const appName = (id?: number | null) => (id ? appById.value.get(id)?.name ?? `#${id}` : '未归属')
  const modelName = (id?: number | null) => (id ? modelById.value.get(id)?.display_name ?? `#${id}` : '—')
  const modelLabel = (id?: number | null) => {
    const m = id ? modelById.value.get(id) : undefined
    if (!m) return id ? `#${id}` : '—'
    return dupNames.value.has(m.display_name) ? `${m.display_name} · ${m.provider?.name ?? providerName(m.provider_id)}` : m.display_name
  }

  return { state, deptName, vendorById, vendorName, providerById, modelById, keyById, appById, providerName, keyName, appName, modelName, modelLabel, reload: () => loadLookups(true) }
}
