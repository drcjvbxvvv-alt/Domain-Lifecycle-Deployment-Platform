<script setup lang="ts">
import { onMounted, ref, h, computed } from 'vue'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNodeChild } from 'vue'
import {
  NButton, NSpace, NModal, NForm, NFormItem, NInput, NSelect, NSwitch,
  NPopconfirm, NTag, NBadge, useMessage,
} from 'naive-ui'
import { AppTable, PageHeader } from '@/components'
import { useNotificationStore } from '@/stores/notification'
import type {
  NotificationChannelResponse,
  ChannelType,
  TelegramConfig,
  SlackConfig,
  WebhookConfig,
  EmailConfig,
} from '@/types/notification'

const store   = useNotificationStore()
const message = useMessage()

// ── Channel type options ───────────────────────────────────────────────────────
const typeOptions: SelectOption[] = [
  { label: 'Telegram',    value: 'telegram' },
  { label: 'Slack',       value: 'slack' },
  { label: 'Webhook',     value: 'webhook' },
  { label: 'Email (SMTP)', value: 'email' },
]

// ── Create / Edit modal ────────────────────────────────────────────────────────
const showForm   = ref(false)
const saving     = ref(false)
const editingId  = ref<number | null>(null)

const formName        = ref('')
const formType        = ref<ChannelType>('telegram')
const formEnabled     = ref(true)
const formIsDefault   = ref(false)

// Per-type config fields
const telegramForm = ref<TelegramConfig>({ bot_token: '', chat_id: '' })
const slackForm    = ref<SlackConfig>({ webhook_url: '', username: '', channel: '' })
const webhookForm  = ref<WebhookConfig>({ url: '' })
const emailForm    = ref<EmailConfig>({
  smtp_host: '', smtp_port: 587, username: '', password: '',
  from_address: '', to_addresses: [],
  use_tls: false, use_starttls: true,
})
const emailToStr = ref('')

function openCreate() {
  editingId.value = null
  formName.value = ''
  formType.value = 'telegram'
  formEnabled.value = true
  formIsDefault.value = false
  telegramForm.value = { bot_token: '', chat_id: '' }
  slackForm.value    = { webhook_url: '', username: '', channel: '' }
  webhookForm.value  = { url: '' }
  emailForm.value    = { smtp_host: '', smtp_port: 587, username: '', password: '', from_address: '', to_addresses: [], use_tls: false, use_starttls: true }
  emailToStr.value = ''
  showForm.value = true
}

function openEdit(ch: NotificationChannelResponse) {
  editingId.value   = ch.id
  formName.value    = ch.name
  formType.value    = ch.channel_type
  formEnabled.value = ch.enabled
  formIsDefault.value = ch.is_default
  // Config is redacted in list — only allow re-entering
  showForm.value = true
}

function buildConfig(): Record<string, unknown> {
  switch (formType.value) {
    case 'telegram': return { ...telegramForm.value }
    case 'slack':    return { ...slackForm.value }
    case 'webhook':  return { ...webhookForm.value }
    case 'email': {
      const to = emailToStr.value.split(/[\n,;]+/).map(s => s.trim()).filter(Boolean)
      return { ...emailForm.value, to_addresses: to }
    }
  }
}

async function submitForm() {
  if (!formName.value.trim()) {
    message.warning('名稱必填')
    return
  }
  saving.value = true
  try {
    const data = {
      name:         formName.value,
      channel_type: formType.value,
      config:       buildConfig(),
      is_default:   formIsDefault.value,
      enabled:      formEnabled.value,
    }
    if (editingId.value !== null) {
      await store.updateChannel(editingId.value, data)
      message.success('已更新')
    } else {
      await store.createChannel(data)
      message.success('通知頻道已建立')
    }
    showForm.value = false
  } catch (e: any) {
    message.error(e?.response?.data?.message ?? '操作失敗')
  } finally {
    saving.value = false
  }
}

// ── Test ──────────────────────────────────────────────────────────────────────
const testing = ref<number | null>(null)

async function testChannel(id: number) {
  testing.value = id
  try {
    await store.testChannel(id)
    message.success('測試訊息已送出')
  } catch (e: any) {
    message.error(e?.response?.data?.message ?? '測試失敗')
  } finally {
    testing.value = null
  }
}

// ── Delete ────────────────────────────────────────────────────────────────────
async function deleteChannel(id: number) {
  try {
    await store.removeChannel(id)
    message.success('已刪除')
  } catch (e: any) {
    message.error(e?.response?.data?.message ?? '刪除失敗')
  }
}

// ── Table columns ─────────────────────────────────────────────────────────────
const typeLabel: Record<ChannelType, string> = {
  telegram: 'Telegram', slack: 'Slack', webhook: 'Webhook', email: 'Email',
}

