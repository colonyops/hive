<script setup lang="ts">
import { useToast } from '../lib/toast'

const { message } = useToast()
</script>

<template>
  <div class="pointer-events-none fixed inset-x-0 bottom-6 z-50 flex justify-center px-4" role="status">
    <p
      v-if="message"
      class="rounded-lg border border-strong bg-raised px-4 py-2.5 text-body text-text shadow-popover"
      data-testid="toast"
    >
      {{ message }}
    </p>
  </div>
</template>
