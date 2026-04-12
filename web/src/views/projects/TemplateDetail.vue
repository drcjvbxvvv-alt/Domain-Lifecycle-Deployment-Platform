<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { NTabs, NTabPane, NDescriptions, NDescriptionsItem } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AppTable, PageHeader } from '@/components'
import { useTemplateStore } from '@/stores/template'
import type { TemplateVersionResponse } from '@/types/template'

const route = useRoute()
const store = useTemplateStore()
const tid   = route.params.tid as string

const versionCols: DataTableColumns<TemplateVersionResponse> = [
  { title: '版本',      key: 'version_label', width: 120 },
  { title: '校驗碼',    key: 'checksum',       ellipsis: { tooltip: true }, width: 200 },
  { title: '已發布',    key: 'published_at',   width: 180,
    render: (row) => row.published_at
      ? new Date(row.published_at).toLocaleString('zh-TW')
      : '草稿' },
  { title: '建立時間',  key: 'created_at',     width: 180,
    render: (row) => new Date(row.created_at).toLocaleString('zh-TW') },
]

onMounted(async () => {
  await store.fetchOne(tid)
  if (store.current) await store.fetchVersions(store.current.id)
})
</script>

<template>
  <div class="detail-page">
    <PageHeader
      :title="store.current?.name ?? '載入中...'"
      subtitle="範本詳情"
    />

    <div v-if="store.current" class="detail-page__body">
      <div class="detail-page__sidebar">
        <NDescriptions bordered :column="1" label-placement="left">
          <NDescriptionsItem label="ID">{{ store.current.id }}</NDescriptionsItem>
          <NDescriptionsItem label="UUID">{{ store.current.uuid }}</NDescriptionsItem>
          <NDescriptionsItem label="說明">{{ store.current.description || '-' }}</NDescriptionsItem>
        </NDescriptions>
      </div>

      <div class="detail-page__main">
        <NTabs type="line" animated>
          <NTabPane name="versions" :tab="`版本列表 (${store.versions.length})`">
            <AppTable :columns="versionCols" :data="store.versions" :row-key="(r) => String(r.id)" />
          </NTabPane>
        </NTabs>
      </div>
    </div>
  </div>
</template>

<style scoped>
.detail-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}
.detail-page__body {
  display: flex;
  flex: 1;
  overflow: hidden;
  gap: 0;
}
.detail-page__sidebar {
  width: 320px;
  flex-shrink: 0;
  border-right: 1px solid var(--border);
  padding: var(--space-6);
  overflow-y: auto;
}
.detail-page__main {
  flex: 1;
  padding: var(--space-6);
  overflow-y: auto;
}
</style>
