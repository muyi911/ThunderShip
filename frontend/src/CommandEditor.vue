<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { Step } from "./types";
import IconButton from "./IconButton.vue";
import CodeEditorDialog from "./CodeEditorDialog.vue";
const props = defineProps<{ step: Step }>();
const mode = computed({
  get: () =>
    props.step.type === "command" && props.step.commandMode === "lines"
      ? "lines"
      : "script",
  set: (value: string) => {
    props.step.commandMode = value;
    if (value === "lines" && !props.step.commands?.length)
      props.step.commands = [""];
  },
});
const commands = computed(() => props.step.commands || []);
const editing = ref<number | "script" | null>(null);
watch(
  () => props.step.id,
  () => {
    editing.value = null;
  },
);
const editorText = computed({
  get: () =>
    typeof editing.value === "number"
      ? commands.value[editing.value] || ""
      : props.step.command,
  set: (value: string) => {
    if (typeof editing.value === "number")
      commands.value[editing.value] = value;
    else if (editing.value === "script") props.step.command = value;
  },
});
function addLine() {
  (props.step.commands ||= []).push("");
}
function move(index: number, delta: number) {
  const rows = commands.value;
  [rows[index], rows[index + delta]] = [rows[index + delta], rows[index]];
}
</script>
<template>
  <div class="command-editor">
    <v-btn-toggle
      v-if="step.type === 'command'"
      v-model="mode"
      mandatory
      divided
      color="primary"
      density="compact"
      class="mb-3"
    >
      <v-btn
        value="lines"
        size="small"
        prepend-icon="mdi-format-list-numbered"
        title="按顺序执行每条命令；失败停止，每条使用独立 Shell"
        >命令行</v-btn
      >
      <v-btn
        value="script"
        size="small"
        prepend-icon="mdi-script-text-outline"
        title="整段脚本共享目录、变量和 Shell 状态；切换模式会保留原内容"
        >脚本</v-btn
      >
    </v-btn-toggle>
    <template v-if="mode === 'lines'">
      <p class="command-hint">
        按顺序执行，失败停止。每条使用独立 Shell；需要共享变量或连续 cd
        时请用脚本。
      </p>
      <div v-for="(_, i) in commands" :key="i" class="command-row">
        <header>
          <span>命令 {{ i + 1 }}</span
          ><span class="spacer" />
          <IconButton
            icon="mdi-arrow-expand"
            :label="'在大窗口中编辑命令 ' + (i + 1)"
            size="x-small"
            @click="editing = i"
          />
          <IconButton
            icon="mdi-arrow-up"
            :label="'上移第 ' + (i + 1) + ' 条命令'"
            size="x-small"
            :disabled="i === 0"
            @click="move(i, -1)"
          />
          <IconButton
            icon="mdi-arrow-down"
            :label="'下移第 ' + (i + 1) + ' 条命令'"
            size="x-small"
            :disabled="i === commands.length - 1"
            @click="move(i, 1)"
          />
          <IconButton
            icon="mdi-close"
            :label="'删除第 ' + (i + 1) + ' 条命令'"
            size="x-small"
            @click="commands.splice(i, 1)"
          />
        </header>
        <v-text-field
          v-model="commands[i]"
          :aria-label="'第 ' + (i + 1) + ' 条命令'"
          placeholder="例如 sudo systemctl restart service-a"
          class="code-input"
          hide-details
        />
      </div>
      <v-btn
        size="small"
        prepend-icon="mdi-plus"
        :disabled="commands.length >= 256"
        @click="addLine"
        title="在末尾添加一条命令"
        >添加命令行</v-btn
      >
    </template>
    <template v-else>
      <div class="script-actions">
        <v-btn
          size="small"
          prepend-icon="mdi-arrow-expand"
          title="在大窗口中编辑完整脚本，支持全屏与横向滚动"
          @click="editing = 'script'"
          >弹出编辑器</v-btn
        >
      </div>
      <v-textarea
        variant="outlined"
        v-model="step.command"
        :rows="step.type === 'health' ? 2 : 5"
        :label="
          step.type === 'health' ? '检查脚本 · 退出码 0 视为健康' : 'Shell 脚本'
        "
        :placeholder="
          step.type === 'health'
            ? 'curl --fail --silent http://127.0.0.1:8080/health'
            : 'set -e\n# 在这里编写多行 Shell 脚本'
        "
        class="code-input"
        wrap="off"
        spellcheck="false"
      />
      <p v-if="step.type === 'command'" class="command-hint">
        整段脚本共享上下文；如需失败停止，请使用 set -e 或 &amp;&amp;。
      </p>
    </template>
    <CodeEditorDialog
      :open="editing !== null"
      @update:open="editing = $event ? editing : null"
      v-model="editorText"
      :title="
        typeof editing === 'number'
          ? '编辑命令 ' + (editing + 1)
          : '编辑 Shell 脚本'
      "
    />
  </div>
</template>
<style scoped>
.script-actions {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 6px;
}
.code-input :deep(textarea) {
  white-space: pre;
  overflow-wrap: normal;
  overflow-x: auto;
}
.command-row {
  margin-bottom: 12px;
}
.command-row > header {
  display: flex;
  align-items: center;
  gap: 2px;
  margin-bottom: 5px;
  color: var(--muted);
  font-size: 11px;
}
.command-hint {
  color: var(--muted);
  font-size: 11px;
  line-height: 1.6;
  margin: 8px 0 12px;
}
</style>
