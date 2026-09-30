<script setup lang="ts">
// Gitea's brand mark, wrapping the bundled asset the way GrafanaMark does: the
// logo is a fixed two-colour drawing with no currentColor part, so there is
// nothing for an inline SVG to inherit from the theme and an <img> keeps one
// copy of the artwork shared with the Integrations screen.
//
// Forgejo items wear it too — the connector covers both forges, and shipping a
// second mark would mean guessing which server answered from a payload that
// does not say.
import giteaLogo from '../../assets/integrations/gitea.svg'
</script>

<template>
  <img :src="giteaLogo" alt="" aria-hidden="true" class="object-contain">
</template>
