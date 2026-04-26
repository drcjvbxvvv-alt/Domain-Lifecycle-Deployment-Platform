import { defineStore } from 'pinia'
import { ref } from 'vue'
import { notificationApi } from '@/api/notification'
import type { ApiResponse } from '@/types/common'
import type {
  NotificationChannelResponse,
  NotificationRuleResponse,
  NotificationHistoryResponse,
  CreateNotificationChannelRequest,
  UpdateNotificationChannelRequest,
  CreateNotificationRuleRequest,
} from '@/types/notification'

type ListResp<T> = { items: T[]; total: number }

export const useNotificationStore = defineStore('notification', () => {
  const channels = ref<NotificationChannelResponse[]>([])
  const rules    = ref<NotificationRuleResponse[]>([])
  const history  = ref<NotificationHistoryResponse[]>([])
  const loading  = ref(false)

  // ── Channels ────────────────────────────────────────────────────────────────

  async function fetchChannels() {
    loading.value = true
    try {
      const res = await notificationApi.listChannels() as unknown as ApiResponse<ListResp<NotificationChannelResponse>>
      channels.value = res.data?.items ?? []
    } finally {
      loading.value = false
    }
  }

  async function createChannel(data: CreateNotificationChannelRequest) {
    const res = await notificationApi.createChannel(data) as unknown as ApiResponse<NotificationChannelResponse>
    channels.value.push(res.data)
    return res.data
  }

  async function updateChannel(id: number, data: UpdateNotificationChannelRequest) {
    const res = await notificationApi.updateChannel(id, data) as unknown as ApiResponse<NotificationChannelResponse>
    const idx = channels.value.findIndex(c => c.id === id)
    if (idx !== -1) channels.value[idx] = res.data
    return res.data
  }

  async function removeChannel(id: number) {
    await notificationApi.deleteChannel(id)
    channels.value = channels.value.filter(c => c.id !== id)
  }

  async function testChannel(id: number) {
    await notificationApi.testChannel(id)
  }

  // ── Rules ────────────────────────────────────────────────────────────────────

  async function fetchRules() {
    loading.value = true
    try {
      const res = await notificationApi.listRules() as unknown as ApiResponse<ListResp<NotificationRuleResponse>>
      rules.value = res.data?.items ?? []
    } finally {
      loading.value = false
    }
  }

  async function createRule(data: CreateNotificationRuleRequest) {
    const res = await notificationApi.createRule(data) as unknown as ApiResponse<NotificationRuleResponse>
    rules.value.push(res.data)
    return res.data
  }

  async function removeRule(id: number) {
    await notificationApi.deleteRule(id)
    rules.value = rules.value.filter(r => r.id !== id)
  }

  // ── History ──────────────────────────────────────────────────────────────────

  async function fetchHistory(params?: { channel_id?: number; status?: string; limit?: number; offset?: number }) {
    loading.value = true
    try {
      const res = await notificationApi.listHistory(params) as unknown as ApiResponse<ListResp<NotificationHistoryResponse>>
      history.value = res.data?.items ?? []
    } finally {
      loading.value = false
    }
  }

  return {
    channels, rules, history, loading,
    fetchChannels, createChannel, updateChannel, removeChannel, testChannel,
    fetchRules, createRule, removeRule,
    fetchHistory,
  }
})
