<script setup lang="ts">
import type { FileMapping } from "./types";
import IconButton from "./IconButton.vue";
defineProps<{ file: FileMapping; disabled?: boolean }>();
defineEmits<{ replace: []; remove: [] }>();
</script>
<template>
  <div class="artifact-file-row">
    <div class="resource-icon artifact-file-icon">
      <v-icon size="22">{{
        file.kind === "folder"
          ? "mdi-folder-outline"
          : "mdi-file-document-outline"
      }}</v-icon>
    </div>
    <div class="artifact-file-content">
      <div class="artifact-file-heading">
        <v-text-field
          v-model="file.name"
          aria-label="文件显示名称"
          placeholder="文件名称"
          variant="plain"
          hide-details
          :disabled="disabled"
          class="artifact-file-name"
        />
        <span class="artifact-file-kind">{{
          file.kind === "folder" ? "文件夹" : "文件"
        }}</span>
      </div>
      <v-text-field
        v-model="file.local"
        aria-label="当前本地位置"
        placeholder="本地文件或文件夹路径"
        :title="file.local"
        :disabled="disabled"
        hide-details
        class="artifact-file-location"
      >
        <template #append-inner>
          <v-btn
            size="x-small"
            prepend-icon="mdi-folder-open-outline"
            title="重新选择本地位置"
            :disabled="disabled"
            @click="$emit('replace')"
            >更换</v-btn
          >
        </template>
      </v-text-field>
    </div>
    <IconButton
      icon="mdi-close"
      label="从构建物中移除此项，不删除本地文件"
      :disabled="disabled"
      @click="$emit('remove')"
    />
  </div>
</template>
<style scoped>
.artifact-file-row {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr) 32px;
  gap: 12px;
  padding: 14px 16px 16px;
  align-items: start;
}
.artifact-file-row + .artifact-file-row {
  border-top: 1px solid var(--border);
}
.artifact-file-icon {
  margin-top: 3px;
}
.artifact-file-content {
  min-width: 0;
}
.artifact-file-heading {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.artifact-file-name {
  min-width: 0;
}
.artifact-file-name :deep(.v-field__input) {
  min-height: 30px;
  padding: 3px 0;
  font-size: 13px;
  font-weight: 600;
}
.artifact-file-name :deep(.v-field) {
  border-radius: 3px;
}
.artifact-file-name :deep(.v-field:hover),
.artifact-file-name :deep(.v-field--focused) {
  background: rgba(var(--v-theme-on-surface), 0.04);
}
.artifact-file-kind {
  flex-shrink: 0;
  padding: 2px 7px;
  border-radius: 4px;
  background: rgba(var(--v-theme-on-surface), 0.06);
  color: var(--muted);
  font-size: 11px;
}
.artifact-file-location :deep(.v-field__input) {
  min-height: 34px;
  padding: 7px 10px;
  font-family: Consolas, "Microsoft YaHei", monospace;
  font-size: 12px;
}
.artifact-file-location :deep(.v-field__append-inner) {
  padding-top: 0;
  align-items: center;
}
@media (max-width: 600px) {
  .artifact-file-row {
    grid-template-columns: minmax(0, 1fr) 32px;
    padding: 12px;
    gap: 8px;
  }
  .artifact-file-icon {
    display: none;
  }
}
</style>
