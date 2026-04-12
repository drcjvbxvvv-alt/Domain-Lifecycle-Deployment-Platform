<script setup lang="ts">
import { onMounted, ref, h } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import {
  NButton, NModal, NCard, NForm, NFormItem, NInputNumber, NText, useMessage,
} from 'naive-ui'
import { AppTable, PageHeader, PageHint } from '@/components'
import { useHostGroupStore } from '@/stores/hostGroup'
import type { HostGroupResponse } from '@/types/hostGroup'

const store   = useHostGroupStore()
const message = useMessage()

const showEdit  = ref(false)
const saving    = ref(false)
const editTarget = ref<HostGroupResponse | null>(null)
const form = ref({ max_concurrency: 0, reload_batch_size: 50, reload_batch_wait_secs: 30 })

const columns: DataTableColumns<HostGroupResponse> = [
  { title: '名稱',    key: 'name', ellipsis: { tooltip: true } },
  { title: '說明',    key: 'description', render: (row) => row.description ?? '-' },
  { title: 'Region',  key: 'region',      width: 100, render: (row) => row.region ?? '-' },
  { title: '最大並發',  key: 'max_concurrency', width: 100,
    render: (row) => h(NText, { type: row.max_concurrency === 0 ? 'default' : 'warning' },
      { default: () => row.max_concurrency === 0 ? '不限' : String(row.max_concurrency) }) },
  { title: 'Reload 批次', key: 'reload_batch_size', width: 110,
    render: (row) => `${row.reload_batch_size} 域名` },
  { title: 'Reload 等待', key: 'reload_batch_wait_secs', width: 110,
    render: (row) => `${row.reload_batch_wait_secs} 秒` },
  { title: '操作', key: 'actions', width: 80, fixed: 'right',
    render: (row) => h(NButton, {
      size: 'small', quaternary: true, type: 'primary',
      onClick: () => openEdit(row),
    }, { default: () => '設定' }) },
]

function openEdit(hg: HostGroupResponse) {
  editTarget.value = hg
  form.value = {
    max_concurrency:       hg.max_concurrency,
    reload_batch_size:     hg.reload_batch_size,
    reload_batch_wait_secs: hg.reload_batch_wait_secs,
  }
  showEdit.value = true
}

async function handleSave() {
  if (!editTarget.value) return
  saving.value = true
  try {
    await store.updateConcurrency(editTarget.value.id, form.value)
    message.success('設定已更新')
    showEdit.value = false
  } catch (e: any) {
    message.error(e?.response?.data?.message || '更新失敗')
  } finally {
    saving.value = false
  }
}

onMounted(() => store.fetchList())
</script>

<template>
  <div class="list-page">
    <PageHeader title="Host Group 管理" subtitle="並發與 Reload 批次設定">
      <template #hint>
        <PageHint storage-key="host-group-list" title="Host Group 並發說明">
          <strong>最大並發</strong>：同一 Host Group 內同時執行的 Agent 任務上限（0 = 不限）。<br>
          <strong>Reload 批次</strong>：同一 Agent 在單次 Shard 派發中，每隔幾個域名執行一次 nginx reload。<br>
          <strong>Reload 等待</strong>：批次間等待秒數（Critical Rule #7：多域名同主機 → 單次 reload）。
        </PageHint>
      </template>
    </PageHeader>

    <AppTable
      :columns="columns"
      :data="store.hostGroups"
      :loading="store.loading"
      :row-key="(r) => String(r.id)"
    />

    <NModal v-model:show="showEdit" :mask-closable="!saving">
      <NCard
        :title="`設定：${editTarget?.name ?? ''}`"
        :bordered="false"
        style="width: 440px"
      >
        <NForm label-placement="left" label-width="100">
          <NFormItem label="最大並發">
            <NInputNumber
              v-model:value="form.max_concurrency"
              :min="0"
              style="width: 100%"
              placeholder="0 = 不限"
            />
          </NFormItem>
          <NFormItem label="Reload 批次">
            <NInputNumber
              v-model:value="form.reload_batch_size"
              :min="1"
              style="width: 100%"
              placeholder="預設 50"
            />
          </NFormItem>
          <NFormItem label="等待秒數">
            <NInputNumber
              v-model:value="form.reload_batch_wait_secs"
              :min="1"
              style="width: 100%"
              placeholder="預設 30"
            />
          </NFormItem>
        </NForm>
        <template #action>
          <div style="display: flex; justify-content: flex-end; gap: 8px">
            <NButton @click="showEdit = false" :disabled="saving">取消</NButton>
            <NButton type="primary" :loading="saving" @click="handleSave">儲存</NButton>
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
</style>
