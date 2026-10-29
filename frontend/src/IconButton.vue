<script setup lang="ts">
defineProps<{
  icon: string;
  label: string;
  disabled?: boolean;
  color?: string;
  size?: string;
}>();
defineEmits<{ click: [] }>();
</script>
<template>
  <v-tooltip :text="label" location="bottom" max-width="360"
    ><template #activator="{ props }"
      ><v-btn
        v-bind="props"
        :icon="icon"
        :aria-label="label"
        :title="label"
        :disabled="disabled"
        :color="color"
        :size="size || 'small'"
        @click="$emit('click')" /></template
  ></v-tooltip>
</template>
