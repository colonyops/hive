<script setup lang="ts">
import GiteaMark from '../marks/GiteaMark.vue'
import TokenConnectDrawer, { type TokenProvider } from './TokenConnectDrawer.vue'
import {
  Connect,
  Disconnect,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/giteaservice'

const emit = defineEmits<{ close: [] }>()

// Gitea's device flow needs an OAuth application registered on each instance,
// so an account connects with a pasted URL and access token instead.
const provider: TokenProvider = {
  id: 'gitea',
  name: 'Gitea',
  // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- type-aware lint does not resolve .vue module types
  icon: GiteaMark,
  noun: 'account',
  connectLabel: 'Connect an instance',
  docsUrl: 'https://docs.gitea.com/development/api-usage',
  // Gitea and Forgejo serve it at the same path.
  tokenPage: { path: '/user/settings/applications', label: 'Tokens on your instance' },
  urlPlaceholder: 'https://git.example.com',
  tokenPlaceholder: 'Access token',
  connect: Connect,
  disconnect: Disconnect,
}
</script>

<template>
  <TokenConnectDrawer :provider="provider" @close="emit('close')">
    Create an <span class="text-text-2">access token</span> with the
    <span class="font-mono text-text-2">read:user</span>, <span class="font-mono text-text-2">read:issue</span> and
    <span class="font-mono text-text-2">read:notification</span>
    scopes, then paste it below. Forgejo instances work the same way.
  </TokenConnectDrawer>
</template>
