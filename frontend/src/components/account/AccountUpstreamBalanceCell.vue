<template>
  <div v-if="isSupported" class="space-y-1">
    <div v-if="summary" class="space-y-0.5">
      <div class="text-[11px] font-medium" :class="statusClass">
        {{ balanceLine }}
      </div>
      <div
        v-if="metaLine"
        class="text-[10px] text-gray-500 dark:text-gray-400 truncate max-w-[140px]"
        :title="summary.message || summary.base_url || ''"
      >
        {{ metaLine }}
      </div>
    </div>
    <div v-else-if="loading" class="space-y-1">
      <div class="h-3 w-16 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>
    <div v-else class="text-xs text-gray-400">-</div>
  </div>
  <div v-else class="text-xs text-gray-400">-</div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageInfo } from '@/types'

const props = withDefaults(defineProps<{
  account: Account
  manualRefreshToken?: number
}>(), {
  manualRefreshToken: 0
})

const loading = ref(false)
const usageInfo = ref<AccountUsageInfo | null>(null)

const isSupported = computed(() => props.account.platform === 'openai' && props.account.type === 'apikey')

const summary = computed(() => usageInfo.value?.platform_usage || null)

const loadBalance = async (source?: 'passive' | 'active') => {
  if (!isSupported.value) return
  loading.value = true
  try {
    usageInfo.value = await adminAPI.accounts.getUsage(props.account.id, source)
  } catch (error) {
    console.error('Failed to load upstream balance:', error)
  } finally {
    loading.value = false
  }
}

const balanceLine = computed(() => {
  if (!summary.value) return '-'
  if (summary.value.remaining == null) return summary.value.status || '-'
  const value = summary.value.remaining
  const formatted = value >= 100 ? value.toFixed(0) : value >= 10 ? value.toFixed(2) : value.toFixed(3)
  return `$${formatted}`
})

const metaLine = computed(() => {
  if (!summary.value) return ''
  const status = summary.value.status || 'ok'
  if (status !== 'ok') return status
  return ''
})

const statusClass = computed(() => {
  const status = summary.value?.status
  if (status === 'ok') return 'text-emerald-600 dark:text-emerald-400'
  if (status === 'invalid_key') return 'text-red-600 dark:text-red-400'
  return 'text-amber-600 dark:text-amber-400'
})

watch(
  () => props.manualRefreshToken,
  async () => {
    await loadBalance('active')
  }
)

watch(
  () => props.account.id,
  async () => {
    usageInfo.value = null
    await loadBalance()
  }
)

onMounted(async () => {
  await loadBalance()
})
</script>
