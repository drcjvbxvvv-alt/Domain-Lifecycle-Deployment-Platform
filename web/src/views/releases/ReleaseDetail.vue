<script setup lang="ts">
import { onMounted, h } from 'vue'
import { useRoute } from 'vue-router'
import {
  NTabs, NTabPane, NDescriptions, NDescriptionsItem,
  NTimeline, NTimelineItem,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AppTable, PageHeader, StatusTag } from '@/components'
import { useReleaseStore } from '@/stores/release'
import type { ReleaseShardResponse } from '@/types/release'

const route = useRoute()
const store = useReleaseStore()
const rid   = route.params.rid as string

const shardCols: DataTableColumns<ReleaseShardResponse> = [
  { title: '#', key: 'shard_index', width: 60 },
  { title: 'Canary', key: 'is_canary', width: 80,
    render: (row) => row.is_canary ? 'canary' : '-' },
  { title: '狀態', key: 'status', width: 140,
    render: (row) => h(StatusTag, { status: row.status }) },
  { title: '域名數', key: 'domain_count', width: 80 },
  { title: '成功', key: 'success_count', width: 70 },
  { title: '失敗', key: 'failure_count', width: 70 },
  { title: '開始時間', key: 'started_at', width: 180,
    render: (row) => row.started_at ? new Date(row.started_at).toLocaleString('zh-TW') : '-' },
  { title: '結束時間', key: 'ended_at', width: 180,
    render: (row) => row.ended_at ? new Date(row.ended_at).toLocaleString('zh-TW') : '-' },
]

onMounted(async () => {
  await store.fetchOne(rid)
  await Promise.all([
    store.fetchShards(rid),
    store.fetchHistory(rid),
  ])
})
</script>

<template>
  <div class="detail-page">
    <PageHeader
      :title="store.current?.release_id ?? '載入中...'"
      subtitle="發布詳情"
    />

    <div v-if="store.current" class="detail-page__body">
      <div class="detail-page__sidebar">
        <NDescriptions bordered :column="1" label-placement="left">
          <NDescriptionsItem label="UUID">{{ store.current.uuid }}</NDescriptionsItem>
          <NDescriptionsItem label="狀態">
            <StatusTag :status="store.current.status" />
          </NDescriptionsItem>
          <NDescriptionsItem label="類型">{{ store.current.release_type }}</NDescriptionsItem>
          <NDescriptionsItem label="觸發來源">{{ store.current.trigger_source }}</NDescriptionsItem>
          <NDescriptionsItem label="域名數">{{ store.current.total_domains ?? '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="Shard 數">{{ store.current.total_shards ?? '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="成功 / 失敗">
            {{ store.current.success_count }} / {{ store.current.failure_count }}
          </NDescriptionsItem>
          <NDescriptionsItem label="建立時間">
            {{ new Date(store.current.created_at).toLocaleString('zh-TW') }}
          </NDescriptionsItem>
          <NDescriptionsItem label="說明">
            {{ store.current.description || '-' }}
          </NDescriptionsItem>
        </NDescriptions>
      </div>

      <div class="detail-page__main">
        <NTabs type="line" animated>
          <NTabPane name="shards" :tab="`Shards (${store.shards.length})`">
            <AppTable :columns="shardCols" :data="store.shards"
              :row-key="(r) => String(r.id)" />
          </NTabPane>

          <NTabPane name="history" :tab="`狀態歷史 (${store.history.length})`">
            <NTimeline class="history-timeline">
              <NTimelineItem
                v-for="entry in store.history"
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