const columns: DataTableColumns<NotificationChannelResponse> = [
  { title: '名稱', key: 'name', ellipsis: { tooltip: true } },
  {
    title: '類型', key: 'channel_type', width: 120,
    render: (row) => row.channel_type ? typeLabel[row.channel_type] : '-',
  },
  {
    title: '狀態', key: 'enabled', width: 80,
    render: (row): VNodeChild =>
      h(NTag, { type: row.enabled ? 'success' : 'default', size: 'small' },
        { default: () => row.enabled ? '啟用' : '停用' }),
  },
  {
    title: '預設', key: 'is_default', width: 80,
    render: (row) => row.is_default ? h(NBadge, { dot: true, type: 'success' }) : '-',
  },
  {
    title: '建立時間', key: 'created_at', width: 180,
    render: (row) => new Date(row.created_at).toLocaleString('zh-TW'),
  },
  {
    title: '操作', key: 'actions', width: 240, fixed: 'right',
    render: (row): VNodeChild => h(NSpace, { size: 'small' }, {
      default: () => [
        h(NButton, {
          size: 'small', ghost: true,
          loading: testing.value === row.id,
          onClick: () => testChannel(row.id),
        }, { default: () => '測試' }),
        h(NButton, {
          size: 'small', type: 'primary', ghost: true,
          onClick: () => openEdit(row),
        }, { default: () => '編輯' }),
        h(NPopconfirm, { onPositiveClick: () => deleteChannel(row.id) }, {
          trigger: () => h(NButton, { size: 'small', type: 'error', ghost: true }, { default: () => '刪除' }),
          default: () => '確定刪除此頻道？相關規則將一併刪除。',
        }),
      ],
    }),
  },
]

const modalTitle = computed(() => editingId.value !== null ? '編輯通知頻道' : '新增通知頻道')

onMounted(() => store.fetchChannels())
</script>

<template>
  <div>
    <PageHeader title="通知頻道管理">
      <template #actions>
        <NButton type="primary" @click="openCreate">新增頻道</NButton>
      </template>
    </PageHeader>

    <AppTable
      :columns="columns"
      :data="store.channels"
      :loading="store.loading"
      :row-key="(row) => row.id"
    />

    <!-- Create / Edit modal -->
    <NModal
      v-model:show="showForm"
      preset="card"
      :title="modalTitle"
      style="width: 520px"
      :mask-closable="false"
    >
      <NForm label-placement="left" label-width="110px">
        <NFormItem label="名稱" required>
          <NInput v-model:value="formName" placeholder="e.g. Ops Telegram" />
        </NFormItem>
        <NFormItem label="類型" required>
          <NSelect v-model:value="(formType as string)" :options="typeOptions" :disabled="editingId !== null" />
        </NFormItem>
        <NFormItem label="啟用">
          <NSwitch v-model:value="formEnabled" />
        </NFormItem>
        <NFormItem label="設為預設">
          <NSwitch v-model:value="formIsDefault" />
        </NFormItem>

        <!-- Telegram config -->
        <template v-if="formType === 'telegram'">
          <NFormItem label="Bot Token" required>
            <NInput v-model:value="telegramForm.bot_token" placeholder="123456:ABC..." />
          </NFormItem>
          <NFormItem label="Chat ID" required>
            <NInput v-model:value="telegramForm.chat_id" placeholder="-100123456789" />
          </NFormItem>
        </template>

        <!-- Slack config -->
        <template v-else-if="formType === 'slack'">
          <NFormItem label="Webhook URL" required>
            <NInput v-model:value="slackForm.webhook_url" placeholder="https://hooks.slack.com/services/..." />
          </NFormItem>
          <NFormItem label="Username">
            <NInput v-model:value="(slackForm.username as string)" placeholder="DomainBot" clearable />
          </NFormItem>
          <NFormItem label="Channel">
            <NInput v-model:value="(slackForm.channel as string)" placeholder="#alerts" clearable />
          </NFormItem>
        </template>

        <!-- Webhook config -->
        <template v-else-if="formType === 'webhook'">
          <NFormItem label="Endpoint URL" required>
            <NInput v-model:value="webhookForm.url" placeholder="https://hooks.example.com/..." />
          </NFormItem>
        </template>

        <!-- Email config -->
        <template v-else-if="formType === 'email'">
          <NFormItem label="SMTP Host" required>
            <NInput v-model:value="emailForm.smtp_host" placeholder="smtp.example.com" />
          </NFormItem>
          <NFormItem label="SMTP Port">
            <NInput v-model:value="(emailForm.smtp_port as unknown as string)" placeholder="587" />
          </NFormItem>
          <NFormItem label="Username">
            <NInput v-model:value="(emailForm.username as string)" clearable />
          </NFormItem>
          <NFormItem label="Password">
            <NInput v-model:value="(emailForm.password as string)" type="password" show-password-on="click" clearable />
          </NFormItem>
          <NFormItem label="From Address" required>
            <NInput v-model:value="emailForm.from_address" placeholder="alerts@example.com" />
          </NFormItem>
          <NFormItem label="To Addresses" required>
            <NInput
              v-model:value="emailToStr"
              type="textarea"
              :rows="3"
              placeholder="one@example.com, two@example.com"
            />
          </NFormItem>
          <NFormItem label="STARTTLS">
            <NSwitch v-model:value="emailForm.use_starttls" />
          </NFormItem>
          <NFormItem label="Implicit TLS">
            <NSwitch v-model:value="emailForm.use_tls" />
          </NFormItem>
        </template>
      </NForm>

      <template #footer>
        <NSpace justify="end">
          <NButton @click="showForm = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="submitForm">
            {{ editingId !== null ? '儲存' : '建立' }}
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
