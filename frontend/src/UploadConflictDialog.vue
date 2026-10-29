<script setup lang="ts">
import { ref, watch } from "vue";
import type { UploadPreview } from "./types";
const props = defineProps<{ file: UploadPreview | null }>();
const emit = defineEmits<{ decide: [action: "overwrite" | "skip" | "cancel", all: boolean] }>();
const all = ref(false);
watch(() => props.file, () => { all.value = false; });
function size(bytes: number) {
  if (bytes < 1024) return bytes + "B";
  const n = Math.min(3, Math.floor(Math.log(bytes) / Math.log(1024)));
  return (bytes / 1024 ** n).toFixed(1) + ["B", "KB", "MB", "GB"][n];
}
function time(value: string) { return new Date(value).toLocaleString("sv-SE"); }
</script>
<template>
  <v-dialog :model-value="!!file" max-width="600" persistent @keydown.esc.prevent="emit('decide', 'cancel', false)">
    <v-card v-if="file" class="upload-conflict">
      <div class="conflict-heading"><strong>文件已存在！</strong><span class="spacer" /><v-btn icon="mdi-close" size="x-small" variant="text" aria-label="取消上传" @click="emit('decide', 'cancel', false)" /></div>
      <div class="conflict-content">
        <strong class="conflict-name" :title="file.file.remote">{{ file.file.remote.split('/').at(-1) }}</strong>
        <p>目标位置已有同名文件，是否覆盖？</p>
        <div class="conflict-comparison">
          <template v-for="(info, index) in [file.local, file.remote!]" :key="index">
            <v-icon v-if="index" class="conflict-arrow" size="20">mdi-arrow-right</v-icon>
            <div class="conflict-file">
              <header><span><v-icon size="16">{{ index ? 'mdi-monitor-arrow-down-variant' : 'mdi-monitor' }}</v-icon> {{ index ? '远程文件' : '本地文件' }}</span><time>{{ time(info.modified) }}</time></header>
              <div class="conflict-values"><div><small>大小</small><strong>{{ size(info.size) }}</strong></div><div><small>权限</small><code>{{ info.mode }}</code></div></div>
            </div>
          </template>
        </div>
      </div>
      <footer><v-checkbox v-model="all" label="全部应用" density="compact" hide-details /><span class="spacer" /><v-btn variant="text" @click="emit('decide', 'skip', all)">跳过</v-btn><v-btn variant="tonal" @click="emit('decide', 'overwrite', all)">覆盖</v-btn></footer>
    </v-card>
  </v-dialog>
</template>
<style scoped>
.upload-conflict { padding: 12px 16px 14px; border: 1px solid var(--border); }
.conflict-heading { display: flex; align-items: center; margin: 0 2px 10px; font-size: 18px; }
.conflict-content { padding: 16px; border-radius: 5px; background: rgba(0, 0, 0, .22); }
.conflict-name { display: block; overflow-wrap: anywhere; font-family: var(--font-mono, monospace); }
.conflict-content p { color: var(--muted); margin: 5px 0 16px; font-size: 12px; }
.conflict-comparison { display: grid; grid-template-columns: minmax(0, 1fr) 20px minmax(0, 1fr); align-items: center; gap: 8px; }
.conflict-file { padding: 12px; border: 1px solid var(--border); border-radius: 4px; min-width: 0; }
.conflict-file header { display: flex; justify-content: space-between; align-items: center; gap: 8px; font-size: 12px; }
.conflict-file header span { white-space: nowrap; font-weight: 600; }
.conflict-file time { max-width: 86px; text-align: right; color: var(--muted); font-size: 11px; }
.conflict-values { display: flex; justify-content: space-between; gap: 6px; margin-top: 18px; }
.conflict-values > div { display: flex; flex-direction: column; gap: 3px; }
.conflict-values > div:last-child { text-align: right; }
.conflict-values small { color: var(--muted); font-size: 11px; }
.conflict-values strong { font-size: 20px; }
.conflict-values code { font-size: 12px; margin-top: 6px; }
footer { display: flex; align-items: center; gap: 12px; padding-top: 10px; }
footer .v-checkbox { flex: 0 0 auto; }
footer :deep(.v-label) { font-size: 12px; }
@media (max-width: 520px) { .conflict-comparison { grid-template-columns: 1fr; } .conflict-arrow { justify-self: center; transform: rotate(90deg); } }
</style>
