// notification.ts — mirrors notification_channels / notification_rules / notification_history Go DTOs

export type ChannelType = 'telegram' | 'slack' | 'webhook' | 'email'
export type HistoryStatus = 'sent' | 'failed' | 'suppressed'
export type Severity = 'P1' | 'P2' | 'P3' | 'INFO'

// ── Channels ──────────────────────────────────────────────────────────────────

export interface NotificationChannelResponse {
  id:           number
  uuid:         string
  name:         string
  channel_type: ChannelType
  config:       string | Record<string, unknown>  // redacted in list; full in detail
  is_default:   boolean
  enabled:      boolean
  created_by:   number | null
  created_at:   string
  updated_at:   string
}

export interface CreateNotificationChannelRequest {
  name:         string
  channel_type: ChannelType
  config:       Record<string, unknown>
  is_default:   boolean
  enabled:      boolean
}

export interface UpdateNotificationChannelRequest {
  name?:         string
  channel_type?: ChannelType
  config?:       Record<string, unknown>
  is_default?:   boolean
  enabled?:      boolean
}

// ── Channel configs (type-specific) ──────────────────────────────────────────

export interface TelegramConfig {
  bot_token: string
  chat_id:   string
}

export interface SlackConfig {
  webhook_url: string
  username?:   string
  channel?:    string
}

export interface WebhookConfig {
  url: string
}

export interface EmailConfig {
  smtp_host:     string
  smtp_port:     number
  username?:     string
  password?:     string
  from_address:  string
  to_addresses:  string[]
  use_tls?:      boolean
  use_starttls?: boolean
}

// ── Rules ─────────────────────────────────────────────────────────────────────

export interface NotificationRuleResponse {
  id:           number
  channel_id:   number
  alert_type:   string | null
  min_severity: Severity
  target_type:  string | null
  target_id:    number | null
  enabled:      boolean
  created_at:   string
}

export interface CreateNotificationRuleRequest {
  channel_id:   number
  alert_type?:  string | null
  min_severity: Severity
  target_type?: string | null
  target_id?:   number | null
  enabled:      boolean
}

// ── History ───────────────────────────────────────────────────────────────────

export interface NotificationHistoryResponse {
  id:             number
  channel_id:     number
  alert_event_id: number | null
  status:         HistoryStatus
  message:        string | null
  error:          string | null
  sent_at:        string
}
