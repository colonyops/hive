<script setup lang="ts">
import RelayMark from '../marks/RelayMark.vue'
import TokenConnectDrawer, { type TokenProvider } from './TokenConnectDrawer.vue'
import {
  ConnectWithKey,
  Disconnect,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/relayservice'

const emit = defineEmits<{ close: [] }>()

const provider: TokenProvider = {
  id: 'relay',
  name: 'Hive Relay',
  // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- type-aware lint does not resolve .vue module types
  icon: RelayMark,
  noun: 'account',
  connectLabel: 'Connect a relay',
  docsUrl: 'https://hivedesktop.com/desktop/inbox/sources/#hive-relay',
  urlPlaceholder: 'https://relay.hivedesktop.com',
  tokenPlaceholder: 'API key',
  defaultUrl: 'https://relay.hivedesktop.com',
  connect: ConnectWithKey,
  disconnect: Disconnect,
}
</script>

<template>
  <TokenConnectDrawer :provider="provider" @close="emit('close')">
    Paste an API key. An operator mints one with <span class="font-mono text-text-2">hive-relay tokens create</span>;
    email sign-in arrives in a later release.
  </TokenConnectDrawer>
</template>
