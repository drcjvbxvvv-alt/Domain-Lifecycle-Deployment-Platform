import { http } from '@/utils/http'
import type {
  NotificationChannelResponse,
  NotificationRuleResponse,
  NotificationHistoryResponse,
  CreateNotificationChannelRequest,
  UpdateNotificationChannelRequest,
  CreateNotificationRuleRequest,
} from '@/types/notification'

type ListResp<T> = { items: T[]; total: number }

export const notificationApi = {
  // ── Channels ────────────────────────────────────────────────────────────────
  listChannels: () =>
    http.get<ListResp<NotificationChannelResponse>>('/notifications/channels'),

  getChannel: (id: number) =>
    http.get<NotificationChannelResponse>(`/notifications/channels/${id}`),

  createChannel: (data: CreateNotificationChannelRequest) =>
    http.post<NotificationChannelResponse>('/notifications/channels', data),

  updateChannel: (id: number, data: UpdateNotificationChannelRequest) =>
    http.put<NotificationChannelResponse>(`/notifications/channels/${id}`, data),

  deleteChannel: (id: number) =>
    http.delete(`/notifications/channels/${id}`),

  testChannel: (id: number) =>
    http.post(`/notifications/channels/${id}/test`),

  // ── Rules ────────────────────────────────────────────────────────────────────
  listChannelRules: (channelId: number) =>
    http.get<ListResp<NotificationRuleResponse>>(`/notifications/channels/${channelId}/rules`),

  listRules: () =>
    http.get<ListResp<NotificationRuleResponse>>('/notification-rules'),

  createRule: (data: CreateNotificationRuleRequest) =>
    http.post<NotificationRuleResponse>('/notification-rules', data),

  updateRule: (id: number, data: Partial<CreateNotificationRuleRequest>) =>
    http.put<NotificationRuleResponse>(`/notification-rules/${id}`, data),

  deleteRule: (id: number) =>
    http.delete(`/notification-rules/${id}`),

  // ── History ──────────────────────────────────────────────────────────────────
  listHistory: (params?: { channel_id?: number; alert_event_id?: number; status?: string; limit?: number; offset?: number }) =>
    http.get<ListResp<NotificationHistoryResponse>>('/notifications/history', { params }),
}
