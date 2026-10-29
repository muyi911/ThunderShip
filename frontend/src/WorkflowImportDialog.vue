<script setup lang="ts">
import type { Workspace } from "./useWorkspace";
const props = defineProps<{ workspace: Workspace }>();
function localFiles(assetID: string, kind: string | undefined) {
  const id = props.workspace.importBindings.artifacts[assetID]?.id;
  return (
    props.workspace.cfg.artifacts
      .find((a) => a.id === id)
      ?.files.filter((f) => f.kind === kind) || []
  );
}
</script>

<template>
  <v-dialog
    :model-value="!!workspace.importPackage"
    @update:model-value="workspace.importPackage = null"
    :persistent="workspace.busy"
    max-width="720"
    scrollable
  >
    <v-card v-if="workspace.importPackage" title="导入工作流">
      <v-card-text>
        <p class="mb-5">
          {{
            workspace.importPackage.workflows.length +
            workspace.importPackage.combinations.length
          }}
          个工作流，另存为新工作流。请重新关联本机资源，远端目录和命令将保留。
        </p>
        <div class="d-flex flex-wrap ga-2 mb-5">
          <v-chip
            v-for="flow in [
              ...workspace.importPackage.workflows,
              ...workspace.importPackage.combinations,
            ]"
            :key="flow.id"
            size="small"
            variant="tonal"
            >{{ flow.name }}</v-chip
          >
        </div>
        <v-alert
          v-if="
            (workspace.importPackage.servers.length &&
              !workspace.cfg.servers.length) ||
            (workspace.importPackage.artifacts.length &&
              !workspace.cfg.artifacts.length)
          "
          type="info"
          variant="tonal"
          class="mb-4"
          >请先在服务器、构建物页面创建所需资源，再回来导入。</v-alert
        >
        <h4 v-if="workspace.importPackage.servers.length" class="mb-3">
          目标服务器
        </h4>
        <v-select
          v-for="server in workspace.importPackage.servers"
          :key="server.id"
          v-model="workspace.importBindings.servers[server.id]"
          :label="server.name + ' → 本机服务器'"
          :items="
            workspace.cfg.servers.map((s) => ({
              title: `${s.name} (${s.user}@${s.host}:${s.port})`,
              value: s.id,
            }))
          "
          :disabled="workspace.busy"
          class="mb-3"
        />
        <h4 v-if="workspace.importPackage.artifacts.length" class="mb-3">
          构建物与文件对应关系
        </h4>
        <v-sheet
          v-for="asset in workspace.importPackage.artifacts"
          :key="asset.id"
          border
          rounded
          class="pa-4 mb-4"
        >
          <v-select
            :model-value="workspace.importBindings.artifacts[asset.id]?.id"
            @update:model-value="workspace.bindImportArtifact(asset.id, $event)"
            :label="asset.name + ' → 本机构建物'"
            :items="workspace.cfg.artifacts"
            item-title="name"
            item-value="id"
            :disabled="workspace.busy"
            class="mb-3"
          />
          <v-select
            v-for="file in asset.files"
            :key="file.id"
            v-model="
              workspace.importBindings.artifacts[asset.id]!.files[file.id]
            "
            :label="`${file.name}（${file.kind === 'folder' ? '文件夹' : '文件'}）`"
            :items="localFiles(asset.id, file.kind)"
            item-title="name"
            item-value="id"
            :disabled="
              workspace.busy ||
              !workspace.importBindings.artifacts[asset.id]?.id
            "
            class="mb-2"
          />
        </v-sheet>
        <p class="text-medium-emphasis">
          导入仅保存配置，不会执行工作流，也不会覆盖已有工作流。
        </p>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          :disabled="workspace.busy"
          @click="workspace.importPackage = null"
          >取消</v-btn
        >
        <v-btn
          color="primary"
          variant="flat"
          :disabled="workspace.busy || !workspace.importReady"
          @click="workspace.finishWorkflowImport()"
          >确认导入</v-btn
        >
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
