<script setup lang="ts">
import { onMounted, h } from 'vue'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNodeChild } from 'vue'
import { NTag, NSpace, NSelect, NButton } from 'naive-ui'
import { AppTable, PageHeader } from '@/components'
import { useNotificationStore } from '@/stores/notification'
import type { NotificationHistoryResponse, HistoryStatus } from '@/types/notification'
import { ref } from 'vue'

const store = useNotificationStore()

const filterStatus  = ref<HistoryStatus | ''>('')
const filterChannel = ref<number>(0)

const statusOptions: SelectOption[] = [
  { label: '全部', value: '' },
  { label: '成功送出', value: 'sent' },
  { label: '失敗', value: 'failed' },
  { label: '已抑制', value: 'suppressed' },
]

const channelOptions = (): SelectOption[] =>
  [{ label: '全部頻道', value: 0 }, ...store.channels.map(c => ({ label: c.name, value: c.id }))]

async function refresh() {
  await store.fetchHistory({
    status:     filterStatus.value || undefined,
    channel_id: filterChannel.value || undefined,
    limit:      100,
  })
}

const statusColor: Record<HistoryStatus, 'success' | 'error' | 'default'> = {
  sent: 'success', failed: 'error', suppressed: 'default',
}

function channelName(id: number) {
  return store.channels.find(c => c.id === id)?.name ?? `#${id}`
}

const columns: DataTableColumns<NotificationHistoryResponse> = [
  {
    title: '頻道', key: 'channel_id', width: 160, ellipsis: { tooltip: true },
    render: (row) => channelName(row.channel_id),
  },
  {
    title: '狀態', key: 'status', width: 100,
    render: (row): VNodeChild =>
      h(NTag, { type: statusColor[row.status] ?? 'default', size: 'small' },
        { default: () => row.status }),
  },
  {
    title: '訊息', key: 'message', ellipsis: { tooltip: true },
    render: (row) => row.message ?? '-',
  },
  {
    title: '錯誤', key: 'error', ellipsis: { tooltip: true },
    render: (row) => row.error ?? '-',
  },
  {
    title: '告警 ID', key: 'alert_event_id', width: 100,
    render: (row) => row.alert_event_id?.toString() ?? '-',
  },
  {
    title: '送出時間', key: 'sent_at', width: 180,
    render: (row) => new Date(row.sent_at).toLocaleString('zh-TW'),
  },
]

onMounted(async () => {
  await store.fetchChannels()
  await refresh()
})
</script>

<template>
  <div>
    <PageHeader title="通知歷史記錄" />

    <!-- Filter bar -->
    <NSpace class="mb-3" align="center">
      <NSelect
        v-model:value="(filterChannel as any)"
        :options="channelOptions()"
        style="width: 180px"
        placeholder="選擇頻道"
      />
      <NSelect
        v-model:value="(filterStatus as string)"
        :options="statusOptions"
        style="width: 140px"
      />
      <NButton @click="refresh">查詢</NButton>
    </NSpace>

    <AppTable
      :columns="columns"
      :data="store.history"
      :loading="store.loading"
      :row-key="(row) => row.id"
    />
  </div>
</template>

<style scoped>
.mb-3 { margin-bottom: 12px; }
</style>
