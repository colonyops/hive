<script setup lang="ts">
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import FormField from '@hive/ui/components/ui/FormField.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '../api'
import { forgetSignIn } from '../router'

const route = useRoute()
const router = useRouter()
const email = ref(typeof route.query.email === 'string' ? route.query.email : '')
const code = ref('')
const busy = ref(false)
const error = ref<string | null>(null)

async function submit(): Promise<void> {
  if (busy.value || !email.value.trim() || !code.value.trim()) return
  busy.value = true
  error.value = null
  try {
    await api.verifySignIn(email.value.trim(), code.value.trim())
    forgetSignIn()
    await router.replace({ name: 'spaces' })
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 429) {
      error.value = `Too many attempts. Try again in ${cause.retryAfter ?? 60} seconds.`
    } else {
      error.value = cause instanceof Error ? cause.message : 'Could not verify the code.'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="mx-auto flex min-h-screen max-w-sm flex-col justify-center gap-5 px-4">
    <div>
      <h1 class="text-heading font-semibold text-text">Enter your code</h1>
      <p class="mt-1 text-body text-text-3">Use the 8-digit code we mailed to you. It expires soon.</p>
    </div>
    <form class="flex flex-col gap-4" data-testid="verify-form" @submit.prevent="submit">
      <FormField label="Email">
        <template #default="{ id }">
          <TextInput :id="id" v-model="email" type="email" autocomplete="email" data-testid="verify-email" />
        </template>
      </FormField>
      <FormField label="Code">
        <template #default="{ id }">
          <TextInput
            :id="id"
            v-model="code"
            inputmode="numeric"
            autocomplete="one-time-code"
            monospace
            data-testid="verify-code"
          />
        </template>
      </FormField>
      <InlineError v-if="error" :message="error" testid="verify-error" />
      <BaseButton type="submit" :busy="busy" :disabled="!email.trim() || !code.trim()" data-testid="verify-submit">
        Sign in
      </BaseButton>
      <RouterLink :to="{ name: 'sign-in' }" class="text-small text-text-3 hover:text-text">Send a new code</RouterLink>
    </form>
  </main>
</template>
