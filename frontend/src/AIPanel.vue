<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from "vue";
import type { Config, Workflow, Combination, Draft } from "./types";
import { api, clone, uid } from "./api";
import { workflowServers } from "./flow";
import ResizablePanel from "./ResizablePanel.vue";
const props = defineProps<{
  config: Config;
  workflow: Workflow | null;
  combination: Combination | null;
  baseState: string;
}>();
const emit = defineEmits<{
  apply: [Draft, string];
  settings: [];
  close: [];
  error: [unknown];
}>();
const providerId = ref(""),
  prompt = ref(""),
  serverIds = ref<string[]>([]),
  artifactIds = ref<string[]>([]),
  workflowIds = ref<string[]>([]),
  includeScripts = ref(false),
  busy = ref(false),
  candidate = ref<Draft | null>(null),
  base = ref(""),
  requestID = ref(""),
  showContext = ref(false);
const provider = computed(
  () =>
    props.config.providers.find((p) => p.id === providerId.value) ||
    props.config.providers[0],
);
const contextCount = computed(
  () =>
    `${serverIds.value.length ? `${serverIds.value.length} 台服务器` : "服务器自动匹配"} · ${artifactIds.value.length ? `${artifactIds.value.length} 个构建物` : "构建物自动匹配"} · ${workflowIds.value.length} 个工作流`,
);
const matchedServers = computed(() => {
  const ids = candidate.value?.workflow
    ? workflowServers(candidate.value.workflow)
    : [];
  return props.config.servers.filter((s) => ids.includes(s.id));
});
const matchedArtifacts = computed(() => {
  const ids = candidate.value?.workflow?.steps.map((s) => s.artifactId) || [];
  return props.config.artifacts.filter((a) => ids.includes(a.id));
});
async function generate() {
  if (!provider.value) {
    emit("settings");
    return;
  }
  busy.value = true;
  candidate.value = null;
  base.value = props.baseState;
  requestID.value = uid();
  const id = requestID.value;
  try {
    const d = await api(
      "GenerateDraft",
      {
        providerId: provider.value.id,
        prompt: prompt.value,
        workflow: props.workflow ? clone(props.workflow) : null,
        combination: props.combination ? clone(props.combination) : null,
        serverIds: serverIds.value,
        artifactIds: artifactIds.value,
        workflowIds: workflowIds.value,
        includeScripts: includeScripts.value,
      },
      id,
    );
    if (requestID.value === id) candidate.value = d;
  } catch (e) {
    if (requestID.value === id) emit("error", e);
  } finally {
    if (requestID.value === id) busy.value = false;
  }
}
async function cancel() {
  const id = requestID.value;
  requestID.value = "";
  busy.value = false;
  try {
    await api("CancelGeneration", id);
  } catch (e) {
    emit("error", e);
  }
}
onBeforeUnmount(() => {
  if (busy.value) {
    const id = requestID.value;
    requestID.value = "";
    void api("CancelGeneration", id).catch(() => {});
  }
});
</script>
<template>
  <ResizablePanel
    class="ai-panel"
    label="AI 编排助手"
    storage-key="thundership.ai-panel-width"
    :default-size="340"
    :max-ratio="0.6"
  >
    <header class="panel-title">
      <div>
        <v-icon size="18" color="primary">mdi-auto-fix</v-icon
        ><strong>AI 编排助手</strong>
      </div>
      <v-btn
        size="x-small"
        icon="mdi-close"
        aria-label="收起 AI 面板"
        @click="emit('close')"
      />
    </header>
    <div class="ai-scroll">
      <div v-if="!config.providers.length" class="ai-welcome">
        <div class="ai-emblem"><v-icon size="28">mdi-auto-fix</v-icon></div>
        <h3>描述需求，生成部署步骤</h3>
        <p>复用你的服务器、文件和工作流，安排上传、重启与执行顺序。</p>
        <v-btn
          variant="tonal"
          color="primary"
          prepend-icon="mdi-cog-outline"
          @click="emit('settings')"
          >配置 AI 服务</v-btn
        >
      </div>
      <template v-else
        ><v-select
          label="AI 提供方"
          :model-value="provider?.id"
          @update:model-value="providerId = $event"
          :items="config.providers"
          item-title="name"
          item-value="id" />
        <div class="model-caption">
          <span class="status-dot" />{{ provider?.model || "尚未选择模型"
          }}<v-btn
            size="x-small"
            icon="mdi-cog-outline"
            aria-label="模型设置"
            @click="emit('settings')"
          /></div
      ></template>
      <div class="ai-section-label">本次上下文</div>
      <button class="context-toggle" @click="showContext = !showContext">
        <v-icon size="15">mdi-database-outline</v-icon
        ><span>{{ contextCount }}</span
        ><v-icon size="15">{{
          showContext ? "mdi-chevron-up" : "mdi-chevron-down"
        }}</v-icon>
      </button>
      <div v-show="showContext" class="context-options">
        <v-select
          multiple
          chips
          closable-chips
          label="限定服务器（留空自动匹配）"
          :items="config.servers"
          item-title="name"
          item-value="id"
          v-model="serverIds"
        /><v-btn
          v-if="workflow && workflowServers(workflow).length"
          size="x-small"
          @click="serverIds = workflowServers(workflow!)"
          >选择当前工作流服务器</v-btn
        ><v-select
          class="mt-3"
          multiple
          chips
          closable-chips
          label="限定构建物（留空自动匹配）"
          :items="config.artifacts"
          item-title="name"
          item-value="id"
          v-model="artifactIds"
        /><v-btn
          v-if="workflow?.steps.some((s) => s.artifactId)"
          size="x-small"
          @click="
            artifactIds = [
              ...new Set(
                workflow!.steps.flatMap((s) =>
                  s.artifactId ? [s.artifactId] : [],
                ),
              ),
            ]
          "
          >选择当前工作流构建物</v-btn
        ><v-select
          class="mt-3"
          multiple
          chips
          closable-chips
          label="可引用的已有工作流"
          :items="[...config.workflows, ...config.combinations]"
          item-title="name"
          item-value="id"
          v-model="workflowIds"
        /><v-checkbox
          label="包含选中工作流的命令文本"
          v-model="includeScripts"
        /><small class="muted"
          >留空时，AI
          根据需求从已有服务器和构建物中匹配；手动选择可限定范围。发送候选名称、ID、构建物文件清单及当前工作流配置，不发送文件内容或保存的凭据。</small
        >
      </div>
      <div class="ai-section-label">编排需求</div>
      <v-textarea
        v-model="prompt"
        rows="5"
        variant="outlined"
        hide-details
        placeholder="例如：将「前端产物」上传到「测试服务器」的 /home/work/front。可直接填写已有服务器和构建物名称。"
      />
      <div class="ai-generate">
        <v-btn
          v-if="!busy"
          block
          variant="flat"
          color="primary"
          prepend-icon="mdi-auto-fix"
          :disabled="!prompt.trim()"
          @click="generate"
          >生成工作流草稿</v-btn
        ><v-btn v-else block variant="tonal" color="primary" @click="cancel"
          ><v-progress-circular
            size="15"
            width="2"
            indeterminate
            class="mr-2"
          />生成中 · 点击取消</v-btn
        >
      </div>
      <div v-if="candidate" class="ai-result">
        <div class="ai-result-heading">
          <v-icon
            size="17"
            :color="
              candidate.workflow || candidate.combination
                ? 'success'
                : 'warning'
            "
            >{{
              candidate.workflow || candidate.combination
                ? "mdi-file-check-outline"
                : "mdi-help-circle-outline"
            }}</v-icon
          >{{
            candidate.workflow || candidate.combination
              ? "草稿已生成"
              : "需要补充信息"
          }}
        </div>
        <p>{{ candidate.summary }}</p>
        <div
          v-if="matchedServers.length || matchedArtifacts.length"
          class="ai-matched-resources"
        >
          <v-chip
            v-for="s in matchedServers"
            :key="`server-${s.id}`"
            size="small"
            prepend-icon="mdi-server"
            >{{ s.name }}</v-chip
          >
          <v-chip
            v-for="a in matchedArtifacts"
            :key="`asset-${a.id}`"
            size="small"
            prepend-icon="mdi-package-variant-closed"
            >{{ a.name }}</v-chip
          >
        </div>
        <v-alert
          v-if="candidate.missing?.length"
          type="warning"
          variant="tonal"
          density="compact"
          >{{ candidate.missing.join("；") }}</v-alert
        >
        <details v-if="candidate.workflow || candidate.combination">
          <summary>核对当前内容与生成草稿</summary>
          <strong>生成前</strong>
          <pre>{{ JSON.stringify(JSON.parse(base), null, 2) }}</pre>
          <strong>生成后</strong>
          <pre>{{ JSON.stringify(candidate, null, 2) }}</pre>
        </details>
        <v-btn
          v-if="candidate.workflow || candidate.combination"
          block
          variant="tonal"
          color="primary"
          @click="emit('apply', candidate!, base)"
          >应用到编辑器</v-btn
        ><small v-if="candidate.workflow || candidate.combination" class="muted"
          >应用只修改草稿，检查步骤后再点击运行。</small
        >
      </div>
      <div v-else-if="!busy" class="ai-tips">
        <span>你可以这样说</span
        ><button
          @click="prompt = '将当前部署改为服务器串行执行，失败停止后续服务器。'"
        >
          把这次部署改成串行
          <v-icon size="14">mdi-arrow-top-right</v-icon></button
        ><button
          @click="
            prompt =
              '将选中的已有工作流组合为并行运行，各分支失败独立处理，不复制步骤。'
          "
        >
          组合已有工作流并行运行 <v-icon size="14">mdi-arrow-top-right</v-icon>
        </button>
      </div>
    </div>
    <footer class="ai-footer">
      <v-icon size="13">mdi-shield-outline</v-icon> AI 生成草稿，执行由你掌控
    </footer>
  </ResizablePanel>
</template>

<style scoped>
.ai-matched-resources {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 12px;
}
</style>
