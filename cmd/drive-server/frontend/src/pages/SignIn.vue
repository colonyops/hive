<script setup lang="ts">
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import FormField from '@hive/ui/components/ui/FormField.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError } from '../api'

const router = useRouter()
const email = ref('')
const busy = ref(false)
const error = ref<string | null>(null)

async function submit(): Promise<void> {
  if (busy.value || !email.value.trim()) return
  busy.value = true
  error.value = null
  try {
    await api.startSignIn(email.value.trim())
    await router.push({ name: 'verify', query: { email: email.value.trim() } })
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 429) {
      const wait = cause.retryAfter ? ` Try again in ${cause.retryAfter} seconds.` : ' Try again later.'
      error.value = `Too many sign-in attempts.${wait}`
    } else {
      error.value = cause instanceof Error ? cause.message : 'Could not start sign-in.'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="mx-auto flex min-h-screen max-w-sm flex-col justify-center gap-5 px-4">
    <div>
      <h1 class="text-heading font-semibold text-text">Hive Drive</h1>
      <p class="mt-1 text-body text-text-3">Enter your email. We send you a sign-in code.</p>
    </div>
    <form class="flex flex-col gap-4" data-testid="sign-in-form" @submit.prevent="submit">
      <FormField label="Email">
        <template #default="{ id }">
          <TextInput :id="id" v-model="email" type="email" autocomplete="email" data-testid="sign-in-email" />
        </template>
      </FormField>
      <InlineError v-if="error" :message="error" testid="sign-in-error" />
      <BaseButton type="submit" :busy="busy" :disabled="!email.trim()" data-testid="sign-in-submit">
        Send code
      </BaseButton>
    </form>
  </main>
</template>
