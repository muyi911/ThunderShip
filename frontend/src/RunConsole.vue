<script setup lang="ts">
import { toRefs } from "vue";
import type { Workspace } from "./useWorkspace";
import ResizablePanel from "./ResizablePanel.vue";
const props = defineProps<{ workspace: Workspace }>();
const {
  labels,
  cfg,
  active,
  statusColor,
  error,
  logsOpen,
  activeRun,
  events,
  api,
} = toRefs(props.workspace);
</script>
<template>
  <ResizablePanel v-if="logsOpen" class="execution-panel" label="执行日志" axis="height"
    storage-key="thundership.execution-log-height" :default-size="240" :min-size="120" :max-size="1200" :max-ratio="0.8">
    <header>
      <v-icon size="15">mdi-console</v-icon><strong>执行日志</strong
      ><span class="muted">{{ active?.name }}</span
      ><v-chip
        v-if="active"
        size="x-small"
        :color="statusColor(active.status)"
        >{{ labels[active.status] }}</v-chip
      ><span class="spacer" /><v-btn
        v-if="active?.status === 'running'"
        size="x-small"
        color="error"
        @click="api('CancelRun', activeRun).catch(error)"
        >停止</v-btn
      ><v-btn
        size="x-small"
        icon="mdi-close"
        aria-label="关闭日志"
        @click="logsOpen = false"
      />
    </header>
    <div class="log-lines">
      <div
        v-for="ev in events"
        :key="ev.rowId"
        :class="['log-line', ev.status]"
      >
        <span class="log-time">{{
          new Date(ev.time).toLocaleTimeString()
        }}</span
        ><span class="log-host">{{
          cfg.servers.find((s) => s.id === ev.server)?.name || "系统"
        }}</span
        ><span class="log-state">{{ labels[ev.status] || ev.status }}</span
        ><span class="log-detail"
          >{{ ev.file }}
          <span v-if="ev.status === 'uploading'" class="log-upload-progress">
            <v-progress-linear
              :model-value="
                ev.total ? Math.min(100, (ev.current / ev.total) * 100) : 0
              "
              :indeterminate="!ev.total"
              color="primary"
              height="3"
              rounded
              aria-label="文件上传进度"
            />
            <span class="log-upload-percent">{{
              ev.total
                ? Math.min(100, Math.round((ev.current / ev.total) * 100)) + "%"
                : "上传中"
            }}</span>
          </span>
          {{ ev.message }}</span
        >
      </div>
      <div v-if="!events.length" class="muted">等待执行事件…</div>
    </div>
  </ResizablePanel>
</template>
