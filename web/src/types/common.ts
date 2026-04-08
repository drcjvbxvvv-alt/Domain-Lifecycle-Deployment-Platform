// ── API response wrappers ──────────────────────────────────────────────────
export interface ApiResponse<T> {
  code: number
  data: T
  message: string
}

export interface PaginatedData<T> {
  items: T[]
  total: number
  cursor?: string
}

// ── Shared field types ─────────────────────────────────────────────────────
export type Role = 'viewer' | 'operator' | 'release_manager' | 'admin' | 'auditor'

export type DomainStatus =
  | 'inactive'
  | 'deploying'
  | 'active'
  | 'degraded'
  | 'switching'
  | 'suspended'
  | 'failed'
  | 'blocked'
  | 'retired'

export type PoolStatus = 'pending' | 'standby' | 'active' | 'blocked' | 'retired'

export type ReleaseStatus =
  | 'pending'
  | 'running'
  | 'paused'
  | 'completed'
  | 'failed'
  | 'rolled_back'

export type AlertSeverity = 'P0' | 'P1' | 'P2' | 'P3' | 'INFO'
