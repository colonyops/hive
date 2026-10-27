<script setup lang="ts">
import HiveMark from './HiveMark.vue'

defineProps<{ title: string; subtitle: string }>()
</script>

<template>
  <div class="auth-stage relative flex min-h-screen items-center justify-center overflow-hidden bg-app px-4">
    <div class="auth-glow auth-glow-a" aria-hidden="true" />
    <div class="auth-glow auth-glow-b" aria-hidden="true" />
    <svg class="auth-comb absolute -left-28 -top-28 h-[calc(100%+14rem)] w-[calc(100%+14rem)]" aria-hidden="true">
      <defs>
        <pattern id="comb" width="56" height="100" patternUnits="userSpaceOnUse" patternTransform="scale(1.1)">
          <path
            d="M28 66L0 50L0 16L28 0L56 16L56 50L28 66L28 100M28 0L28 34L0 50L0 84L28 100L56 84L56 50L28 34"
            fill="none"
            stroke="var(--color-text-4)"
            stroke-width="1"
          />
        </pattern>
        <radialGradient id="comb-fade" cx="50%" cy="50%" r="65%">
          <stop offset="0" stop-color="#fff" stop-opacity="1" />
          <stop offset="1" stop-color="#fff" stop-opacity="0" />
        </radialGradient>
        <mask id="comb-mask"><rect width="100%" height="100%" fill="url(#comb-fade)" /></mask>
      </defs>
      <rect width="100%" height="100%" fill="url(#comb)" mask="url(#comb-mask)" />
    </svg>
    <main
      class="relative flex w-full max-w-sm flex-col gap-5 rounded-xl border border-card bg-raised p-6 shadow-popover"
    >
      <div class="flex items-center gap-3">
        <HiveMark :size="36" />
        <div>
          <h1 class="text-heading font-semibold text-text">{{ title }}</h1>
          <p class="mt-0.5 text-small text-text-3">{{ subtitle }}</p>
        </div>
      </div>
      <slot />
    </main>
  </div>
</template>

<style scoped>
.auth-comb {
  opacity: 0.22;
  animation: comb-drift 90s linear infinite;
}
.auth-glow {
  position: absolute;
  width: 46rem;
  height: 46rem;
  border-radius: 9999px;
  background: radial-gradient(circle, color-mix(in srgb, var(--color-accent) 20%, transparent), transparent 65%);
  filter: blur(20px);
}
.auth-glow-a {
  top: -16rem;
  left: -12rem;
  animation: glow-a 28s ease-in-out infinite alternate;
}
.auth-glow-b {
  right: -14rem;
  bottom: -18rem;
  animation: glow-b 34s ease-in-out infinite alternate;
}
@keyframes comb-drift {
  to {
    transform: translate(56px, 100px);
  }
}
@keyframes glow-a {
  to {
    transform: translate(10rem, 6rem) scale(1.15);
  }
}
@keyframes glow-b {
  to {
    transform: translate(-9rem, -5rem) scale(1.1);
  }
}
@media (prefers-reduced-motion: reduce) {
  .auth-comb,
  .auth-glow {
    animation: none;
  }
}
</style>
