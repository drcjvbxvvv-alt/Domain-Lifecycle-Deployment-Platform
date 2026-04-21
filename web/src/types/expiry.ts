export interface ExpiryBand {
  status: string
  count:  number
}

export interface CalendarEntry {
  date:  string  // YYYY-MM-DD
  count: number
}

export interface ExpiryDashboardData {
  domain_bands:   ExpiryBand[]
  total_expiring: number
  calendar:       CalendarEntry[]
}

export const BAND_CONFIG: Record<string, { label: string; color: string; emoji: string }> = {
  expired:      { label: '已過期',       color: '#ef4444', emoji: '❌' },
  grace:        { label: 'Grace Period', color: '#f97316', emoji: '⏳' },
  expiring_7d:  { label: '7 天內到期',   color: '#f59e0b', emoji: '🔴' },
  expiring_30d: { label: '30 天內到期',  color: '#eab308', emoji: '⚠️' },
  expiring_90d: { label: '90 天內到期',  color: '#3b82f6', emoji: 'ℹ️' },
}
