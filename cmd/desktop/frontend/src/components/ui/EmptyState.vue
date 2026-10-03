<script setup lang="ts">
withDefaults(
  defineProps<{
    message?: string
    /**
     * `plain` centers the message in an empty list or pane; `boxed` draws it as
     * a card; `inline` is a left-aligned note inside a list, card, or sidebar
     * section, padded by the caller to match its rows.
     */
    variant?: 'plain' | 'boxed' | 'inline'
  }>(),
  { message: undefined, variant: 'plain' },
)
</script>

<template>
  <div
    :class="
      {
        plain: 'py-8 text-center text-xs text-text-4',
        boxed: 'rounded-lg border border-border bg-raised px-4 py-8 text-center text-xs text-text-3',
        inline: 'text-xs leading-relaxed text-text-3',
      }[variant]
    "
  >
    <slot>{{ message }}</slot>
  </div>
</template>
