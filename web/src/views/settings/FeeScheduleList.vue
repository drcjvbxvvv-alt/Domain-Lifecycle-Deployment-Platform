<script setup lang="ts">
import { onMounted, ref, computed, h } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import type { VNodeChild } from 'vue'
import {
  NButton, NModal, NCard, NForm, NFormItem, NInput, NInputNumber,
  NSelect, NSpace, NPopconfirm, useMessage,
} from 'naive-ui'
import { AppTable, PageHeader, PageHint } from '@/components'
import { useCostStore } from '@/stores/cost'
import { useRegistrarStore } from '@/stores/registrar'
import type { FeeScheduleResponse, CreateFeeScheduleRequest, UpdateFeeScheduleRequest } from '@/types/cost'

const store    = useCostStore()
const regStore = useRegistrarStore()
const message  = useMessage()

// Filters
const filterRegistrarId = ref<number | null>(null)

const registrarOptions = computed(() =>
  regStore.registrars.map(r => ({ label: r.name, value: r.id }))
)

// Registrar name lookup for table display
const registrarName = computed(() => {
  const m: Record<number, string> = {}
  regStore.registrars.forEach(r => { m[r.id] = r.name })
  return m
})

const currencyOptions = [
  { label: 'USD', value: 'USD' }, { label: 'EUR', value: 'EUR' },
  { label: 'GBP', value: 'GBP' }, { label: 'TWD', value: 'TWD' },
  { label: 'CNY', value: 'CNY' }, { label: 'JPY', value: 'JPY' },
  { label: 'AUD', value: 'AUD' }, { label: 'CAD', value: 'CAD' },
]

function loadSchedules() {
  store.fetchFeeSchedules(filterRegistrarId.value ?? undefined)
}

// ── Create modal ──────────────────────────────────────────────────────────────
const showCreate = ref(false)
const creating   = ref(false)
const createForm = ref<CreateFeeScheduleRequest>({
  registrar_id:      0,
  tld:               '',
  registration_fee:  0,
  renewal_fee:       0,
  transfer_fee:      0,
  privacy_fee:       0,
  currency:          'USD',
})

function openCreate() {
  createForm.value = {
    registrar_id: 0, tld: '', registration_fee: 0,
    renewal_fee: 0, transfer_fee: 0, privacy_fee: 0, currency: 'USD',
  }
  showCreate.value = true
}

async function handleCreate() {
  if (!createForm.value.registrar_id || !createForm.value.tld) {
    message.warning('請選擇 Registrar 並填寫 TLD')
    return
  }
  creating.value = true
  try {
    await store.createFeeSchedule(createForm.value)
    message.success('費率表已建立')
    showCreate.value = false
  } catch (e: any) {
    message.error(e?.response?.data?.message || '建立失敗')
  } finally {
    creating.value = false
  }
}

// ── Edit modal ────────────────────────────────────────────────────────────────
const showEdit    = ref(false)
const editing     = ref(false)
const editId      = ref(0)
const editForm    = ref<UpdateFeeScheduleRequest>({
  registration_fee: 0, renewal_fee: 0, transfer_fee: 0, privacy_fee: 0, currency: 'USD',
})

function openEdit(row: FeeScheduleResponse) {
  editId.value = row.id
  editForm.value = {
    registration_fee: row.registration_fee,
    renewal_fee:      row.renewal_fee,
    transfer_fee:     row.transfer_fee,
    privacy_fee:      row.privacy_fee,
    currency:         row.currency,
  }
  showEdit.value = true
}

async function handleEdit() {
  editing.value = true
  try {
    await store.updateFeeSchedule(editId.value, editForm.value)
    message.success('費率表已更新')
    showEdit.value = false
  } catch (e: any) {
    message.error(e?.response?.data?.message || '更新失敗')
  } finally {
    editing.value = false
  }
}

async function handleDelete(id: number) {
  try {
    await store.deleteFeeSchedule(id)
    message.success('已刪除')
  } catch (e: any) {
    message.error(e?.response?.data?.message || '刪除失敗')
  }
}

// ── Table columns ─────────────────────────────────────────────────────────────
const columns: DataTableColumns<FeeScheduleResponse> = [
  { title: 'Registrar', key: 'registrar_id', width: 160,
    render: (row): VNodeChild => registrarName.value[row.registrar_id] ?? `#${row.registrar_id}` },
  { title: 'TLD', key: 'tld', width: 100 },
  { title: '幣別', key: 'currency', width: 70 },
  { title: '註冊費', key: 'registration_fee', width: 90,
    render: (row): VNodeChild => row.registration_fee.toFixed(2) },
  { title: '續約費', key: 'renewal_fee', width: 90,
    render: (row): VNodeChild => row.renewal_fee.toFixed(2) },
  { title: '轉移費', key: 'transfer_fee', width: 90,
    render: (row): VNodeChild => row.transfer_fee.toFixed(2) },
  { title: 'WHOIS 隱私費', key: 'privacy_fee', width: 110,
    render: (row): VNodeChild => row.privacy_fee.toFixed(2) },
  {
    title: '操作', key: 'actions', width: 120, fixed: 'right',
    render: (row): VNodeChild => h(NSpace, {}, {
      default: () => [
        h(NButton, { size: 'small', quaternary: true, type: 'primary', onClick: () => openEdit(row) }, { default: () => '編輯' }),
        h(NPopconfirm, { onPositiveClick: () => handleDelete(row.id) }, {
          trigger: () => h(NButton, { size: 'small', quaternary: true, type: 'error' }, { default: () => '刪除' }),
          default: () => `確認刪除 ${row.tld} 費率？`,
        }),
      ],
    }),
  },
]

