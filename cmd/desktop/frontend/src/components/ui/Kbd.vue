<script setup lang="ts">
withDefaults(defineProps<{ variant?: 'plain' | 'boxed' | 'on-accent' }>(), { variant: 'plain' })
</script>

<template>
  <kbd
    :class="[
      'font-mono',
      {
        boxed: 'rounded border border-card px-1.5 py-0.5 text-[10.5px] leading-none text-text-3',
        // Sits on a filled primary button, so it darkens the fill instead of drawing a border.
        'on-accent': 'rounded bg-black/15 px-1 py-0.5 text-[10.5px] leading-none',
        plain: '',
      }[variant],
    ]"
    ><slot
  /></kbd>
</template>
