<script setup lang="ts">
import AuthLayout from '../components/AuthLayout.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import FormField from '@hive/ui/components/ui/FormField.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, describeApiError } from '../api'
import { redirectQuery } from '../lib/redirect'

const route = useRoute()
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
    await router.push({
      name: 'verify',
      query: { email: email.value.trim(), ...redirectQuery(route.query.redirect) },
    })
  } catch (cause) {
    error.value = describeApiError(cause, 'Could not start sign-in.')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AuthLayout title="Hive Drive" subtitle="Enter your email. We send you a sign-in code.">
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
  </AuthLayout>
</template>
