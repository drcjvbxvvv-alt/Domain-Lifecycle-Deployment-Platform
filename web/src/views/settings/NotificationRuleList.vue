<script setup lang="ts">
import { onMounted, ref, h, computed } from 'vue'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNodeChild } from 'vue'
import {
  NButton, NSpace, NModal, NForm, NFormItem, NSelect, NSwitch,
  NPopconfirm, NTag, useMessage,
} from 'naive-ui'
import { AppTable, PageHeader } from '@/components'
import { useNotificationStore } from '@/stores/notification'
import type { NotificationRuleResponse, Severity } from '@/types/notification'

const store   = useNotificationStore()
const message = useMessage()

// ── Options ───────────────────────────────────────────────────────────────────
const severityOptions: SelectOption[] = [
  { label: 'P1 (緊急)', value: 'P1' },
  { label: 'P2 (錯誤)', value: 'P2' },
  { label: 'P3 (警告)', value: 'P3' },
  { label: 'INFO (資訊)', value: 'INFO' },
]

const channelOptions = computed<SelectOption[]>(() =>
  store.channels.map(c => ({ label: c.name, value: c.id }))
)

// ── Create modal ───────────────────────────────────────────────────────────────
const showCreate  = ref(false)
const creating    = ref(false)
const formChannelId  = ref<number | null>(null)
const formMinSeverity = ref<Severity>('P3')
const formEnabled = ref(true)

function openCreate() {
  formChannelId.value   = null
  formMinSeverity.value = 'P3'
  formEnabled.value     = true
  showCreate.value      = true
}

async function submitCreate() {
  if (!formChannelId.value) {
    message.warning('請選擇通知頻道')
    return
  }
  creating.value = true
  try {
    await store.createRule({
      channel_id:   formChannelId.value,
      min_severity: formMinSeverity.value,
      enabled:      formEnabled.value,
    })
    message.success('規則已建立')
    showCreate.value = false
  } catch (e: any) {
    message.error(e?.response?.data?.message ?? '建立失敗')
  } finally {
    creating.value = false
  }
}

// ── Delete ────────────────────────────────────────────────────────────────────
async function deleteRule(id: number) {
  try {
    await store.removeRule(id)
    message.success('已刪除')
  } catch (e: any) {
    message.error(e?.response?.data?.message ?? '刪除失敗')
  }
}

// ── Helper: find channel name ─────────────────────────────────────────────────
function channelName(id: number) {
  return store.channels.find(c => c.id === id)?.name ?? `#${id}`
}

// ── Table columns ──────────────────────────────────────────────────────────────
const severityColor: Record<string, 'error' | 'warning' | 'info' | 'default'> = {
  P1: 'error', P2: 'warning', P3: 'info', INFO: 'default',
}

const columns: DataTableColumns<NotificationRuleResponse> = [
  {
    title: '頻道', key: 'channel_id', ellipsis: { tooltip: true },
    render: (row) => channelName(row.channel_id),
  },
  {
    title: '最低等級', key: 'min_severity', width: 120,
    render: (row): VNodeChild =>
      h(NTag, { type: severityColor[row.min_severity] ?? 'default', size: 'small' },
        { default: () => row.min_severity }),
  },
  {
    title: '告警類型', key: 'alert_type', width: 120,
    render: (row) => row.alert_type ?? '全部',
  },
  {
    title: '目標類型', key: 'target_type', width: 120,
    render: (row) => row.target_type ?? '全域',
  },
  {
    title: '狀態', key: 'enabled', width: 80,
    render: (row): VNodeChild =>
      h(NTag, { type: row.enabled ? 'success' : 'default', size: 'small' },
        { default: () => row.enabled ? '啟用' : '停用' }),
  },
  {
    title: '操作', key: 'actions', width: 100, fixed: 'right',
    render: (row): VNodeChild => h(NPopconfirm, {
      onPositiveClick: () => deleteRule(row.id),
    }, {
      trigger: () => h(NButton, { size: 'small', type: 'error', ghost: true }, { default: () => '刪除' }),
      default: () => '確定刪除此規則？',
    }),
  },
]

onMounted(async () => {
  await Promise.all([store.fetchChannels(), store.fetchRules()])
})
</script>

<template>
  <div>
    <PageHeader title="通知規則管理">
      <template #actions>
        <NButton type="primary" @click="openCreate">新增規則</NButton>
      </template>
    </PageHeader>

    <AppTable
      :columns="columns"
      :data="store.rules"
      :loading="store.loading"
      :row-key="(row) => row.id"
    />

    <!-- Create modal -->
    <NModal
      v-model:show="showCreate"
      preset="card"
      title="新增通知規則"
      style="width: 480px"
      :mask-closable="false"
    >
      <NForm label-placement="left" label-width="100px">
        <NFormItem label="通知頻道" required>
          <NSelect
            v-model:value="(formChannelId as any)"
            :options="channelOptions"
            placeholder="選擇頻道"
          />
        </NFormItem>
        <NFormItem label="最低嚴重性">
          <NSelect v-model:value="(formMinSeverity as string)" :options="severityOptions" />
        </NFormItem>
        <NFormItem label="啟用">
          <NSwitch v-model:value="formEnabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCreate = false">取消</NButton>
          <NButton type="primary" :loading="creating" @click="submitCreate">建立</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
