<template>
  <div class="card p-4">
    <div class="mb-4 flex items-center justify-between gap-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.dashboard.accountDistribution') }}
      </h3>
      <div
        class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-gray-700 dark:bg-dark-800"
      >
        <button
          type="button"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
          :class="metric === 'tokens'
            ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
            : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="emit('update:metric', 'tokens')"
        >
          {{ t('admin.dashboard.metricTokens') }}
        </button>
        <button
          type="button"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
          :class="metric === 'actual_cost'
            ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
            : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'"
          @click="emit('update:metric', 'actual_cost')"
        >
          {{ t('admin.dashboard.metricActualCost') }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>

    <div v-else-if="sortedAccounts.length > 0" class="overflow-x-auto">
      <table class="w-full text-xs">
        <thead>
          <tr class="text-gray-500 dark:text-gray-400">
            <th class="pb-2 text-left">{{ t('admin.usage.upstreamAccount') }}</th>
            <th class="pb-2 text-right">{{ t('admin.dashboard.requests') }}</th>
            <th class="pb-2 text-right">{{ t('admin.dashboard.tokens') }}</th>
            <th class="pb-2 text-right">{{ t('admin.dashboard.actual') }}</th>
            <th class="pb-2 text-right">{{ t('admin.dashboard.standard') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in sortedAccounts" :key="item.account_id" class="border-t border-gray-100 dark:border-gray-700">
            <td class="max-w-[180px] truncate py-1.5 font-medium text-gray-900 dark:text-white" :title="item.account_name || '-'">
              {{ item.account_name || '-' }}
            </td>
            <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
              {{ formatNumber(item.requests) }}
            </td>
            <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">
              {{ formatTokens(item.total_tokens) }}
            </td>
            <td class="py-1.5 text-right text-green-600 dark:text-green-400">
              ${{ formatCost(item.actual_cost) }}
            </td>
            <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">
              ${{ formatCost(item.cost) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-else class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { AccountStat } from '@/types'

type DistributionMetric = 'tokens' | 'actual_cost'

const props = withDefaults(defineProps<{
  accounts: AccountStat[]
  loading?: boolean
  metric?: DistributionMetric
}>(), {
  loading: false,
  metric: 'tokens',
})

const emit = defineEmits<{
  'update:metric': [value: DistributionMetric]
}>()

const { t } = useI18n()

const sortedAccounts = computed(() => {
  if (!props.accounts?.length) return []
  const metricKey = props.metric === 'actual_cost' ? 'actual_cost' : 'total_tokens'
  return [...props.accounts].sort((a, b) => b[metricKey] - a[metricKey])
})

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const formatNumber = (value: number): string => value.toLocaleString()

const formatCost = (value: number): string => {
  if (value >= 1000) return (value / 1000).toFixed(2) + 'K'
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(4)
}
</script>
