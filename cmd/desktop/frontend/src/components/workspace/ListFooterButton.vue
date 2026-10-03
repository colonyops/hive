<!-- The accent action row a workspace list card ends with; the card's hairlines
     separate it from the rows above. -->
<template>
  <button
    type="button"
    class="flex w-full cursor-pointer items-center gap-2 px-4 py-3 text-left text-[13px] font-medium text-accent hover:bg-chip disabled:cursor-not-allowed disabled:opacity-50"
  >
    <slot />
  </button>
</template>
