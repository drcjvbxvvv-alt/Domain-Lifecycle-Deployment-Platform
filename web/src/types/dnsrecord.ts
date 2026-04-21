// DNS record management types for the staged-edit + plan/apply workflow.
// Mirrors pkg/provider/dns/safety.go Plan/SafetyResult types.

// ── Record types ──────────────────────────────────────────────────────────────

export type DNSRecordType =
  | 'A' | 'AAAA' | 'CNAME' | 'MX' | 'TXT'
  | 'NS' | 'SRV' | 'CAA' | 'PTR'

export type DNSChangeAction = 'create' | 'update' | 'delete'

/** A DNS record as managed by the platform — may have a staged local change. */
export interface ManagedRecord {
  /** Provider-assigned record ID. Undefined for newly staged creates. */
  id?: string
  type: string
  name: string
  content: string
  ttl: number
  priority?: number
  proxied?: boolean
  /** True if this record is in the platform's desired state (managed). */
  managed: boolean
  /**
   * Staged local action. Only set on records that the user has edited locally
   * but not yet applied. Undefined = no pending change for this record.
   */
  _action?: DNSChangeAction
  /** Temp ID for new (unsaved) records, to allow table row-key deduplication. */
  _tempId?: string
}

// ── Plan types (mirror pkg/provider/dns/safety.go) ────────────────────────────

export interface PlanChange {
  action: DNSChangeAction
  before?: ManagedRecord   // nil for creates
  after?: ManagedRecord    // nil for deletes
}

export interface Plan {
  zone: string
  creates: PlanChange[]
  updates: PlanChange[]
  deletes: PlanChange[]
}

/** Computed total number of changes in a plan. */
export function planTotalChanges(plan: Plan): number {
  return plan.creates.length + plan.updates.length + plan.deletes.length
}

// ── Safety types (mirror pkg/provider/dns/safety.go) ─────────────────────────

export interface SafetyResult {
  passed: boolean
  reason?: string
  update_pct: number
  delete_pct: number
  update_threshold: number
  delete_threshold: number
  existing_count: number
  root_ns_changed: boolean
  requires_force: boolean
}

/**
 * Client-side safety check — mirrors CheckSafety() in pkg/provider/dns/safety.go.
 * Returns a SafetyResult without any network call.
 */
export function checkSafety(
  plan: Plan,
  existingCount: number,
  opts: {
    updateThreshold?: number   // default 0.3
    deleteThreshold?: number   // default 0.3
    minExisting?: number       // default 10
    protectRootNS?: boolean    // default true
  } = {}
): SafetyResult {
  const updateThreshold = opts.updateThreshold ?? 0.3
  const deleteThreshold = opts.deleteThreshold ?? 0.3
  const minExisting     = opts.minExisting     ?? 10
  const protectRootNS   = opts.protectRootNS   ?? true

  const zone = plan.zone
  const rootNSChanged = hasRootNSChange(plan, zone)

  const updatePct = existingCount > 0 ? plan.updates.length / existingCount : 0
  const deletePct = existingCount > 0 ? plan.deletes.length / existingCount : 0

  const base: SafetyResult = {
    passed: true,
    update_pct: updatePct,
    delete_pct: deletePct,
    update_threshold: updateThreshold,
    delete_threshold: deleteThreshold,
    existing_count: existingCount,
    root_ns_changed: rootNSChanged,
    requires_force: false,
  }

  // Small zone: bypass percentage checks (but still check root NS)
  if (existingCount < minExisting) {
    if (protectRootNS && rootNSChanged) {
      return { ...base, passed: false, requires_force: true, reason: '根 NS 變更需要 force' }
    }
    return base
  }

  if (updatePct > updateThreshold) {
    return {
      ...base, passed: false, requires_force: true,
      reason: `將更新 ${Math.round(updatePct * 100)}% 的記錄（閾值：${Math.round(updateThreshold * 100)}%）`,
    }
  }
  if (deletePct > deleteThreshold) {
    return {
      ...base, passed: false, requires_force: true,
      reason: `將刪除 ${Math.round(deletePct * 100)}% 的記錄（閾值：${Math.round(deleteThreshold * 100)}%）`,
    }
  }
  if (protectRootNS && rootNSChanged) {
    return { ...base, passed: false, requires_force: true, reason: '根 NS 變更需要 force' }
  }

  return base
}

