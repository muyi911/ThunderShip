<script setup lang="ts">
import { computed, ref } from "vue";
import type { ArtifactResource, FileMapping } from "./types";
import type { Workspace } from "./useWorkspace";
import { api, clone } from "./api";
import IconButton from "./IconButton.vue";
import FileSelection from "./FileSelection.vue";
import ArtifactFileRow from "./ArtifactFileRow.vue";
import ResourceGrid from "./ResourceGrid.vue";
import { appendLocalFiles } from "./composables/useLocalPicker";
const props = defineProps<{ workspace: Workspace; embedded?: boolean }>();
const search = ref("");
const assets = computed(() =>
  props.workspace.orderedArtifacts.filter((asset) =>
    `${asset.name} ${asset.files.map((file) => file.local).join(" ")}`
      .toLowerCase()
      .includes(search.value.toLowerCase()),
  ),
);
const draft = ref<ArtifactResource | null>(null);
const error = ref("");
function edit(asset?: ArtifactResource) {
  draft.value = asset ? clone(asset) : { id: "", name: "", files: [] };
  error.value = "";
}
function add(files: FileMapping[]) {
  if (draft.value)
    appendLocalFiles(
      draft.value.files,
      files.map((file) => ({ ...file, remote: "" })),
    );
  error.value = "";
}
async function replace(file: FileMapping) {
  if (!draft.value) return;
  try {
    const files = await api("PickFiles", file.kind === "folder");
    if (files.length > 1) throw new Error("更换位置时请选择一个文件");
    if (files[0]) {
      file.local = files[0].local;
      file.kind = files[0].kind;
    }
    error.value = "";
  } catch (e) {
    error.value = String(e);
  }
}
async function save() {
  if (!draft.value) return;
  await props.workspace.task(async () => {
    try {
      props.workspace.cfg = await api("SaveArtifact", clone(draft.value!));
      draft.value = null;
      props.workspace.notice = "构建物已保存，引用它的工作流会使用新位置";
    } catch (e) {
      error.value = String(e);
    }
  });
}
</script>
<template>
  <div class="page-heading">
    <h1 v-if="!embedded">构建物</h1>
    <v-spacer v-else />
    <v-btn
      color="primary"
      variant="flat"
      prepend-icon="mdi-plus"
      @click="edit()"
      >添加构建物</v-btn
    >
  </div>
  <template v-if="workspace.cfg.artifacts.length">
    <div class="library-toolbar">
      <v-text-field
        v-model="search"
        prepend-inner-icon="mdi-magnify"
        placeholder="搜索构建物或文件位置"
        aria-label="搜索构建物"
        hide-details
        clearable
        @click:clear="search = ''"
      />
      <span class="resource-total">{{ assets.length }} 个构建物</span>
    </div>
    <ResourceGrid
      v-if="assets.length"
      :items="assets"
      icon="mdi-package-variant-closed"
      :busy="workspace.busy"
      @open="edit"
      @move="(id, before) => workspace.moveResource('artifacts', id, before)"
    >
      <template #default="{ item }">
        <div
          v-for="file in item.files.slice(0, 3)"
          :key="file.id"
          class="resource-file"
          :title="file.local"
        >
          <v-icon size="17">{{
            file.kind === "folder" ? "mdi-folder-outline" : "mdi-file-outline"
          }}</v-icon>
          <div>
            <strong>{{ file.name || file.local.split(/[\\/]/).pop() }}</strong
            ><small>{{ file.local || "尚未选择文件" }}</small>
          </div>
        </div>
        <div v-if="item.files.length > 3" class="resource-meta">
          还有 {{ item.files.length - 3 }} 项，点击名称查看
        </div>
      </template>
      <template #footer="{ item }">
        <span class="resource-total"
          >{{
            item.files.filter((file) => file.kind !== "folder").length
          }}
          个文件 ·
          {{
            item.files.filter((file) => file.kind === "folder").length
          }}
          个文件夹</span
        >
        <v-spacer />
        <IconButton
          icon="mdi-pencil-outline"
          label="编辑构建物或更换本地位置"
          @click="edit(item)"
        />
        <IconButton
          icon="mdi-trash-can-outline"
          label="删除构建物"
          @click="
            workspace.confirmDelete = {
              kind: 'artifact',
              id: item.id,
              name: item.name,
            }
          "
        />
      </template>
    </ResourceGrid>
    <div v-else class="large-empty">
      <v-icon size="36">mdi-magnify</v-icon>
      <h3>没有匹配的构建物</h3>
      <v-btn @click="search = ''">清除搜索</v-btn>
    </div>
  </template>
  <div v-else class="large-empty">
    <v-icon size="48" color="primary">mdi-package-variant-closed</v-icon>
    <h3>添加一次，供多个工作流引用</h3>
    <p>保存文件位置；文件移动后在这里更新。</p>
    <v-btn color="primary" @click="edit()">添加构建物</v-btn>
  </div>
  <v-dialog
    :model-value="!!draft"
    @update:model-value="draft = null"
    max-width="780"
    scrollable
    :persistent="workspace.busy"
  >
    <v-card v-if="draft" class="artifact-dialog">
      <v-card-title class="artifact-dialog-heading">
        <div class="resource-icon">
          <v-icon size="23">mdi-package-variant-closed</v-icon>
        </div>
        <span>{{ draft.id ? "编辑构建物" : "添加构建物" }}</span>
        <v-spacer />
        <IconButton
          icon="mdi-close"
          label="关闭构建物配置"
          :disabled="workspace.busy"
          @click="draft = null"
        />
      </v-card-title>
      <v-card-text class="artifact-dialog-body">
        <v-alert v-if="error" type="error" variant="tonal">{{ error }}</v-alert>
        <v-text-field
          label="构建物名称"
          v-model="draft.name"
          placeholder="例如：A 服务程序和配置"
        />
        <section>
          <div class="artifact-files-toolbar">
            <div class="artifact-files-title">
              文件与文件夹 <span>{{ draft.files.length }} 项</span>
            </div>
            <FileSelection
              :initial-path="draft.files[0]?.local"
              @add="add"
              @error="error = String($event)"
            />
          </div>
          <div class="artifact-files-list">
            <ArtifactFileRow
              v-for="(file, i) in draft.files"
              :key="file.id"
              :file="file"
              :disabled="workspace.busy"
              @replace="replace(file)"
              @remove="draft.files.splice(i, 1)"
            />
            <div v-if="!draft.files.length" class="artifact-files-empty">
              <v-icon size="28">mdi-folder-plus-outline</v-icon>
              <span>选择要部署的文件或文件夹</span>
            </div>
          </div>
        </section>
      </v-card-text>
      <v-card-actions class="artifact-dialog-actions"
        ><v-spacer /><v-btn :disabled="workspace.busy" @click="draft = null"
          >取消</v-btn
        ><v-btn
          color="primary"
          variant="flat"
          :disabled="workspace.busy"
          @click="save"
          >保存构建物</v-btn
        ></v-card-actions
      >
    </v-card>
  </v-dialog>
</template>
<style scoped>
.artifact-dialog-heading {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 24px;
  border-bottom: 1px solid var(--border);
}
.artifact-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px !important;
}
.artifact-files-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}
.artifact-files-title {
  font-size: 13px;
  font-weight: 600;
}
.artifact-files-title span {
  margin-left: 8px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 400;
}
.artifact-files-list {
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: hidden;
}
.artifact-files-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 32px 16px;
  color: var(--muted);
}
.artifact-dialog-actions {
  padding: 14px 24px;
  border-top: 1px solid var(--border);
}
</style>
