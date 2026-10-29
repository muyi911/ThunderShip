<script setup lang="ts">
import { computed, ref, watch } from "vue";
import IconButton from "./IconButton.vue";
const props = defineProps<{
  open: boolean;
  modelValue: string;
  title: string;
  saveLabel?: string;
  saving?: boolean;
  error?: string;
  hint?: string;
  saved?: boolean;
}>();
const emit = defineEmits<{
  "update:open": [boolean];
  "update:modelValue": [string];
  save: [string];
}>();
const draft = ref("");
const fullscreen = ref(false);
const wrap = ref(false);
const discard = ref(false);
const dirty = computed(() => draft.value !== props.modelValue);
watch(
  () => props.open,
  (open) => {
    if (open) {
      draft.value = props.modelValue;
      discard.value = false;
    }
  },
  { immediate: true },
);
function apply() {
  if (props.saving) return;
  if (props.saveLabel) {
    if (dirty.value) emit("save", draft.value);
    return;
  }
  emit("update:modelValue", draft.value);
  emit("update:open", false);
}
function close() {
  if (props.saving) return;
  if (props.saveLabel && dirty.value) discard.value = true;
  else emit("update:open", false);
}
</script>
<template>
  <v-dialog
    :model-value="open"
    @update:model-value="!$event && close()"
    :persistent="!!saveLabel"
    @click:outside="close"
    @keydown.esc.stop.prevent="close"
    :fullscreen="fullscreen"
    :width="fullscreen ? '100vw' : '94vw'"
    :max-width="fullscreen ? undefined : 1280"
    :height="fullscreen ? '100vh' : '85vh'"
    scrollable
    aria-labelledby="code-editor-title"
  >
    <v-card
      class="code-editor-dialog"
      @keydown.ctrl.enter.prevent="apply"
      @keydown.ctrl.s.prevent="saveLabel && apply()"
    >
      <v-card-title class="d-flex align-center ga-2">
        <v-icon size="20">mdi-console-line</v-icon
        ><span id="code-editor-title" class="editor-title" :title="title">{{ title }}<span v-if="saveLabel && dirty"> *</span></span
        ><v-spacer />
        <v-btn
          size="small"
          :variant="wrap ? 'tonal' : 'text'"
          :aria-pressed="wrap"
          @click="wrap = !wrap"
          title="仅调整显示，不会插入换行"
          prepend-icon="mdi-wrap"
          >自动换行</v-btn
        >
        <IconButton
          :icon="fullscreen ? 'mdi-fullscreen-exit' : 'mdi-fullscreen'"
          :label="fullscreen ? '退出全屏编辑' : '全屏编辑'"
          @click="fullscreen = !fullscreen"
        />
        <IconButton
          icon="mdi-close"
          :label="saveLabel ? '关闭文件编辑器' : '取消编辑并关闭'"
          :disabled="saving"
          @click="close"
        />
      </v-card-title>
      <p v-if="hint" class="editor-hint">{{ hint }}</p>
      <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mx-5 mb-3">{{ error }}</v-alert>
      <v-card-text class="code-editor-body">
        <textarea
          v-model="draft"
          :disabled="saving"
          :aria-label="title"
          :wrap="wrap ? 'soft' : 'off'"
          :class="{ wrapped: wrap }"
          spellcheck="false"
          autocomplete="off"
          autocapitalize="off"
        />
      </v-card-text>
      <v-card-actions class="px-5 pb-4"
        ><span class="muted">{{ saveLabel ? 'Ctrl + S 保存到服务器' : 'Ctrl + Enter 应用到当前节点' }}</span><span v-if="saveLabel && saved && !dirty" class="text-success ml-3">已保存</span><v-spacer />
        <v-btn :disabled="saving" @click="close">{{ saveLabel ? '关闭' : '取消' }}</v-btn
        ><v-btn color="primary" variant="flat" :loading="saving" :disabled="!!saveLabel && !dirty" @click="apply">{{ saveLabel || '应用' }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
  <v-dialog v-model="discard" max-width="420" persistent>
    <v-card title="放弃未保存的修改？">
      <v-card-text>修改尚未保存到服务器，关闭后将丢弃当前草稿。</v-card-text>
      <v-card-actions><v-spacer /><v-btn @click="discard = false">继续编辑</v-btn><v-btn color="error" @click="discard = false; emit('update:open', false)">放弃修改</v-btn></v-card-actions>
    </v-card>
  </v-dialog>
</template>
<style scoped>
.editor-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.editor-hint { flex-shrink: 0; margin: 0 20px 12px; color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
.code-editor-body {
  display: flex;
  padding: 0 20px 12px !important;
  min-height: 0;
}
textarea {
  flex: 1;
  width: 100%;
  min-height: 180px;
  padding: 16px;
  border: 1px solid var(--border);
  border-radius: 5px;
  outline: none;
  resize: none;
  overflow: auto;
  white-space: pre;
  overflow-wrap: normal;
  background: var(--soft);
  color: rgb(var(--v-theme-on-surface));
  font:
    13px/1.7 var(--terminal-font-family);
  tab-size: 2;
}
textarea:focus {
  border-color: rgb(var(--v-theme-primary));
}
textarea.wrapped {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
