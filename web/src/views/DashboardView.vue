<script setup lang="ts">
import { onMounted } from 'vue'
import { PageHeader, StatCard } from '@/components'
import { useProjectStore } from '@/stores/project'
import { useDomainStore } from '@/stores/domain'
import { useAgentStore } from '@/stores/agent'
import { useReleaseStore } from '@/stores/release'

const projectStore = useProjectStore()
const domainStore  = useDomainStore()
const agentStore   = useAgentStore()
const releaseStore = useReleaseStore()

onMounted(async () => {
  await Promise.all([
    projectStore.fetchList(),
    domainStore.fetchList(),
    agentStore.fetchList({ limit: 1 }),
  ])
})
</script>

<template>
  <div class="dashboard">
    <PageHeader title="Dashboard" subtitle="平台概覽" />

    <div class="dashboard__stats">
      <StatCard label="專案數"  :value="projectStore.projects.length" color="#38bdf8" />
      <StatCard label="域名數"  :value="domainStore.total"            color="#4ade80" />
      <StatCard label="Agent 數" :value="agentStore.total"            color="#38bdf8" />
      <StatCard label="發布數"  :value="releaseStore.total"           color="#fbbf24" />
    </div>
  </div>
</template>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow-y: auto;
}
.dashboard__stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
  padding: var(--space-6) var(--content-padding);
}
</style>
