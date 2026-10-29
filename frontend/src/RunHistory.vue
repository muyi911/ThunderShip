<script setup lang="ts">
import { toRefs } from "vue";
import type { Workspace } from "./useWorkspace";
const props = defineProps<{ workspace: Workspace }>();
const { view, statusColor, selectRun, runs, labels } = toRefs(props.workspace);
</script>
<template>
  <template v-if="view === 'runs'"
    ><div class="page-heading">
      <div>
        <h1>运行记录</h1>
        <p>逐服务器查看步骤、文件发布和健康检查结果。</p>
      </div>
    </div>
    <div v-if="!runs.length" class="large-empty">
      <v-icon size="45">mdi-history</v-icon>
      <h3>还没有运行记录</h3>
      <p>保存工作流并运行后，进度和结果会显示在这里。</p>
    </div>
    <v-table v-else class="data-table"
      ><thead>
        <tr>
          <th>任务</th>
          <th>状态</th>
          <th>开始时间</th>
          <th>结果</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="r in runs"
          :key="r.id"
          class="clickable"
          @click="selectRun(r)"
        >
          <td>{{ r.name }}</td>
          <td>
            <v-chip size="x-small" :color="statusColor(r.status)">{{
              labels[r.status] || r.status
            }}</v-chip>
          </td>
          <td>{{ new Date(r.started).toLocaleString() }}</td>
          <td>{{ r.error || "—" }}</td>
        </tr>
      </tbody></v-table
    ></template
  >
</template>
