<script setup lang="ts">
import type { Workspace } from "./useWorkspace";
const props = defineProps<{ workspace: Workspace }>();
function configure() {
  const first = props.workspace.missingInputs[0];
  props.workspace.missingInputs = [];
  if (first?.workflowId) {
    const workflow = props.workspace.cfg.workflows.find(
      (w) => w.id === first.workflowId,
    );
    if (workflow) props.workspace.openWorkflow(workflow);
  } else props.workspace.view = "artifacts";
}
</script>
<template>
  <v-dialog
    :model-value="!!workspace.missingInputs.length"
    @update:model-value="workspace.missingInputs = []"
    max-width="740"
  >
    <v-card title="构建物不可用，未启动运行">
      <v-card-text
        ><div
          v-for="input in workspace.missingInputs"
          :key="input.workflowId + input.artifactId + input.fileId"
          class="missing-artifact"
        >
          <strong>{{ input.workflowName }} / {{ input.name }}</strong>
          <div class="code">{{ input.local || "未配置本地位置" }}</div>
          <small class="text-error">{{ input.error }}</small>
        </div></v-card-text
      >
      <v-card-actions
        ><v-spacer /><v-btn @click="workspace.missingInputs = []">关闭</v-btn
        ><v-btn color="primary" variant="flat" @click="configure">{{
          workspace.missingInputs[0]?.workflowId
            ? "前往工作流配置"
            : "前往构建物管理"
        }}</v-btn></v-card-actions
      >
    </v-card>
  </v-dialog>
</template>