function hasRootNSChange(plan: Plan, zone: string): boolean {
  const zoneName = zone.replace(/\.$/, '').toLowerCase()
  const isRootNS = (r?: ManagedRecord): boolean => {
    if (!r) return false
    if (r.type.toUpperCase() !== 'NS') return false
    const name = r.name.replace(/\.$/, '').toLowerCase()
    return name === '@' || name === '' || name === zoneName
  }

  return (
    plan.creates.some(c => isRootNS(c.after)) ||
    plan.updates.some(c => isRootNS(c.before) || isRootNS(c.after)) ||
    plan.deletes.some(c => isRootNS(c.before))
  )
}

// ── Validation ────────────────────────────────────────────────────────────────

export interface ValidationError {
  field: string
  message: string
}

const IPv4_RE = /^(\d{1,3}\.){3}\d{1,3}$/
const IPv6_RE = /^[0-9a-fA-F:]+$/
const HOSTNAME_RE = /^[a-zA-Z0-9]([a-zA-Z0-9\-\.]*[a-zA-Z0-9])?\.?$/

/** Validates a DNS record's fields. Returns list of errors (empty = valid). */
export function validateRecord(r: Partial<ManagedRecord>): ValidationError[] {
  const errors: ValidationError[] = []

  if (!r.name?.trim()) errors.push({ field: 'name', message: '名稱為必填' })
  if (!r.type?.trim()) errors.push({ field: 'type', message: '類型為必填' })
  if (!r.content?.trim()) errors.push({ field: 'content', message: '值為必填' })
  if (!r.ttl || r.ttl < 1) errors.push({ field: 'ttl', message: 'TTL 必須 ≥ 1' })

  const type = (r.type || '').toUpperCase()
  const content = r.content || ''

  switch (type) {
    case 'A':
      if (!IPv4_RE.test(content)) {
        errors.push({ field: 'content', message: 'A 記錄需要合法的 IPv4 地址（如 1.2.3.4）' })
      } else {
        const parts = content.split('.').map(Number)
        if (parts.some(p => p > 255)) {
          errors.push({ field: 'content', message: 'IPv4 每段數值需介於 0-255' })
        }
      }
      break

    case 'AAAA':
      if (!IPv6_RE.test(content)) {
        errors.push({ field: 'content', message: 'AAAA 記錄需要合法的 IPv6 地址' })
      }
      break

    case 'CNAME':
      if (!HOSTNAME_RE.test(content)) {
        errors.push({ field: 'content', message: 'CNAME 需要合法的主機名稱' })
      }
      break

    case 'MX':
      if (r.priority === undefined || r.priority < 0) {
        errors.push({ field: 'priority', message: 'MX 記錄需要優先級（0-65535）' })
      }
      if (!HOSTNAME_RE.test(content)) {
        errors.push({ field: 'content', message: 'MX 郵件伺服器需要合法的主機名稱' })
      }
      break

    case 'TXT': {
      // TXT records max 255 chars per segment; we enforce total < 2048
      if (content.length > 2048) {
        errors.push({ field: 'content', message: 'TXT 記錄總長度不可超過 2048 字元' })
      }
      break
    }

    case 'NS':
      if (!HOSTNAME_RE.test(content)) {
        errors.push({ field: 'content', message: 'NS 記錄需要合法的名稱伺服器主機名稱' })
      }
      break

    case 'CAA':
      // CAA format: <flags> <tag> <value>
      if (!/^\d+ (issue|issuewild|iodef) ".+"$/.test(content)) {
        errors.push({ field: 'content', message: 'CAA 格式：<flags> <tag> "<value>"（如：0 issue "letsencrypt.org"）' })
      }
      break
  }

  return errors
}
