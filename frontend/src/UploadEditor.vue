<script setup lang="ts">
import { computed, ref } from "vue";
import type { Step, Artifact, ArtifactResource, FileMapping } from "./types";
import { api, uid } from "./api";
import FileSelection from "./FileSelection.vue";
import { appendLocalFiles } from "./composables/useLocalPicker";
import {
  syncUploadMappings,
  addUploadDirectory,
  uploadDestination,
} from "./uploadMappings";
import IconButton from "./IconButton.vue";
const props = defineProps<{ step: Step; artifacts: ArtifactResource[] }>();
const emit = defineEmits<{ error: [unknown]; manage: [] }>();
const preview = ref<Artifact[] | null>(null);
const directoryDialog = ref(false);
const directory = ref("");
const directoryError = ref("");
const asset = computed(() =>
  props.artifacts.find((a) => a.id === props.step.artifactId),
);
const legacy = computed(
  () => !props.step.artifactId && props.step.files.some((f) => f.local),
);
const choices = computed(() =>
  legacy.value
    ? [{ id: "", name: "本地文件（旧配置）" }, ...props.artifacts]
    : props.artifacts,
);
function choose(id: string | null) {
  const same = props.step.artifactId === id;
  props.step.artifactId = id || "";
  const selected = props.artifacts.find((a) => a.id === id);
  props.step.files = selected
    ? syncUploadMappings(selected, same ? props.step.files : [])
    : [];
}
function addDirectory() {
  if (!asset.value) return;
  try {
    props.step.files = addUploadDirectory(
      asset.value,
      props.step.files,
      directory.value,
    );
    directoryDialog.value = false;
  } catch (e) {
    directoryError.value = String(e);
  }
}
function anotherTarget(file: FileMapping, index: number) {
  props.step.files.splice(index + 1, 0, { ...file, id: uid(), remote: "" });
}
function source(mapping: FileMapping) {
  return asset.value?.files.find((f) => f.id === mapping.source);
}
function folder(mapping: FileMapping) {
  return (source(mapping) || mapping).kind === "folder";
}
function destination(mapping: FileMapping) {
  return uploadDestination(mapping, source(mapping) || mapping);
}
async function showPreview() {
  try {
    const files = props.step.files.map((f) =>
      props.step.artifactId ? { ...f, local: source(f)?.local || "" } : f,
    );
    preview.value = await api("PreviewFiles", files);
  } catch (e) {
    emit("error", e);
  }
}
</script>
<template>
  <div class="d-flex flex-column ga-3">
    <v-select
      label="引用构建物"
      :items="choices"
      item-title="name"
      item-value="id"
      :model-value="step.artifactId || ''"
      @update:model-value="choose"
    />
    <v-btn
      size="small"
      prepend-icon="mdi-package-variant-closed"
      @click="emit('manage')"
      >管理构建物</v-btn
    >
    <template v-if="step.artifactId">
      <div class="upload-toolbar">
        <v-btn
          size="small"
          prepend-icon="mdi-folder-plus-outline"
          :disabled="!asset"
          @click="
            directory = '';
            directoryError = '';
            directoryDialog = true;
          "
          >添加目标目录</v-btn
        >
        <v-btn
          size="small"
          :disabled="!asset"
          title="同步资产里的全部文件，保留已有文件的目标配置"
          @click="choose(step.artifactId)"
          >同步构建物文件</v-btn
        ><v-btn size="small" :disabled="!step.files.length" @click="showPreview"
          >预览目标</v-btn
        >
      </div>
      <div v-for="(f, i) in step.files" :key="f.id" class="file-mapping">
        <div class="local-file">
          <v-icon size="17">{{
            folder(f) ? "mdi-folder-outline" : "mdi-file-document-outline"
          }}</v-icon
          ><span :title="source(f)?.local">{{
            source(f)?.name || source(f)?.local || "构建物文件已移除"
          }}</span
          ><IconButton
            icon="mdi-content-copy"
            label="为此文件添加另一个上传目标"
            @click="anotherTarget(f, i)"
          /><v-btn
            size="x-small"
            icon="mdi-close"
            title="移除此上传映射"
            aria-label="移除此上传映射"
            @click="step.files.splice(i, 1)"
          />
        </div>
        <v-text-field
          :label="
            folder(f)
              ? f.includeFolder
                ? '远程目标目录（放入文件夹）'
                : '远程目录（放入内容）'
              : '远程文件完整路径'
          "
          v-model="f.remote"
        />
        <template v-if="folder(f)">
          <v-checkbox label="包含文件夹本身" v-model="f.includeFolder" />
          <div class="upload-destination">
            {{
              f.includeFolder
                ? "文件夹及其内容上传到"
                : "仅文件夹内的内容上传到"
            }}：
            <code>{{ destination(f) || "请填写远程目录" }}/…</code>
          </div>
        </template>
        <v-checkbox label="可执行" v-model="f.executable" />
      </div>
    </template>
    <template v-else-if="legacy">
      <div class="upload-toolbar">
        <FileSelection
          :initial-path="step.files[0]?.local"
          @add="appendLocalFiles(step.files, $event)"
          @error="emit('error', $event)"
        /><span class="spacer" /><v-btn
          size="small"
          :disabled="!step.files.length"
          @click="showPreview"
          >预览目标</v-btn
        >
      </div>
      <div v-if="!step.files.length" class="upload-empty">
        <v-icon size="29" color="primary">mdi-folder-upload-outline</v-icon>
        <div>添加本地构建产物</div>
        <small>可以分次添加多个文件或文件夹，运行时一起发布</small>
      </div>
      <div v-for="(f, i) in step.files" :key="f.id" class="file-mapping">
        <div class="local-file">
          <v-icon size="17">mdi-file-outline</v-icon
          ><span :title="f.local">{{ f.local }}</span
          ><IconButton
            icon="mdi-content-copy"
            label="为此文件添加另一个上传目标"
            @click="anotherTarget(f, i)"
          /><v-btn
            size="x-small"
            icon="mdi-close"
            aria-label="移除文件"
            @click="step.files.splice(i, 1)"
          />
        </div>
        <div class="remote-file">
          <v-icon size="16">mdi-subdirectory-arrow-right</v-icon
          ><v-text-field
            :label="folder(f) ? '远程目标目录' : '远程文件完整路径'"
            v-model="f.remote"
          /><v-checkbox label="可执行" v-model="f.executable" />
        </div>
        <template v-if="folder(f)">
          <v-checkbox label="包含文件夹本身" v-model="f.includeFolder" />
          <div class="upload-destination">
            {{
              f.includeFolder
                ? "文件夹及其内容上传到"
                : "仅文件夹内的内容上传到"
            }}：
            <code>{{ destination(f) || "请填写远程目录" }}/…</code>
          </div>
        </template>
      </div>
    </template>
    <div class="upload-note">
      <v-icon size="14">mdi-shield-check-outline</v-icon> 同目录临时上传 ·
      SHA-256 校验 · 原子替换
    </div>
    <details class="advanced">
      <summary>发布前命令（可选）</summary>
      <v-textarea
        v-model="step.beforePublish"
        rows="2"
        variant="outlined"
        label="全部文件准备好后、替换前执行，例如停服"
        class="code-input"
      />
    </details>
  </div>
  <v-dialog v-model="directoryDialog" max-width="560"
    ><v-card title="添加上传目标目录"
      ><v-card-text class="d-flex flex-column ga-3">
        <v-alert v-if="directoryError" type="error" variant="tonal">{{
          directoryError
        }}</v-alert>
        <div>
          将“{{
            asset?.name
          }}”的全部文件和文件夹添加到此目录，保留文件名与目录层级。
        </div>
        <v-text-field
          v-model="directory"
          label="远程目标目录"
          placeholder="/home/work/service-b"
          @keyup.enter="addDirectory"
        /> </v-card-text
      ><v-card-actions
        ><v-spacer /><v-btn @click="directoryDialog = false">取消</v-btn
        ><v-btn
          color="primary"
          variant="flat"
          :disabled="!directory.trim() || !asset"
          @click="addDirectory"
          >添加目标</v-btn
        ></v-card-actions
      ></v-card
    ></v-dialog
  >
  <v-dialog
    :model-value="preview !== null"
    @update:model-value="preview = null"
    max-width="900"
    ><v-card title="最终目标文件"
      ><v-card-text
        ><v-table density="compact"
          ><thead>
            <tr>
              <th>本地文件</th>
              <th>远程文件</th>
              <th>大小</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in preview" :key="f.remote">
              <td class="code">{{ f.local }}</td>
              <td class="code">{{ f.remote }}</td>
              <td>{{ f.size }} B</td>
            </tr>
          </tbody></v-table
        ></v-card-text
      ><v-card-actions
        ><v-spacer /><v-btn @click="preview = null">关闭</v-btn></v-card-actions
      ></v-card
    ></v-dialog
  >
</template>
<style scoped>
.upload-destination {
  margin: 0 0 8px 10px;
  color: var(--muted);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.upload-destination code {
  display: block;
  margin-top: 3px;
  color: rgb(var(--v-theme-primary));
}
</style>
