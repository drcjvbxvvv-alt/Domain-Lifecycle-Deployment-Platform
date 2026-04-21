// DNS record management store — staged-edit + plan/apply pattern.
// Follows PowerDNS-Admin's "staged-edit + batch-apply" UX design.
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { dnsApi } from '@/api/dns'
import type { ManagedRecord, Plan, PlanChange, SafetyResult } from '@/types/dnsrecord'
import { checkSafety } from '@/types/dnsrecord'
import type { ProviderRecord } from '@/types/dns'

let _tempIdCounter = 0
function nextTempId(): string {
  return `__new_${++_tempIdCounter}`
}

/** Convert a ProviderRecord (from API) to a ManagedRecord. */
function toManaged(r: ProviderRecord): ManagedRecord {
  return {
    id:       r.id,
    type:     r.type,
    name:     r.name,
    content:  r.content,
    ttl:      r.ttl,
    priority: r.priority,
    proxied:  r.proxied,
    managed:  true,
  }
}

export const useDNSRecordStore = defineStore('dnsrecord', () => {
  /** Domain ID this store is currently managing. */
  const domainId = ref<number | null>(null)

  /** FQDN of the current domain (used as zone name for safety checks). */
  const fqdn = ref<string>('')

  /** Records as fetched from the provider API — the source of truth. */
  const providerRecords = ref<ManagedRecord[]>([])

  /** Working copy — reflects local staged changes. */
  const stagedRecords = ref<ManagedRecord[]>([])

  /** Loading state for fetch operations. */
  const loading = ref(false)

  /** True if an apply operation is in progress. */
  const applying = ref(false)

  /** Computed: records that have a pending local action. */
  const pendingChanges = computed<ManagedRecord[]>(() =>
    stagedRecords.value.filter(r => r._action)
  )

  /** Computed: true when there is at least one staged change. */
  const hasPendingChanges = computed(() => pendingChanges.value.length > 0)

  /** Computed: structured diff between staged and provider state. */
  const plan = computed<Plan>(() => {
    const zone = fqdn.value
    const creates: PlanChange[] = []
    const updates: PlanChange[] = []
    const deletes: PlanChange[] = []

    for (const staged of stagedRecords.value) {
      if (!staged._action) continue

      if (staged._action === 'create') {
        creates.push({ action: 'create', after: staged })
      } else if (staged._action === 'update') {
        const before = providerRecords.value.find(p => p.id === staged.id)
        updates.push({ action: 'update', before, after: staged })
      } else if (staged._action === 'delete') {
        const before = providerRecords.value.find(p => p.id === staged.id)
        deletes.push({ action: 'delete', before })
      }
    }

    return { zone, creates, updates, deletes }
  })

  /** Computed: safety check result for the current plan. */
  const safetyResult = computed<SafetyResult>(() =>
    checkSafety(plan.value, providerRecords.value.length)
  )

  // ── Actions ───────────────────────────────────────────────────────────────

  /** Fetch provider records and reset staged state. */
  async function fetchRecords(dId: number, domainFqdn: string) {
    domainId.value = dId
    fqdn.value = domainFqdn
    loading.value = true
    try {
      const res = await dnsApi.listProviderRecords(dId) as unknown as { data: { items: ProviderRecord[] } }
      const items = res.data?.items ?? []
      providerRecords.value = items.map(toManaged)
      // Reset staged to a fresh copy of provider records (no pending changes)
      stagedRecords.value = items.map(r => ({ ...toManaged(r) }))
    } finally {
      loading.value = false
    }
  }

  /**
   * Stage a new record (create).
   * The record is added to staged state with _action='create'.
   * It will be sent to the provider API on apply.
   */
  function stageCreate(record: Omit<ManagedRecord, 'id' | 'managed' | '_action' | '_tempId'>) {
    const staged: ManagedRecord = {
      ...record,
      managed: true,
      _action: 'create',
      _tempId: nextTempId(),
    }
    stagedRecords.value = [...stagedRecords.value, staged]
  }

  /**
   * Stage an edit to an existing record.
   * Replaces the record in staged state with _action='update'.
   */
  function stageUpdate(id: string, changes: Partial<ManagedRecord>) {
    stagedRecords.value = stagedRecords.value.map(r => {
      if (r.id !== id) return r
      const existing = providerRecords.value.find(p => p.id === id)
      // If reverting to original values, clear the action
      const updated = { ...r, ...changes }
      const isReverted = existing &&
        updated.type    === existing.type    &&
        updated.name    === existing.name    &&
        updated.content === existing.content &&
        updated.ttl     === existing.ttl     &&
        (updated.priority ?? 0) === (existing.priority ?? 0)
      return { ...updated, _action: isReverted ? undefined : 'update' }
    })
  }

  /**
   * Stage a record for deletion.
   * Marks the record in staged state with _action='delete'.
   */
  function stageDelete(id: string) {
    stagedRecords.value = stagedRecords.value.map(r =>
      r.id === id ? { ...r, _action: 'delete' as const } : r
    )
  }

  /** Unstage a staged create (remove the temp row). */
  function removeStagedCreate(tempId: string) {
    stagedRecords.value = stagedRecords.value.filter(r => r._tempId !== tempId)
  }

  /** Discard all staged changes — revert to provider state. */
  function discardChanges() {
    stagedRecords.value = providerRecords.value.map(r => ({ ...r }))
  }

  /**
   * Apply all staged changes to the provider via existing CRUD endpoints.
   * Executes creates, then updates, then deletes in sequence.
   * Refreshes from provider on success.
   *
   * Returns { success: true } or { success: false, error: string }
   */
  async function applyPlan(dId: number, domainFqdn: string): Promise<{ success: boolean; error?: string }> {
    applying.value = true
    const currentPlan = plan.value
    try {
      // Creates
      for (const change of currentPlan.creates) {
        if (!change.after) continue
        const { type, name, content, ttl, priority, proxied } = change.after
        await dnsApi.createProviderRecord(dId, { type, name, content, ttl, priority, proxied })
      }

      // Updates
      for (const change of currentPlan.updates) {
        if (!change.after?.id) continue
        const { id, type, name, content, ttl, priority, proxied } = change.after
        await dnsApi.updateProviderRecord(dId, id!, { type, name, content, ttl, priority, proxied })
      }

      // Deletes
      for (const change of currentPlan.deletes) {
        if (!change.before?.id) continue
        await dnsApi.deleteProviderRecord(dId, change.before.id)
      }

      // Refresh from provider
      await fetchRecords(dId, domainFqdn)
      return { success: true }
    } catch (err: any) {
      return {
        success: false,
        error: err?.response?.data?.message ?? err?.message ?? '套用失敗',
      }
    } finally {
      applying.value = false
    }
  }

  return {
    // state
    domainId, fqdn, providerRecords, stagedRecords, loading, applying,
    // computed
    pendingChanges, hasPendingChanges, plan, safetyResult,
    // actions
    fetchRecords, stageCreate, stageUpdate, stageDelete,
    removeStagedCreate, discardChanges, applyPlan,
  }
})
