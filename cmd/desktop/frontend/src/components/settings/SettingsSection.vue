<script setup lang="ts">
// A labelled group of related settings. The header is a small tracked caps
// label with the description on the same baseline, so the eye lands on group
// boundaries rather than on prose — settings pages are scanned for the one
// control you came for, not read top to bottom.
//
// The header comes from SettingsHeading so section and group labels cannot
// drift apart; this component adds the grouping and spacing around it.
//
// `boxed` draws the group as a single bordered card with hairlines between its
// children, which is what makes a section read as one unit. Leave it off when
// the slot already renders its own cards. `padded` additionally insets each
// child, for slots holding bare controls rather than row components that
// already carry their own padding.
import SettingsHeading from './SettingsHeading.vue'

withDefaults(defineProps<{
  title: string
  description?: string
  boxed?: boolean
  padded?: boolean
  testid?: string
}>(), {
  boxed: false,
  padded: false,
})
</script>

<template>
  <section class="flex flex-col gap-2.5" :data-testid="testid">
    <SettingsHeading :title="title" :description="description">
      <template v-if="$slots.actions" #actions><slot name="actions" /></template>
    </SettingsHeading>
    <div
      v-if="boxed"
      class="divide-y divide-row overflow-hidden rounded-[11px] border border-card bg-raised"
      :class="padded ? '[&>*]:px-4 [&>*]:py-3.5' : ''"
    >
      <slot />
    </div>
    <slot v-else />
  </section>
</template>
