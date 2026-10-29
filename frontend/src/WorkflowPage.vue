<script setup lang="ts">
import { computed } from "vue";
import FlowEditor from "./FlowEditor.vue";
import DraftIssues from "./DraftIssues.vue";
import IconButton from "./IconButton.vue";
import type { Workspace } from "./useWorkspace";
const props = defineProps<{ workspace: Workspace }>();
const deployment = computed(() => props.workspace.editorKind === "workflow");
const name = computed({
  get: () =>
    deployment.value
      ? props.workspace.wf?.name || ""
      : props.workspace.cmb?.name || "",
  set: (v) => {
    if (deployment.value && props.workspace.wf) props.workspace.wf.name = v;
    else if (props.workspace.cmb) props.workspace.cmb.name = v;
  },
});
const id = computed(() =>
  deployment.value ? props.workspace.wf?.id : props.workspace.cmb?.id,
);
const flow = computed(() =>
  deployment.value ? props.workspace.wf?.flow : props.workspace.cmb?.root.graph,
);
</script>
<template>
  <div class="page-heading editor-heading">
    <div class="flex-row">
      <IconButton
        icon="mdi-arrow-left"
        label="返回工作流列表，保留当前草稿"
        @click="workspace.view = 'workflows'"
      /><v-text-field
        v-model="name"
        label="工作流名称"
        placeholder="请输入工作流名称"
        prepend-inner-icon="mdi-pencil-outline"
        variant="outlined"
        aria-label="工作流名称"
        title="修改名称后点击保存"
        :disabled="workspace.busy"
        hide-details
      />
    </div>
    <div class="heading-actions">
      <v-btn
        title="检查连接和文件映射，不执行脚本"
        :disabled="
          workspace.busy || workspace.dirty || !id || !!workspace.fatal
        "
        @click="workspace.execute(true)"
        >预检查</v-btn
      ><v-btn
        prepend-icon="mdi-content-save-outline"
        variant="outlined"
        :disabled="
          workspace.busy || !!workspace.unresolved.length || !!workspace.fatal
        "
        @click="
          deployment ? workspace.saveWorkflow() : workspace.saveCombination()
        "
        >保存</v-btn
      ><v-btn
        color="primary"
        variant="flat"
        prepend-icon="mdi-play"
        :disabled="
          workspace.busy ||
          workspace.dirty ||
          !id ||
          !!workspace.unresolved.length ||
          !!workspace.fatal
        "
        @click="workspace.execute(false, workspace.root, name)"
        title="按流程图依赖运行已保存的工作流"
        >运行</v-btn
      >
    </div>
  </div>
  <DraftIssues
    :items="workspace.unresolved"
    @resolve="workspace.unresolved.splice($event, 1)"
  />
  <FlowEditor
    v-if="flow"
    :key="workspace.editorKind + ':' + id"
    :flow="flow"
    :workflow="deployment ? workspace.wf : null"
    :workspace="workspace"
  />
</template>
