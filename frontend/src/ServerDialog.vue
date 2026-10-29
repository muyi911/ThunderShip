<script setup lang="ts">
import { computed, ref, toRefs, watch } from "vue";
import type { Workspace } from "./useWorkspace";
import IconButton from "./IconButton.vue";
import ServerAuthentication from "./ServerAuthentication.vue";
const props = defineProps<{ workspace: Workspace }>();
const {
  serverDialog,
  error,
  saveServer,
  testConnection,
  testingConnection,
  trustHost,
  detectSFTP,
  busy,
  server,
  secret,
  probe,
  trustDialog,
  cfg,
} = toRefs(props.workspace);
const tab = ref("general");
const hasSavedSecret = computed(() =>
  cfg.value.servers.some(
    (s) =>
      s.id === server.value.id &&
      s.hasSecret &&
      s.host === server.value.host &&
      s.port === server.value.port &&
      s.user === server.value.user &&
      s.auth === server.value.auth &&
      s.keyPath === server.value.keyPath,
  ),
);
watch(serverDialog, (open) => {
  if (open) tab.value = "general";
});
</script>
<template>
  <v-dialog v-model="serverDialog" max-width="720" scrollable
    ><v-card class="server-dialog"
      ><v-card-title class="d-flex align-center"
        >{{ server.id ? "编辑 SSH 连接" : "添加 SSH 连接"
        }}<v-spacer /><IconButton
          icon="mdi-close"
          label="关闭连接配置"
          :disabled="testingConnection"
          @click="serverDialog = false"
      /></v-card-title>
      <v-card-text>
        <div class="connection-panel">
          <v-tabs
            v-model="tab"
            color="primary"
            density="compact"
            height="32"
            align-tabs="center"
            class="connection-tabs"
          >
            <v-tab value="general" prepend-icon="mdi-server-network"
              >常规</v-tab
            >
            <v-tab value="advanced" prepend-icon="mdi-tune">高级</v-tab>
          </v-tabs>
          <v-window v-model="tab" class="server-tabs" :touch="false">
            <v-window-item value="general">
              <div class="connection-grid">
                <div class="connection-field">
                  <label for="connection-name">名称</label
                  ><v-text-field
                    id="connection-name"
                    v-model="server.name"
                    variant="solo-filled"
                    flat
                  />
                </div>
                <div class="connection-field">
                  <label for="connection-host">主机 / IP</label
                  ><v-text-field
                    id="connection-host"
                    v-model="server.host"
                    variant="solo-filled"
                    flat
                  />
                </div>
                <div class="connection-field">
                  <label for="connection-user">登录用户</label
                  ><v-text-field
                    id="connection-user"
                    v-model="server.user"
                    variant="solo-filled"
                    flat
                  />
                </div>
                <div class="connection-field">
                  <label for="connection-port">端口</label
                  ><v-text-field
                    id="connection-port"
                    v-model.number="server.port"
                    type="number"
                    min="1"
                    max="65535"
                    variant="solo-filled"
                    flat
                  />
                </div>
                <ServerAuthentication
                  v-if="serverDialog"
                  class="span-2"
                  :server="server"
                  :servers="cfg.servers"
                  v-model:secret="secret"
                  :has-saved-secret="hasSavedSecret"
                  :disabled="testingConnection"
                  @error="error"
                />
                <div class="connection-field span-2">
                  <label for="connection-group">分组</label
                  ><v-combobox
                    id="connection-group"
                    v-model="server.group"
                    :items="[
                      ...new Set(
                        cfg.servers.map((s) => s.group.trim()).filter(Boolean),
                      ),
                    ]"
                    placeholder="选择或输入分组"
                    variant="solo-filled"
                    flat
                  />
                </div>
              </div>
            </v-window-item>
            <v-window-item value="advanced">
              <div class="server-section-title">文件传输</div>
              <v-checkbox
                label="默认使用 sudo 上传"
                title="也可在工作流的上传节点中单独开启；终端中的 sudo 不影响文件传输"
                v-model="server.sudoUpload"
                class="mb-3"
              />
              <div class="config-grid">
                <v-text-field label="sudo 目标用户" v-model="server.sudoUser" />
                <v-text-field
                  label="sftp-server 绝对路径"
                  v-model="server.sftpPath"
                  title="仅 sudo 文件传输使用：以目标用户启动服务器上的 SFTP 程序"
                />
              </div>
              <v-btn
                size="small"
                class="mt-2"
                prepend-icon="mdi-magnify"
                title="通过已保存的连接检测 SFTP 程序路径；请先保存并信任服务器指纹"
                :disabled="!server.id || busy"
                @click="detectSFTP"
                >检测常见路径</v-btn
              >
              <v-divider class="my-5" />
              <div class="server-section-title">任务调度</div>
              <div class="config-grid">
                <v-text-field
                  label="同机最大并发"
                  type="number"
                  min="0"
                  max="64"
                  v-model.number="server.concurrency"
                  hint="0 不限制，1 串行；仍受全局并发限制"
                  persistent-hint
                />
                <v-text-field
                  label="物理主机标识"
                  v-model="server.machine"
                  hint="同一机器的多个连接填相同值；留空按地址和端口识别"
                  persistent-hint
                />
              </div>
              <div v-if="server.fingerprint" class="fingerprint mt-4">
                已信任指纹：{{ server.fingerprint }}
              </div>
            </v-window-item>
          </v-window>
        </div>
        <v-alert
          v-if="probe && !trustDialog"
          :type="probe.ok ? 'success' : 'error'"
          density="compact"
          variant="tonal"
          class="mt-4"
          ><p>{{ probe.message }}</p>
          <code v-if="probe.fingerprint">{{ probe.fingerprint }}</code>
        </v-alert> </v-card-text
      ><v-card-actions
        ><v-btn
          title="测试当前填写的 SSH / SFTP 配置，无需先保存"
          prepend-icon="mdi-connection"
          color="primary"
          variant="tonal"
          :loading="testingConnection"
          :disabled="busy"
          @click="testConnection"
          >测试连接</v-btn
        ><v-spacer /><v-btn
          :disabled="testingConnection"
          @click="serverDialog = false"
          >取消</v-btn
        ><v-btn
          color="primary"
          variant="flat"
          :disabled="busy || testingConnection"
          @click="saveServer"
          >保存服务器</v-btn
        ></v-card-actions
      ></v-card
    ></v-dialog
  >
  <v-dialog
    v-model="trustDialog"
    max-width="560"
    aria-labelledby="host-trust-title"
  >
    <v-card>
      <v-card-title id="host-trust-title" class="d-flex align-center ga-2">
        <v-icon color="primary">mdi-shield-key-outline</v-icon>
        确认服务器指纹
      </v-card-title>
      <v-card-text>
        <p>首次连接此服务器，请核对 SHA-256 指纹后继续。</p>
        <div class="host-trust-endpoint my-4">
          <strong>{{ server.name || server.host }}</strong>
          <span>{{ server.user }}@{{ server.host }}:{{ server.port }}</span>
        </div>
        <code class="host-trust-fingerprint">{{ probe?.fingerprint }}</code>
        <p class="muted mt-4">确认后继续测试；保存服务器时才保存信任记录。</p>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="trustDialog = false">取消</v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :disabled="testingConnection"
          @click="trustHost"
          >信任并继续测试</v-btn
        >
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
<style scoped>
.host-trust-endpoint {
  display: grid;
  gap: 4px;
  overflow-wrap: anywhere;
}
.host-trust-fingerprint {
  display: block;
  padding: 12px;
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  border-radius: 4px;
  overflow-wrap: anywhere;
  user-select: text;
}
.server-tabs {
  min-height: 340px;
}
.connection-panel {
  background: rgb(var(--v-theme-background));
  border-radius: 5px;
  padding: 18px 20px;
}
.connection-tabs {
  margin-bottom: 24px;
}
.connection-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 22px 24px;
}
.connection-field {
  display: grid;
  gap: 7px;
}
.connection-field > label {
  font-size: 13px;
  color: rgb(var(--v-theme-on-surface));
  opacity: 0.8;
}
.connection-grid :deep(.v-field__input) {
  min-height: 36px;
  padding-top: 7px;
  padding-bottom: 7px;
}
.connection-grid :deep(.v-field__append-inner) {
  padding-top: 0;
  align-items: center;
}
.connection-grid :deep(.v-field__append-inner .v-btn) {
  width: 28px;
  height: 28px;
}
@media (max-width: 600px) {
  .connection-grid {
    grid-template-columns: 1fr;
  }
  .span-2 {
    grid-column: span 1;
  }
}
.server-section-title {
  font-weight: 600;
  margin: 6px 0 12px;
}
</style>
