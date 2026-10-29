<script setup lang="ts">
import { api } from "./api";
import type { FileMapping } from "./types";
import { useLocalPicker } from "./composables/useLocalPicker";
import IconButton from "./IconButton.vue";
const props = withDefaults(defineProps<{ initialPath?: string }>(), {
  initialPath: "",
});
const emit = defineEmits<{ add: [FileMapping[]]; error: [unknown] }>();
const picker = useLocalPicker((files) => emit("add", files));
const {
  opened,
  loading,
  confirming,
  error,
  address,
  directory,
  selected,
  all,
} = picker;
async function native(folder: boolean, locate = false) {
  try {
    const files = await api("PickFiles", folder);
    if (locate) {
      if (files[0]) await picker.browse(files[0].local);
    } else emit("add", files);
  } catch (e) {
    if (locate) error.value = String(e);
    else emit("error", e);
  }
}
</script>
<template>
  <div class="file-selection-actions">
    <v-btn
      size="small"
      prepend-icon="mdi-folder-multiple-outline"
      @click="picker.open(props.initialPath)"
      >批量添加</v-btn
    >
    <v-btn
      size="small"
      prepend-icon="mdi-file-plus-outline"
      @click="native(false)"
      >添加文件</v-btn
    >
    <v-btn
      size="small"
      prepend-icon="mdi-folder-plus-outline"
      @click="native(true)"
      >添加文件夹</v-btn
    >
  </div>
  <v-dialog
    :model-value="opened"
    max-width="860"
    :persistent="confirming"
    @update:model-value="picker.close"
  >
    <v-card title="选择文件和文件夹">
      <v-card-text class="local-picker-body">
        <v-alert v-if="error" type="error" variant="tonal">{{ error }}</v-alert>
        <form class="flex-row" @submit.prevent="picker.browse(address)">
          <IconButton
            icon="mdi-arrow-up"
            label="上一级目录"
            :disabled="
              loading || confirming || directory.parent === directory.path
            "
            @click="picker.browse(directory.parent)"
          />
          <v-text-field
            label="本地目录"
            v-model="address"
            :disabled="confirming"
          />
          <v-btn type="submit" size="small" :disabled="loading || confirming"
            >前往</v-btn
          >
          <v-btn
            size="small"
            :disabled="loading || confirming"
            @click="native(true, true)"
            >定位文件夹</v-btn
          >
        </form>
        <div class="local-picker-hint">
          勾选文件或整个文件夹；点击文件夹名称进入。可跨目录选择。
        </div>
        <v-progress-linear v-if="loading" indeterminate color="primary" />
        <v-table fixed-header height="300" class="local-picker-table">
          <thead>
            <tr>
              <th class="local-check">
                <v-checkbox-btn
                  :model-value="all"
                  :disabled="loading || confirming"
                  aria-label="选择当前目录全部项"
                  @update:model-value="picker.toggleAll"
                />
              </th>
              <th>名称</th>
              <th>类型</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in directory.entries" :key="entry.path">
              <td>
                <v-checkbox-btn
                  :model-value="picker.has(entry)"
                  :disabled="!entry.selectable || loading || confirming"
                  :aria-label="'选择 ' + entry.name"
                  @update:model-value="picker.toggle(entry)"
                />
              </td>
              <td>
                <button
                  class="local-entry-name"
                  :disabled="!entry.selectable || loading || confirming"
                  @click="
                    entry.directory
                      ? picker.browse(entry.path)
                      : picker.toggle(entry)
                  "
                >
                  <v-icon
                    size="19"
                    :color="entry.directory ? 'primary' : undefined"
                    >{{
                      entry.directory ? "mdi-folder" : "mdi-file-outline"
                    }}</v-icon
                  >{{ entry.name }}
                </button>
              </td>
              <td>
                {{
                  !entry.selectable
                    ? "不支持的类型"
                    : entry.directory
                      ? "文件夹"
                      : "文件"
                }}
              </td>
            </tr>
            <tr v-if="!loading && !directory.entries.length">
              <td colspan="3">此目录为空</td>
            </tr>
          </tbody>
        </v-table>
        <div class="flex-row">
          <span>已选择 {{ selected.length }} 项</span
          ><v-btn
            size="small"
            :disabled="!selected.length || confirming"
            @click="selected = []"
            >清空选择</v-btn
          >
        </div>
        <div class="local-picker-selection">
          <v-chip
            v-for="entry in selected"
            :key="entry.path"
            :title="entry.path"
            :closable="!confirming"
            @click:close="picker.toggle(entry)"
            >{{ entry.name }}</v-chip
          >
        </div>
      </v-card-text>
      <v-card-actions
        ><v-spacer /><v-btn :disabled="confirming" @click="picker.close"
          >取消</v-btn
        ><v-btn
          color="primary"
          variant="flat"
          :loading="confirming"
          :disabled="loading || !selected.length"
          @click="picker.confirm"
          >添加 {{ selected.length }} 项</v-btn
        ></v-card-actions
      >
    </v-card>
  </v-dialog>
</template>
<style scoped>
.file-selection-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.local-picker-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.local-picker-hint {
  font-size: 12px;
  color: var(--muted);
}
.local-check {
  width: 48px;
}
.local-entry-name {
  display: flex;
  gap: 10px;
  align-items: center;
  text-align: left;
  overflow-wrap: anywhere;
}
.local-entry-name:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
}
.local-picker-selection {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 32px;
  max-height: 100px;
  overflow: auto;
}
</style>
