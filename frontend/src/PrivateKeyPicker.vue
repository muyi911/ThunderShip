<script setup lang="ts">
import { computed, ref } from "vue";
import { api } from "./api";
import type { Server } from "./types";
import IconButton from "./IconButton.vue";

const props = defineProps<{
  modelValue: string;
  servers: Server[];
  disabled?: boolean;
}>();
const emit = defineEmits<{
  "update:modelValue": [path: string];
  error: [error: unknown];
}>();
const open = ref(false),
  selected = ref("");
const keys = computed(() => {
  const paths = new Map<
    string,
    { path: string; name: string; connections: string[] }
  >();
  for (const server of [
    ...props.servers,
    { keyPath: props.modelValue, name: "" },
  ]) {
    const path = server.keyPath.trim();
    if (!path) continue;
    const id = path.replaceAll("\\", "/").toLowerCase();
    const entry = paths.get(id) || {
      path,
      name: path.split(/[\\/]/).at(-1) || path,
      connections: [],
    };
    if (server.name && !entry.connections.includes(server.name))
      entry.connections.push(server.name);
    paths.set(id, entry);
  }
  return [...paths.values()];
});
function choose(path: string) {
  emit("update:modelValue", path);
  open.value = false;
}
async function browse() {
  try {
    const path = await api("PickKey");
    if (path) choose(path);
  } catch (error) {
    emit("error", error);
  }
}
function show() {
  selected.value = props.modelValue;
  open.value = true;
}
</script>
<template>
  <v-text-field
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    aria-label="私钥文件"
    placeholder="选择或填写本地私钥路径"
    variant="solo-filled"
    flat
    :disabled="disabled"
  >
    <template #append-inner>
      <IconButton
        icon="mdi-key-chain-variant"
        label="选择私钥"
        :disabled="disabled"
        @click="show"
      />
    </template>
  </v-text-field>
  <v-dialog v-model="open" max-width="680" scrollable>
    <v-card>
      <v-card-title class="d-flex align-center">
        选择私钥<v-spacer /><IconButton
          icon="mdi-close"
          label="关闭私钥选择"
          @click="open = false"
        />
      </v-card-title>
      <v-card-text>
        <v-table v-if="keys.length" density="compact" class="key-table">
          <thead>
            <tr>
              <th>文件名</th>
              <th>本地路径 / 使用此私钥的连接</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="key in keys"
              :key="key.path"
              :class="{ selected: selected === key.path }"
              tabindex="0"
              role="button"
              :aria-label="'选择私钥 ' + key.name"
              @click="selected = key.path"
              @dblclick="choose(key.path)"
              @keydown.enter.prevent="choose(key.path)"
              @keydown.space.prevent="selected = key.path"
            >
              <td>
                <v-icon size="17" class="mr-2" color="primary"
                  >mdi-key-outline</v-icon
                >{{ key.name }}
              </td>
              <td>
                <div class="key-path" :title="key.path">{{ key.path }}</div>
                <div class="text-caption text-medium-emphasis">
                  {{ key.connections.join("、") || "当前选择" }}
                </div>
              </td>
            </tr>
          </tbody>
        </v-table>
        <div v-else class="key-empty">
          <v-icon size="36" color="primary">mdi-key-outline</v-icon>
          <p>还没有使用过的私钥</p>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-btn prepend-icon="mdi-folder-open-outline" @click="browse"
          >选择本地文件</v-btn
        >
        <v-spacer /><v-btn @click="open = false">取消</v-btn>
        <v-btn color="primary" :disabled="!selected" @click="choose(selected)"
          >使用私钥</v-btn
        >
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
<style scoped>
.key-table {
  min-height: 240px;
}
.key-table tr[role="button"] {
  cursor: pointer;
}
.key-table tr.selected {
  background: rgba(var(--v-theme-primary), 0.12);
}
.key-table td {
  padding-block: 10px !important;
}
.key-path {
  max-width: 380px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.key-empty {
  min-height: 240px;
  display: grid;
  align-content: center;
  justify-items: center;
  gap: 12px;
}
</style>
