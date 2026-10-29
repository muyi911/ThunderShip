<script setup lang="ts">
import { computed, ref } from "vue";
import {
  VueFlow,
  Handle,
  Position,
  Panel,
  MarkerType,
  useVueFlow,
  type Connection,
  type NodeDragEvent,
  type GraphNode,
  type GraphEdge,
} from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import IconButton from "./IconButton.vue";
import type { Flow } from "./types";
import { canConnect, canDeleteFrom, START_REF } from "./flow";
import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
const { id, zoomIn, zoomOut, fitView, getSelectedNodes, getSelectedEdges } =
  useVueFlow();
const canvas = ref<HTMLElement | null>(null);
let fitted = false;
function fitInitial() {
  if (!fitted && props.flow.nodes.length) {
    fitted = true;
    void fitView({ maxZoom: 1.2, padding: 0.2 });
  }
}
const props = defineProps<{
  flow: Flow;
  items: Record<string, { name: string; icon: string; detail: string }>;
  selected: string;
}>();
const emit = defineEmits<{
  select: [string];
  remove: [string];
  error: [unknown];
}>();
const nodes = computed(() =>
  props.flow.nodes.map((n) => ({
    id: n.id,
    type: n.ref === START_REF ? "start" : "task",
    deletable: n.ref !== START_REF,
    position: { x: n.x, y: n.y },
    data: props.items[n.ref] || {
      name: "请选择工作流",
      icon: "mdi-file-tree-outline",
      detail: "未配置",
    },
    selected: n.id === props.selected,
  })),
);
const edges = computed(() =>
  props.flow.edges.map((e) => ({
    ...e,
    id: e.source + ":" + e.target,
    type: "smoothstep",
    markerEnd: {
      type: MarkerType.ArrowClosed,
      width: 24,
      height: 24,
      color: "#82b1ff",
    },
    style: { stroke: "#82b1ff", strokeWidth: 2 },
  })),
);
function connect(c: Connection) {
  if (!c.source || !c.target) return;
  if (!canConnect(props.flow, c.source, c.target)) {
    emit("error", "不能创建重复连线或循环依赖");
    return;
  }
  props.flow.edges.push({ source: c.source, target: c.target });
}
function move({ node }: NodeDragEvent) {
  const n = props.flow.nodes.find((n) => n.id === node.id);
  if (n) {
    n.x = node.position.x;
    n.y = node.position.y;
  }
}
function removeNodes(nodes: GraphNode[]) {
  for (const n of nodes) if (n.type !== "start") emit("remove", n.id);
}
function selectNode(id: string) {
  emit("select", id);
  canvas.value?.focus({ preventScroll: true });
}
function deleteSelection(event: KeyboardEvent) {
  if (
    event.isComposing ||
    !["Delete", "Backspace"].includes(event.key) ||
    !canDeleteFrom(event.target)
  )
    return;
  event.preventDefault();
  event.stopPropagation();
  const selected = getSelectedNodes.value;
  if (selected.length) removeNodes(selected);
  else if (
    props.selected &&
    props.flow.nodes.find((n) => n.id === props.selected)?.ref !== START_REF
  )
    emit("remove", props.selected);
  removeEdges(getSelectedEdges.value);
}
function removeEdges(edges: GraphEdge[]) {
  props.flow.edges = props.flow.edges.filter(
    (e) => !edges.some((v) => v.source === e.source && v.target === e.target),
  );
}
</script>
<template>
  <div
    ref="canvas"
    class="flow-canvas"
    tabindex="0"
    aria-label="工作流画布，选中区块后按 Del 删除"
    @keydown="deleteSelection"
  >
    <VueFlow
      :id="id"
      :nodes="nodes"
      :edges="edges"
      :min-zoom="0.25"
      :max-zoom="2"
      @nodes-initialized="fitInitial"
      :snap-to-grid="true"
      :snap-grid="[10, 10]"
      :delete-key-code="[]"
      @connect="connect"
      @node-drag-stop="move"
      @node-click="selectNode($event.node.id)"
      @edge-click="canvas?.focus({ preventScroll: true })"
      @pane-click="emit('select', '')"
      @nodes-delete="removeNodes"
      @edges-delete="removeEdges"
      @edge-double-click="removeEdges([$event.edge])"
    >
      <Background :gap="20" :size="1" pattern-color="#78808d55" />
      <Panel position="bottom-left" class="canvas-controls">
        <IconButton icon="mdi-plus" label="放大画布" @click="zoomIn()" />
        <IconButton icon="mdi-minus" label="缩小画布" @click="zoomOut()" />
        <IconButton
          icon="mdi-fit-to-screen-outline"
          label="显示完整流程图"
          @click="fitView({ maxZoom: 1.2 })"
        />
      </Panel>
      <template #node-start>
        <div class="flow-node-content">
          <div class="flow-node-icon"><v-icon size="24">mdi-play</v-icon></div>
          <div><strong>开始</strong><small>从这里按连线执行</small></div>
        </div>
        <Handle
          type="source"
          :position="Position.Bottom"
          title="输出：可连接多个后续区块"
          :connectable="true"
        /><span class="flow-port-label output">输出 ＋</span>
      </template>
      <template #node-task="{ data }"
        ><Handle
          type="target"
          :position="Position.Top"
          title="输入：等待所有前序完成"
          :connectable="true"
        /><span class="flow-port-label input">输入</span>
        <div class="flow-node-content">
          <div class="flow-node-icon">
            <v-icon size="24">{{ data.icon }}</v-icon>
          </div>
          <div>
            <strong>{{ data.name }}</strong
            ><small>{{ data.detail }}</small>
          </div>
        </div>
        <Handle
          type="source"
          :position="Position.Bottom"
          title="输出：可连接多个后续区块"
          :connectable="true"
        /><span class="flow-port-label output">输出 ＋</span></template
      >
    </VueFlow>
    <div v-if="!flow.nodes.length" class="canvas-empty">
      <v-icon size="42" color="primary">mdi-vector-polyline-plus</v-icon>
      <h3>添加第一个节点</h3>
      <span>从上方选择组件，拖动节点圆点连接执行顺序</span>
    </div>
  </div>
</template>
