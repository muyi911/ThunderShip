<script setup lang="ts">
import { onMounted, ref, toRefs } from "vue";
import type { Workspace } from "./useWorkspace";
const props = defineProps<{ workspace: Workspace }>();
const {
  cfg,
  view,
  provider,
  confirmDelete,
  editProvider,
  saveProvider,
  getModels,
  testModel,
  saveSettings,
  busy,
  apiKey,
  globalConcurrency,
  themeChoice,
  uploadProtocol,
} = toRefs(props.workspace);
const dataDirectory = ref("");
const openingDirectory = ref(false);
onMounted(async () => {
  try {
    dataDirectory.value = await props.workspace.api("GetDataDirectory");
  } catch (e) {
    props.workspace.error(e);
  }
});
async function openDataDirectory() {
  openingDirectory.value = true;
  try {
    await props.workspace.api("OpenDataDirectory");
  } catch (e) {
    props.workspace.error(e);
  } finally {
    openingDirectory.value = false;
  }
}
</script>
<template>
  <template v-if="view === 'settings'"
    ><div class="page-heading">
      <div>
        <h1>设置</h1>
      </div>
    </div>
    <section class="config-card">
      <header>
        <v-icon size="18">mdi-auto-fix</v-icon
        ><strong>AI 服务 · OpenAI 兼容接口</strong><span class="spacer" /><v-btn
          size="small"
          @click="editProvider()"
          >新增提供方</v-btn
        >
      </header>
      <div v-if="cfg.providers.length" class="provider-tabs">
        <v-chip
          v-for="p in cfg.providers"
          :key="p.id"
          :color="provider.id === p.id ? 'primary' : undefined"
          @click="editProvider(p)"
          >{{ p.name }}</v-chip
        >
      </div>
      <div class="config-grid">
        <v-text-field label="提供方名称" v-model="provider.name" /><v-select
          label="结构化输出模式"
          v-model="provider.mode"
          :items="[
            { title: '自动兼容', value: 'auto' },
            { title: '严格 JSON Schema', value: 'schema' },
            { title: '普通 JSON 文本', value: 'json' },
          ]"
        /><v-text-field
          label="API Base URL（包含 /v1 等路径前缀）"
          v-model="provider.baseUrl"
          class="span-2"
        /><v-text-field
          label="API Key"
          v-model="apiKey"
          type="password"
          autocomplete="new-password"
          :placeholder="
            provider.hasSecret ? '已保存 · 留空保持原值' : '输入 API Key'
          "
        /><v-combobox
          label="模型 ID · 获取列表或手动填写"
          v-model="provider.model"
          :items="provider.models || []"
        />
      </div>
      <v-checkbox
        label="允许内网 HTTP（Key 将明文传输）"
        v-model="provider.allowHttp"
      />
      <div class="setting-actions">
        <v-btn
          variant="flat"
          color="primary"
          :disabled="busy"
          @click="saveProvider"
          >保存 AI 服务</v-btn
        ><v-btn
          variant="outlined"
          :disabled="busy || !provider.id"
          @click="getModels"
          >获取模型</v-btn
        ><v-btn
          variant="outlined"
          :disabled="busy || !provider.id || !provider.model"
          @click="testModel"
          >测试已保存的模型</v-btn
        ><v-btn
          v-if="provider.id"
          color="error"
          :disabled="busy"
          @click="
            confirmDelete = {
              kind: 'provider',
              id: provider.id,
              name: provider.name,
            }
          "
          >删除</v-btn
        >
      </div>
      <p class="setting-note">
        先保存再测试。接口：{{
          provider.baseUrl.replace(/\/$/, "")
        }}/models；生成：/chat/completions。Key 存储在 Windows 凭据管理器。
      </p>
    </section>
    <section class="config-card mt-5">
      <header>
        <v-icon size="18">mdi-tune</v-icon><strong>工作空间</strong>
      </header>
      <div class="config-grid">
        <v-select
          label="外观主题"
          :items="[
            { title: '跟随系统', value: 'system' },
            { title: '浅色', value: 'light' },
            { title: '深色', value: 'dark' },
          ]"
          v-model="themeChoice"
        /><v-text-field
          type="number"
          label="全局最大远程步骤数"
          min="1"
          max="64"
          v-model.number="globalConcurrency"
        />
        <v-select
          label="文件上传方式"
          v-model="uploadProtocol"
          :items="[
            { title: 'SFTP', value: 'sftp' },
            { title: 'SCP', value: 'scp' },
          ]"
          hint="用于文件列表和工作流上传，两种方式均支持替换正在运行的 Linux 程序。SCP 需要服务器提供 scp 命令。"
          persistent-hint
        />
      </div>
      <div class="setting-actions">
        <v-btn
          variant="tonal"
          color="primary"
          :disabled="busy"
          @click="saveSettings"
          >保存偏好</v-btn
        >
      </div>
      <div class="data-directory">
        <v-text-field
          label="数据目录"
          :model-value="dataDirectory"
          readonly
          hide-details
          prepend-inner-icon="mdi-folder-outline"
        />
        <v-btn
          variant="outlined"
          prepend-icon="mdi-folder-open-outline"
          :loading="openingDirectory"
          :disabled="!dataDirectory"
          title="在文件管理器中打开当前数据目录"
          @click="openDataDirectory"
          >打开数据目录</v-btn
        >
      </div>
    </section></template
  >
</template>

<style scoped>
.data-directory {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 24px;
}
.data-directory :deep(.v-input) {
  min-width: 0;
}
</style>
