<script setup lang="ts">
import IconActivity from '~icons/lucide/activity'
import TokenConnectDrawer, { type TokenProvider } from './TokenConnectDrawer.vue'
import {
  Connect,
  Disconnect,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/grafanaservice'

const emit = defineEmits<{ close: [] }>()

const provider: TokenProvider = {
  id: 'grafana',
  name: 'Grafana',
  icon: IconActivity,
  noun: 'stack',
  connectLabel: 'Connect a stack',
  // Read-only (Viewer) is all the connector needs, so guidance points at the
  // least-privilege token.
  docsUrl: 'https://grafana.com/docs/grafana/latest/administration/service-accounts/',
  tokenPage: { path: '/org/serviceaccounts', label: 'Service accounts on your stack' },
  urlPlaceholder: 'https://grafana.example.com',
  tokenPlaceholder: 'glsa_…',
  connect: Connect,
  disconnect: Disconnect,
}
</script>

<template>
  <TokenConnectDrawer :provider="provider" @close="emit('close')">
    Create a <span class="text-text-2">service account</span> with the <span class="text-text-2">Viewer</span> role,
    then add a token — Viewer can query metrics and read alerts. Paste the token below.
  </TokenConnectDrawer>
</template>
