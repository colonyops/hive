<script setup lang="ts">
// Grafana's brand mark. Unlike GithubMark and PostHogMark this wraps the
// bundled asset rather than inlining geometry: the logo is a single gradient
// fill with no currentColor part, so there is nothing for an inline SVG to
// inherit from the theme and an <img> keeps one copy of the artwork shared
// with the Integrations screen.
import grafanaLogo from '../../assets/integrations/grafana.svg'
</script>

<template>
  <img :src="grafanaLogo" alt="" aria-hidden="true" class="object-contain">
</template>
