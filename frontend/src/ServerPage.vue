<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { Workspace } from "./useWorkspace";
import IconButton from "./IconButton.vue";
import ResourceGrid from "./ResourceGrid.vue";
const props = defineProps<{ workspace: Workspace }>();
const search = ref("");
const group = ref<string | null>(null);
const groups = computed(() => [
  ...new Set(props.workspace.orderedServers.map((s) => s.group.trim())),
]);
const servers = computed(() =>
  props.workspace.orderedServers.filter(
    (s) =>
      (group.value === null || s.group.trim() === group.value) &&
      `${s.name} ${s.host} ${s.user} ${s.group}`
        .toLowerCase()
        .includes(search.value.toLowerCase()),
  ),
);
watch(groups, (values) => {
  if (group.value !== null && !values.includes(group.value)) group.value = null;
});
</script>
<template>
  <div class="page-heading">
    <h1>服务器</h1>
    <v-btn
      color="primary"
      variant="flat"
      prepend-icon="mdi-plus"
      @click="workspace.editServer()"
      >添加服务器</v-btn
    >
  </div>
  <template v-if="workspace.cfg.servers.length">
    <div class="library-toolbar">
      <v-text-field
        v-model="search"
        prepend-inner-icon="mdi-magnify"
        placeholder="搜索名称、地址或分组"
        aria-label="搜索服务器"
        hide-details
        clearable
        @click:clear="search = ''"
      />
      <span class="resource-total">{{ servers.length }} 台服务器</span>
    </div>
    <div class="resource-filters" aria-label="服务器分组">
      <v-chip
        :color="group === null ? 'primary' : undefined"
        :variant="group === null ? 'tonal' : 'outlined'"
        :aria-pressed="group === null"
        @click="group = null"
        >全部
        <span class="filter-count">{{
          workspace.cfg.servers.length
        }}</span></v-chip
      >
      <v-chip
        v-for="name in groups"
        :key="name"
        :color="group === name ? 'primary' : undefined"
        :variant="group === name ? 'tonal' : 'outlined'"
        :aria-pressed="group === name"
        @click="group = name"
      >
        <v-icon start size="15">mdi-folder-outline</v-icon
        >{{ name || "未分组" }}
        <span class="filter-count">{{
          workspace.cfg.servers.filter((s) => s.group.trim() === name).length
        }}</span>
      </v-chip>
    </div>
    <ResourceGrid
      v-if="servers.length"
      :items="servers"
      class="server-resource-grid"
      icon="mdi-server-network"
      double-click
      :busy="workspace.busy"
      @open="workspace.connectServer"
      @move="(id, before) => workspace.moveResource('servers', id, before)"
    >
      <template #default="{ item }">
        <div
          class="resource-address code"
          :title="`${item.user}@${item.host}:${item.port}`"
        >
          {{ item.user }}@{{ item.host }}:{{ item.port }}
        </div>
        <div
          v-if="item.sudoUpload || item.concurrency > 0"
          class="server-badges"
        >
          <span
            v-if="item.sudoUpload"
            class="server-badge"
            :title="'文件上传使用 sudo，以 ' + item.sudoUser + ' 身份执行'"
          >
            <v-icon size="13">mdi-shield-account-outline</v-icon>sudo →
            {{ item.sudoUser }}
          </span>
          <span
            v-if="item.concurrency > 0"
            class="server-badge"
            title="同一服务器同时执行的最大任务数"
          >
            <v-icon size="13">mdi-source-branch</v-icon
            >{{ item.concurrency === 1 ? "串行" : item.concurrency + " 并发" }}
          </span>
        </div>
      </template>
      <template #footer="{ item }">
        <v-btn
          size="small"
          prepend-icon="mdi-folder-outline"
          class="card-group"
          :title="'筛选分组：' + (item.group || '未分组')"
          @click="group = item.group.trim()"
          >{{ item.group || "未分组" }}</v-btn
        >
        <v-spacer />
        <v-btn
          size="small"
          color="primary"
          prepend-icon="mdi-console"
          title="打开 SSH 终端和文件管理"
          @click="workspace.connectServer(item)"
          >连接</v-btn
        >
        <IconButton
          icon="mdi-connection"
          label="测试连接"
          size="x-small"
          :disabled="workspace.busy"
          @click="workspace.probeServer(item)"
        />
        <IconButton
          icon="mdi-pencil-outline"
          label="编辑服务器"
          size="x-small"
          @click="workspace.editServer(item)"
        />
        <IconButton
          icon="mdi-delete-outline"
          label="删除服务器"
          size="x-small"
          @click="
            workspace.confirmDelete = {
              kind: 'server',
              id: item.id,
              name: item.name,
            }
          "
        />
      </template>
    </ResourceGrid>
    <div v-else class="large-empty">
      <v-icon size="36">mdi-magnify</v-icon>
      <h3>没有匹配的服务器</h3>
      <v-btn
        @click="
          search = '';
          group = null;
        "
        >清除筛选</v-btn
      >
    </div>
  </template>
  <div v-else class="large-empty">
    <v-icon size="45">mdi-server-network-outline</v-icon>
    <h3>添加你的第一台服务器</h3>
    <v-btn variant="tonal" color="primary" @click="workspace.editServer()"
      >添加连接</v-btn
    >
  </div>
</template>
<style scoped>
.server-resource-grid :deep(.resource-card-content) {
  min-height: 0;
}
.resource-address {
  margin: 0;
  color: var(--muted);
  font-weight: 400;
}
.server-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 12px;
}
.server-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 7px;
  border-radius: 4px;
  background: var(--soft);
  color: var(--muted);
  font-size: 10px;
}
</style>
