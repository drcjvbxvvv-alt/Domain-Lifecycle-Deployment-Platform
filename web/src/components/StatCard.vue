<script setup lang="ts">
withDefaults(defineProps<{
  label:   string
  value:   string | number
  // Accent color — use colors.xxx from tokens (e.g. colors.statusSemantic.success.color)
  color?:  string
  // Optional trend: positive = green, negative = red
  trend?:  number
  suffix?: string
}>(), {
  color: '#4f7ef8',
})
</script>

<!--
  Usage:
    <StatCard label="活躍 Releases"  :value="stats.executing" color="#b45309" />
    <StatCard label="在線 Agent"     :value="24"              color="#15803d" :trend="3" />
    <StatCard label="失敗 / 待處理"  :value="stats.failed"    color="#b91c1c" suffix="個" />
-->
<template>
  <div class="stat-card">
    <!-- Left accent bar — 3px wide, full height, color = props.color -->
    <div class="stat-card__accent" :style="{ backgroundColor: color }" />

    <div class="stat-card__body">
      <p class="stat-card__label">{{ label }}</p>
      <div class="stat-card__value-row">
        <span class="stat-card__value">{{ value }}</span>
        <span v-if="suffix" class="stat-card__suffix">{{ suffix }}</span>
        <span
          v-if="trend !== undefined"
          class="stat-card__trend"
          :class="trend >= 0 ? 'trend-up' : 'trend-down'"
        >
          {{ trend >= 0 ? '▲' : '▼' }} {{ Math.abs(trend) }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.stat-card {
  display: flex;
  flex-direction: row;
  align-items: stretch;
  background-color: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: var(--shadow-card);
  overflow: hidden;
  position: relative;
  transition: box-shadow 0.15s;
}
.stat-card:hover {
  box-shadow: var(--shadow-elevated);
}

.stat-card__accent {
  flex-shrink: 0;
  width: 3px;
  border-radius: 10px 0 0 10px;
  align-self: stretch;
}

.stat-card__body {
  display: flex;
  flex-direction: column;
  padding: 20px 24px;
  gap: 4px;
  flex: 1;
}

.stat-card__label {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.4px;
  line-height: 1;
}

.stat-card__value-row {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  margin-top: 4px;
}

.stat-card__value {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.2;
}

.stat-card__suffix {
  font-size: 13px;
  color: var(--text-secondary);
}

.stat-card__trend {
  font-size: 12px;
  font-weight: 500;
}
.trend-up   { color: #15803d; }
.trend-down { color: #b91c1c; }
</style>
