<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { fromRouteParams } from '../lib/space'
import DocumentView from './DocumentView.vue'
import SpaceDocuments from './SpaceDocuments.vue'

const route = useRoute()
const target = computed(() => fromRouteParams(route.params))
</script>

<template>
  <DocumentView v-if="target.path" :key="route.fullPath" :space="target.space" :path="target.path" />
  <SpaceDocuments v-else :key="route.fullPath" :space="target.space" />
</template>
