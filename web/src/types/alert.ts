import type { AlertSeverity } from './common'

export interface AlertResponse {
  uuid:       string
  severity:   AlertSeverity
  domain:     string | null
  probe_node: string | null
  alert_type: string
  message:    string
  acked_by:   number | null
  acked_at:   string | null
  created_at: string
}