onMounted(async () => {
  await regStore.fetchList()
  loadSchedules()
})
</script>

<template>
  <div class="list-page">
    <PageHeader title="費率表管理" subtitle="設定各 Registrar × TLD 的費用標準">
      <template #actions>
        <NButton type="primary" @click="openCreate">新增費率</NButton>
      </template>
      <template #hint>
        <PageHint storage-key="fee-schedule" title="費率表說明">
          費率表定義各 Registrar × TLD 組合的標準費用。<br>
          為域名設定 Registrar 帳號後，若 <strong>費用固定</strong> 未勾選，系統將自動套用對應費率。
        </PageHint>
      </template>
    </PageHeader>

    <!-- Filter -->
    <div class="filter-bar">
      <NSelect
        v-model:value="filterRegistrarId"
        :options="registrarOptions"
        placeholder="篩選 Registrar"
        clearable
        style="width: 200px"
        @update:value="loadSchedules"
      />
    </div>

    <AppTable
      :columns="columns"
      :data="store.feeSchedules"
      :loading="store.loading"
      :row-key="(r: FeeScheduleResponse) => r.id"
      scroll-x="820"
    />

    <!-- Create modal -->
    <NModal v-model:show="showCreate" :mask-closable="!creating">
      <NCard title="新增費率" :bordered="false" style="width: 520px">
        <NForm label-placement="left" label-width="100px">
          <NFormItem label="Registrar" required>
            <NSelect
              v-model:value="(createForm as any).registrar_id"
              :options="registrarOptions"
              placeholder="選擇 Registrar"
            />
          </NFormItem>
          <NFormItem label="TLD" required>
            <NInput v-model:value="createForm.tld" placeholder="例：.com 或 .co.uk" />
          </NFormItem>
          <NFormItem label="幣別" required>
            <NSelect v-model:value="(createForm as any).currency" :options="currencyOptions" />
          </NFormItem>
          <NFormItem label="註冊費">
            <NInputNumber
              v-model:value="(createForm as any).registration_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
          <NFormItem label="續約費">
            <NInputNumber
              v-model:value="(createForm as any).renewal_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
          <NFormItem label="轉移費">
            <NInputNumber
              v-model:value="(createForm as any).transfer_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
          <NFormItem label="隱私費">
            <NInputNumber
              v-model:value="(createForm as any).privacy_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
        </NForm>
        <template #action>
          <div style="display:flex; justify-content:flex-end; gap:8px">
            <NButton :disabled="creating" @click="showCreate = false">取消</NButton>
            <NButton type="primary" :loading="creating" @click="handleCreate">建立</NButton>
          </div>
        </template>
      </NCard>
    </NModal>

    <!-- Edit modal -->
    <NModal v-model:show="showEdit" :mask-closable="!editing">
      <NCard title="編輯費率" :bordered="false" style="width: 480px">
        <NForm label-placement="left" label-width="100px">
          <NFormItem label="幣別" required>
            <NSelect v-model:value="(editForm as any).currency" :options="currencyOptions" />
          </NFormItem>
          <NFormItem label="註冊費">
            <NInputNumber
              v-model:value="(editForm as any).registration_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
          <NFormItem label="續約費">
            <NInputNumber
              v-model:value="(editForm as any).renewal_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
          <NFormItem label="轉移費">
            <NInputNumber
              v-model:value="(editForm as any).transfer_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
          <NFormItem label="隱私費">
            <NInputNumber
              v-model:value="(editForm as any).privacy_fee"
              :min="0" :precision="2" style="width:100%"
            />
          </NFormItem>
        </NForm>
        <template #action>
          <div style="display:flex; justify-content:flex-end; gap:8px">
            <NButton :disabled="editing" @click="showEdit = false">取消</NButton>
            <NButton type="primary" :loading="editing" @click="handleEdit">儲存</NButton>
          </div>
        </template>
      </NCard>
    </NModal>
  </div>
</template>

<style scoped>
.list-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}
.filter-bar {
  display: flex;
  gap: 8px;
  padding: 8px var(--content-padding);
  border-bottom: 1px solid var(--border);
}
</style>
