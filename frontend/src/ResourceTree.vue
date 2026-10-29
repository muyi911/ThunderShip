<script setup lang="ts">
import { computed } from "vue";
import type { Workspace } from "./useWorkspace";
import IconButton from "./IconButton.vue";
import { orderedResources } from "./resourceOrder";
const props = defineProps<{ workspace: Workspace }>();
const workflows = computed(() =>
  orderedResources(
    [
      ...props.workspace.filteredWorkflows
        .filter((w) => w.id !== props.workspace.wf?.id)
        .map((w) => ({
          ...w,
          kind: "workflow",
          icon: "mdi-file-tree-outline",
        })),
      ...props.workspace.cfg.combinations
        .filter(
          (c) =>
            c.id !== props.workspace.cmb?.id &&
            c.name.toLowerCase().includes(props.workspace.search.toLowerCase()),
        )
        .map((w) => ({ ...w, kind: "reference", icon: "mdi-source-branch" })),
    ],
    props.workspace.cfg.resourceOrder?.workflows,
  ),
);
</script>
<template>
  <aside class="resource-tree">
    <header class="tree-heading">
      <v-icon color="primary" size="21">{{
        workspace.view === "connections"
          ? "mdi-server-network"
          : "mdi-file-tree-outline"
      }}</v-icon
      ><strong>{{
        workspace.view === "connections" ? "服务器连接" : "工作流"
      }}</strong
      ><span class="spacer" /><IconButton
        icon="mdi-plus"
        :label="workspace.view === 'connections' ? '添加服务器' : '新建工作流'"
        size="x-small"
        @click="
          workspace.view === 'connections'
            ? workspace.editServer()
            : workspace.newWorkflow()
        "
      />
    </header>
    <div class="tree-search">
      <v-text-field
        v-model="workspace.search"
        prepend-inner-icon="mdi-magnify"
        placeholder="搜索"
        aria-label="搜索资源"
        variant="solo-filled"
        flat
        hide-details
      />
    </div>
    <div class="tree-scroll">
      <template v-if="workspace.view === 'editor'">
        <button
          v-if="workspace.wf"
          class="tree-row resource-row"
          :class="{ selected: workspace.editorKind === 'workflow' }"
          @click="workspace.resumeDraft('workflow')"
        >
          <span class="resource-icon"
            ><v-icon size="21">mdi-file-edit-outline</v-icon></span
          >{{ workspace.wf.name }} · 当前草稿
        </button>
        <button
          v-if="workspace.cmb"
          class="tree-row resource-row"
          :class="{ selected: workspace.editorKind === 'reference' }"
          @click="workspace.resumeDraft('reference')"
        >
          <span class="resource-icon"
            ><v-icon size="21">mdi-source-branch</v-icon></span
          >{{ workspace.cmb.name }} · 编排草稿
        </button>
      </template>
      <template v-if="workspace.view === 'editor'"
        ><button
          v-for="w in workflows"
          :key="w.id"
          class="tree-row resource-row"
          :class="{
            selected:
              w.id ===
              (workspace.editorKind === 'workflow'
                ? workspace.wf?.id
                : workspace.cmb?.id),
          }"
          @click="
            w.kind === 'workflow'
              ? workspace.openWorkflow(
                  workspace.cfg.workflows.find((v) => v.id === w.id)!,
                )
              : workspace.openCombination(
                  workspace.cfg.combinations.find((v) => v.id === w.id)!,
                )
          "
        >
          <span class="resource-icon"
            ><v-icon size="22">{{ w.icon }}</v-icon></span
          >{{ w.name }}</button
        ><button class="tree-empty" @click="workspace.view = 'workflows'">
          <v-icon size="18">mdi-view-list-outline</v-icon> 全部工作流
        </button></template
      ><template v-else
        ><template v-for="group in workspace.groups" :key="group"
          ><div class="tree-group">
            <v-icon size="16">mdi-folder-outline</v-icon>{{ group }}
          </div>
          <button
            v-for="s in workspace.filteredServers.filter(
              (s) => (s.group || '未分组') === group,
            )"
            :key="s.id"
            class="tree-row resource-row"
            :class="{ selected: s.id === workspace.connectedServer?.id }"
            :title="s.user + '@' + s.host + ' · 双击打开新连接'"
            @dblclick="workspace.connectServer(s)"
            @keydown.enter="workspace.connectServer(s)"
          >
            <span class="resource-icon"
              ><v-icon size="23">mdi-server-network</v-icon></span
            >{{ s.name }}
          </button></template
        ></template
      >
    </div>
  </aside>
</template>
