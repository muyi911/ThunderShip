<script setup lang="ts">
import type { Step, ArtifactResource } from "./types";
import CommandEditor from "./CommandEditor.vue";
import UploadEditor from "./UploadEditor.vue";
const props = defineProps<{
  step: Step;
  index: number;
  last: boolean;
  graph?: boolean;
  artifacts: ArtifactResource[];
}>();
const emit = defineEmits<{
  remove: [];
  move: [number];
  error: [unknown];
  manageArtifacts: [];
}>();
</script>
<template>
  <article class="step-card">
    <header class="step-header">
      <div class="step-type" :class="step.type">
        <v-icon size="18">{{
          step.type === "upload"
            ? "mdi-upload-outline"
            : step.type === "health"
              ? "mdi-heart-pulse"
              : "mdi-console-line"
        }}</v-icon>
      </div>
      <v-text-field
        aria-label="步骤名称"
        v-model="step.name"
        variant="plain"
        class="step-title"
      /><span class="step-badge">{{
        step.type === "upload"
          ? "文件发布"
          : step.type === "health"
            ? "健康检查"
            : "远程命令"
      }}</span
      ><v-btn
        size="x-small"
        icon="mdi-arrow-up"
        v-if="!graph"
        title="上移步骤"
        :disabled="index === 0"
        aria-label="上移步骤"
        @click="emit('move', -1)"
      /><v-btn
        size="x-small"
        icon="mdi-arrow-down"
        v-if="!graph"
        title="下移步骤"
        :disabled="last"
        aria-label="下移步骤"
        @click="emit('move', 1)"
      /><v-btn
        size="x-small"
        icon="mdi-close"
        aria-label="删除步骤"
        title="删除节点"
        @click="emit('remove')"
      />
    </header>
    <div class="step-body">
      <UploadEditor
        v-if="step.type === 'upload'"
        :step="step"
        :artifacts="artifacts"
        @error="emit('error', $event)"
        @manage="emit('manageArtifacts')"
      />
      <template v-else
        ><CommandEditor :step="step" /><v-text-field
          label="工作目录（可选）"
          :hint="
            step.commandMode === 'lines'
              ? '每条命令从此目录开始，不继承上一条的 cd。'
              : '脚本开始前切换目录，失败则停止。脚本内 cd 仍生效；每个节点使用独立 Shell。'
          "
          persistent-hint
          title="目录切换失败时不执行脚本。此目录只影响当前节点，脚本中的 cd 可以再次切换。"
          v-model="step.directory"
          class="mt-3"
      /></template>
      <div class="step-options">
        <v-text-field
          label="超时（秒）"
          type="number"
          min="1"
          max="86400"
          v-model.number="step.timeout"
        /><v-text-field
          v-if="step.type === 'health'"
          label="检查间隔（秒）"
          type="number"
          min="1"
          v-model.number="step.interval"
        /><v-checkbox
          :label="
            step.type === 'upload'
              ? 'sudo 上传'
              : step.commandMode === 'lines'
                ? '每条命令使用 sudo -n'
                : '整段脚本使用 sudo -n'
          "
          v-model="step.sudo"
          :title="
            step.type === 'upload'
              ? '上传使用独立的 sudo SFTP，会话不继承终端中的 sudo；需配置服务器非交互提权和 sftp-server 路径'
              : '可不勾选并在脚本中自行写 sudo。自动工作流使用 sudo -n，不会提示输入密码。'
          "
        />
      </div>
    </div>
  </article>
</template>
