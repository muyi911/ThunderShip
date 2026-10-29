<script setup lang="ts">
import { computed, ref } from "vue";
import type { Flow, Workflow } from "./types";
import FlowCanvas from "./FlowCanvas.vue";
import StepEditor from "./StepEditor.vue";
import ServerStepEditor from "./ServerStepEditor.vue";
import ArtifactPage from "./ArtifactPage.vue";
import IconButton from "./IconButton.vue";
import ResizablePanel from "./ResizablePanel.vue";
import { uid } from "./api";
import type { Workspace } from "./useWorkspace";
import { START_REF } from "./flow";
const props = defineProps<{
  flow: Flow;
  workflow?: Workflow | null;
  workspace: Workspace;
}>();
const selected = ref("");
const artifactsOpen = ref(false);
const kinds = [
  { type: "server", name: "服务器", icon: "mdi-server-network" },
  {
    type: "upload",
    name: "上传文件",
    icon: "mdi-cloud-upload-outline",
  },
  {
    type: "command",
    name: "执行命令",
    icon: "mdi-console-line",
  },
  {
    type: "workflow",
    name: "运行工作流",
    icon: "mdi-file-tree-outline",
  },
];
const references = computed(() =>
  [
    ...props.workspace.cfg.workflows,
    ...props.workspace.cfg.combinations,
  ].filter((w) => w.id !== (props.workflow?.id || props.workspace.cmb?.id)),
);
const node = computed(() =>
  props.flow.nodes.find((n) => n.id === selected.value),
);
const step = computed(() =>
  props.workflow?.steps.find((s) => s.id === node.value?.ref),
);
const items = computed(() =>
  Object.fromEntries(
    props.workflow
      ? props.workflow.steps.map((s) => [
          s.id,
          {
            name: s.name,
            icon:
              kinds.find((k) => k.type === s.type)?.icon || "mdi-console-line",
            detail:
              s.type === "workflow"
                ? references.value.find((w) => w.id === s.workflowId)?.name ||
                  "请选择工作流"
                : s.type === "server"
                  ? s.targets?.servers
                      .map(
                        (id) =>
                          props.workspace.cfg.servers.find(
                            (server) => server.id === id,
                          )?.name || id,
                      )
                      .join("、") || "请选择服务器"
                  : s.type === "upload"
                    ? `${s.files.length} 个文件映射`
                    : s.type === "command" && s.commandMode === "lines"
                      ? `${s.commands?.length || 0} 条命令 · 失败停止`
                      : s.command.split("\n")[0] || "待配置",
          },
        ])
      : references.value.map((w) => [
          w.id,
          { name: w.name, icon: "mdi-file-tree-outline", detail: "引用工作流" },
        ]),
  ),
);
function add(type: string) {
  const id = uid();
  let ref = "";
  if (props.workflow) {
    const s = props.workspace.newStep(type);
    props.workflow.steps.push(s);
    ref = s.id;
  } else ref = references.value[0]?.id || "";
  const parent =
    props.flow.nodes.find((n) => n.id === selected.value) ||
    props.flow.nodes.find((n) => n.ref === START_REF);
  props.flow.nodes.push({
    id,
    ref,
    x:
      (parent?.x ?? 90) +
      props.flow.edges.filter((e) => e.source === parent?.id).length * 300,
    y: parent ? parent.y + 150 : 70 + props.flow.nodes.length * 130,
  });
  if (parent) props.flow.edges.push({ source: parent.id, target: id });
  selected.value = id;
}
function remove(id: string) {
  const n = props.flow.nodes.find((n) => n.id === id);
  if (n?.ref === START_REF) return;
  if (props.workflow && n)
    props.workflow.steps = props.workflow.steps.filter((s) => s.id !== n.ref);
  props.flow.nodes = props.flow.nodes.filter((n) => n.id !== id);
  props.flow.edges = props.flow.edges.filter(
    (e) => e.source !== id && e.target !== id,
  );
  if (selected.value === id) selected.value = "";
}
</script>
<template>
  <section class="flow-editor">
    <div class="flow-toolbar">
      <template v-if="workflow"
        ><v-btn
          v-for="kind in kinds"
          :key="kind.type"
          :prepend-icon="kind.icon"
          size="small"
          :title="
            kind.type === 'command'
              ? '添加执行命令节点，可切换命令行或脚本模式'
              : '添加' + kind.name + '节点'
          "
          @click="add(kind.type)"
          >{{ kind.name }}</v-btn
        ></template
      ><v-btn
        v-else
        size="small"
        prepend-icon="mdi-file-tree-outline"
        title="添加已有工作流引用，不复制步骤"
        @click="add('reference')"
        >引用工作流</v-btn
      ><span class="spacer" /><IconButton
        icon="mdi-help-circle-outline"
        label="拖动圆点连线；多条入线表示等待所有前序成功。无依赖节点可并行，仍受并发限制。双击连线可删除；也可选中节点或连线后按 Delete 删除。"
      />
    </div>
    <div class="flow-editor-body">
      <FlowCanvas
        :flow="flow"
        :items="items"
        :selected="selected"
        @select="selected = $event"
        @remove="remove"
        @error="workspace.error"
      />
      <ResizablePanel
        v-if="node && node.ref !== START_REF"
        class="node-inspector"
        label="节点配置栏"
        storage-key="thundership.node-inspector-width"
        :default-size="350"
      >
        <header>
          <strong>节点配置</strong><span class="spacer" /><IconButton
            icon="mdi-trash-can-outline"
            label="删除节点及其连线"
            @click="remove(node.id)"
          /><IconButton
            icon="mdi-close"
            label="收起节点配置"
            @click="selected = ''"
          />
        </header>
        <div
          v-if="step?.type === 'workflow'"
          class="pa-4 d-flex flex-column ga-4"
        >
          <v-text-field label="区块名称" v-model="step.name" />
          <v-select
            label="运行工作流"
            :items="references"
            item-title="name"
            item-value="id"
            v-model="step.workflowId"
          />
          <p class="muted">
            使用所选工作流自己的服务器与构建物；完成后继续后续区块。
          </p>
        </div>
        <ServerStepEditor
          v-else-if="step?.type === 'server'"
          :step="step"
          :servers="workspace.cfg.servers"
          @manage="workspace.editServer()"
        />
        <StepEditor
          v-else-if="step"
          :step="step"
          :index="0"
          :last="true"
          :graph="true"
          :artifacts="workspace.cfg.artifacts"
          @manage-artifacts="artifactsOpen = true"
          @remove="remove(node.id)"
          @error="workspace.error"
        />
        <div v-else class="pa-4">
          <v-select
            label="引用工作流"
            :items="references"
            item-title="name"
            item-value="id"
            v-model="node.ref"
          />
        </div>
      </ResizablePanel>
    </div>
  </section>
  <v-dialog
    v-model="artifactsOpen"
    max-width="1060"
    scrollable
    :persistent="workspace.busy"
    aria-labelledby="artifact-manager-title"
  >
    <v-card>
      <v-card-title id="artifact-manager-title" class="d-flex align-center">
        管理构建物
        <v-spacer />
        <IconButton
          icon="mdi-close"
          label="关闭构建物管理，继续编辑工作流"
          :disabled="workspace.busy"
          @click="artifactsOpen = false"
        />
      </v-card-title>
      <v-card-text>
        <ArtifactPage :workspace="workspace" embedded />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          color="primary"
          variant="flat"
          :disabled="workspace.busy"
          @click="artifactsOpen = false"
          >返回工作流</v-btn
        >
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
