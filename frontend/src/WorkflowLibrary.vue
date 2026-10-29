<script setup lang="ts">
import { computed } from "vue";
import type { Workspace } from "./useWorkspace";
import type { Workflow, Combination } from "./types";
import IconButton from "./IconButton.vue";
import { workflowServers } from "./flow";
import ResourceGrid from "./ResourceGrid.vue";
import { orderedResources } from "./resourceOrder";
const props = defineProps<{ workspace: Workspace }>();
const entries = computed(() =>
  orderedResources(
    [
      ...props.workspace.cfg.workflows.map((w) => ({
        id: w.id,
        name: w.name,
        kind: "部署",
        icon: "mdi-file-tree-outline",
        targets: `${workflowServers(w).length} 台服务器`,
        count: `${w.steps.length} 个节点`,
        resource: w,
      })),
      ...props.workspace.cfg.combinations.map((w) => ({
        id: w.id,
        name: w.name,
        kind: "编排",
        icon: "mdi-source-branch",
        targets: "引用已有工作流",
        count: `${w.root.graph?.nodes.length || w.root.children.length} 个引用`,
        resource: w,
      })),
    ],
    props.workspace.cfg.resourceOrder?.workflows,
  ).filter((w) =>
    w.name.toLowerCase().includes(props.workspace.search.toLowerCase()),
  ),
);
function open(item: (typeof entries.value)[number]) {
  if (item.kind === "部署")
    props.workspace.openWorkflow(item.resource as Workflow);
  else props.workspace.openCombination(item.resource as Combination);
}
function run(id: string, name: string) {
  void props.workspace.execute(
    false,
    { mode: "", workflowId: id, children: [] },
    name,
  );
}
</script>
<template>
  <div class="page-heading">
    <h1>工作流</h1>
    <div class="heading-actions">
      <v-btn
        prepend-icon="mdi-import"
        variant="outlined"
        :disabled="workspace.busy || !!workspace.fatal"
        @click="workspace.openWorkflowImport()"
        title="导入工作流文件，并重新关联本机服务器和构建物"
        >导入工作流</v-btn
      >
      <v-btn
        prepend-icon="mdi-plus"
        color="primary"
        variant="flat"
        @click="workspace.newWorkflow()"
        title="从空画布创建部署工作流"
        >新建工作流</v-btn
      ><v-btn
        prepend-icon="mdi-source-branch"
        variant="outlined"
        @click="workspace.newCombination()"
        title="通过引用现有工作流编排执行依赖"
        >编排工作流</v-btn
      >
    </div>
  </div>
  <div class="library-toolbar">
    <v-btn
      v-if="workspace.wf || workspace.cmb"
      size="small"
      prepend-icon="mdi-file-edit-outline"
      @click="workspace.resumeDraft(workspace.editorKind)"
      title="返回上次编辑的草稿"
      >继续编辑</v-btn
    >
    <v-text-field
      v-model="workspace.search"
      prepend-inner-icon="mdi-magnify"
      placeholder="搜索工作流"
      hide-details
      clearable
      @click:clear="workspace.search = ''"
    /><template v-if="workspace.selected.length"
      ><span>已选 {{ workspace.selected.length }} 项</span
      ><v-select
        v-model="workspace.quickMode"
        :items="[
          { title: '串行', value: 'serial' },
          { title: '并行', value: 'parallel' },
        ]"
        hide-details
      /><v-btn
        color="primary"
        :disabled="workspace.busy"
        @click="workspace.quickRun()"
        title="运行选中的已保存工作流"
        >一起运行</v-btn
      ></template
    >
  </div>
  <ResourceGrid
    v-if="entries.length"
    :items="entries"
    icon="mdi-file-tree-outline"
    :busy="workspace.busy"
    @open="open"
    @move="(id, before) => workspace.moveResource('workflows', id, before)"
  >
    <template #selection="{ item }"
      ><v-checkbox
        v-model="workspace.selected"
        :value="item.id"
        :aria-label="'选择' + item.name"
        class="card-checkbox"
    /></template>
    <template #default="{ item }">
      <div class="resource-meta">
        <v-icon size="16">mdi-server-network</v-icon>{{ item.targets }}
      </div>
      <div class="resource-meta">
        <v-icon size="16">mdi-vector-polyline</v-icon>{{ item.count }}
      </div>
    </template>
    <template #footer="{ item }">
      <v-chip size="x-small" variant="tonal">{{ item.kind }}</v-chip>
      <v-spacer />
      <IconButton
        icon="mdi-play-outline"
        label="运行已保存的工作流"
        color="primary"
        :disabled="workspace.busy || !!workspace.fatal"
        @click="run(item.id, item.name)"
      />
      <IconButton
        icon="mdi-pencil-outline"
        label="编辑流程图"
        @click="open(item)"
      />
      <IconButton
        icon="mdi-content-copy"
        label="复制工作流"
        :disabled="workspace.busy || !!workspace.fatal"
        @click="workspace.duplicateWorkflow(item.resource)"
      />
      <IconButton
        icon="mdi-export"
        label="导出工作流及引用的子工作流"
        :disabled="workspace.busy || !!workspace.fatal"
        @click="workspace.exportWorkflow(item.id)"
      />
      <IconButton
        icon="mdi-trash-can-outline"
        label="删除工作流"
        @click="
          workspace.confirmDelete = {
            kind: item.kind === '部署' ? 'workflow' : 'combination',
            id: item.id,
            name: item.name,
          }
        "
      />
    </template>
  </ResourceGrid>
  <div v-else class="large-empty">
    <v-icon size="48" color="primary">mdi-file-tree-outline</v-icon>
    <h3>{{ workspace.search ? "没有匹配的工作流" : "还没有工作流" }}</h3>
    <v-btn
      v-if="!workspace.search"
      color="primary"
      variant="flat"
      prepend-icon="mdi-plus"
      @click="workspace.newWorkflow()"
      >创建工作流</v-btn
    >
  </div>
</template>
