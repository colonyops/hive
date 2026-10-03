<script setup lang="ts">
import { ref } from 'vue'
import BaseButton from '../ui/BaseButton.vue'
import FormField from '../ui/FormField.vue'

const props = defineProps<{
  accounts: string[]
  /** One account, lowercase: `account`, `stack`, `project`. */
  noun: string
  /** Prefix for `-connected`, `-connected-<account>`, `-disconnect-<account>`, `-connected-empty`. */
  testid: string
  disconnect: (account: string) => Promise<void>
}>()

const disconnecting = ref<string | null>(null)

async function onDisconnect(account: string): Promise<void> {
  disconnecting.value = account
  try {
    await props.disconnect(account)
  } finally {
    disconnecting.value = null
  }
}
</script>

<template>
  <FormField :label="`Connected ${noun}s`" :testid="`${testid}-connected`">
    <div v-if="accounts.length > 0" class="flex flex-col gap-2">
      <div
        v-for="account in accounts"
        :key="account"
        class="flex items-center justify-between gap-3 rounded-lg border border-border bg-raised px-3 py-2.5"
        :data-testid="`${testid}-connected-${account}`"
      >
        <div class="min-w-0 truncate font-mono text-[13px] text-text">{{ account }}</div>
        <BaseButton
          variant="secondary"
          size="sm"
          :busy="disconnecting === account"
          :data-testid="`${testid}-disconnect-${account}`"
          @click="onDisconnect(account)"
          >Disconnect</BaseButton
        >
      </div>
    </div>
    <div
      v-else
      class="rounded-lg border border-border bg-raised px-3 py-2.5 text-[13px] text-text-3"
      :data-testid="`${testid}-connected-empty`"
    >
      No {{ noun }} connected
    </div>
  </FormField>
</template>
