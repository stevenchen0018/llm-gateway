import { computed, reactive } from 'vue'
import { transfer } from '../api'
import type { TransferEntity } from '../types'

// The importable / exportable entities for the signed-in user, loaded once.
const state = reactive({ list: [] as TransferEntity[], loaded: false, loading: null as Promise<void> | null })

export function loadTransferEntities(force = false) {
  if (state.loaded && !force) return Promise.resolve()
  if (state.loading) return state.loading
  state.loading = transfer.entities().then((l) => { state.list = l; state.loaded = true }).finally(() => { state.loading = null })
  return state.loading
}

export function useTransferEntity(name: string) {
  loadTransferEntities()
  return computed(() => state.list.find((e) => e.name === name))
}

export function useTransferEntities() {
  loadTransferEntities()
  return state
}
