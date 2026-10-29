<script setup lang="ts">
defineProps<{ items: string[] }>();
defineEmits<{ resolve: [number] }>();
</script>
<template>
  <div v-if="items.length" class="unresolved">
    <strong>AI 草稿待补充 · 完成后逐项确认</strong>
    <v-checkbox
      v-for="(item, i) in items"
      :key="i"
      :label="item"
      :model-value="false"
      @update:model-value="$emit('resolve', i)"
    />
  </div>
</template>
