<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { NTabs, NTabPane, NDescriptions, NDescriptionsItem, NTimeline, NTimelineItem } from 'naive-ui'
import { PageHeader, StatusTag } from '@/components'
import { useDomainStore } from '@/stores/domain'
import { domainApi } from '@/api/domain'
import type { DomainLifecycleHistoryEntry } from '@/types/domain'
import type { ApiResponse } from '@/types/common'

const route   = useRoute()
const store   = useDomainStore()
const id      = route.params.id as string
const history = ref<DomainLifecycleHistoryEntry[]>([])

onMounted(async () => {
  await store.fetchOne(id)
  const res = await domainApi.history(id) as unknown as ApiResponse<DomainLifecycleHistoryEntry[]>
  history.value = res.data ?? []
})
</script>

<template>
  <div class="detail-page">
    <PageHeader
      :title="store.current?.fqdn ?? '載入中...'"
      subtitle="域名詳情"
    />

    <div v-if="store.current" class="detail-page__body">
      <div class="detail-page__sidebar">
        <NDescriptions bordered :column="1" label-placement="left">
          <NDescriptionsItem label="UUID">{{ store.current.uuid }}</NDescriptionsItem>
          <NDescriptionsItem label="狀態">
            <StatusTag :status="store.current.lifecycle_state" />
          </NDescriptionsItem>
          <NDescriptionsItem label="專案 ID">{{ store.current.project_id }}</NDescriptionsItem>
          <NDescriptionsItem label="DNS Provider">{{ store.current.dns_provider || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="DNS Zone">{{ store.current.dns_zone || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="建立時間">
            {{ new Date(store.current.created_at).toLocaleString('zh-TW') }}
          </NDescriptionsItem>
        </NDescriptions>
      </div>

      <div class="detail-page__main">
        <NTabs type="line" animated>
          <NTabPane name="history" :tab="`狀態歷史 (${history.length})`">
            <NTimeline class="history-timeline">
              <NTimelineItem
                v-for="entry in history"
                :key="entry.id"
                :title="`${entry.from_state ?? '—'} → ${entry.to_state}`"
                :time="new Date(entry.created_at).toLocaleString('zh-TW')"
                :content="entry.reason || undefined"
              />
            </NTimeline>
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
.history-timeline {
  padding-left: var(--space-4);
}
</style>
