<script setup lang="ts">
import { watch, onBeforeUnmount, ref } from "vue";
import type { Server, ConnectionInfo, Provider } from "./types";
import { api } from "./api";
import TerminalView from "./TerminalView.vue";
import RemoteFiles from "./RemoteFiles.vue";
import IconButton from "./IconButton.vue";
import EnhancedTerminal from "./EnhancedTerminal.vue";
import ServerMonitor from "./ServerMonitor.vue";
import TunnelManager from "./TunnelManager.vue";
const props = defineProps<{ server: Server; providers: Provider[]; active: boolean }>();
const emit = defineEmits<{ error: [unknown] }>();
const connection = ref<ConnectionInfo | null>(null),
  busy = ref(false),
  closed = ref(true),
  filesOpen = ref(true);
const mode = ref("enhanced");
const interactiveTerminal = ref<InstanceType<typeof TerminalView>>(), enhancedTerminal = ref<InstanceType<typeof EnhancedTerminal>>();
async function terminalDirectory(path: string) {
  try {
    if (mode.value === "enhanced") enhancedTerminal.value?.changeDirectory(path);
    else await interactiveTerminal.value?.changeDirectory(path);
  } catch (e) { emit("error", e); }
}
const tunnelsOpen = ref(false);
let generation = 0;
async function connect() {
  const current = ++generation;
  busy.value = true;
  if (connection.value) {
    void api("CloseConnection", connection.value.id).catch(() => {});
    connection.value = null;
  }
  try {
    const info = await api("OpenConnection", props.server.id);
    if (current !== generation) {
      await api("CloseConnection", info.id);
      return;
    }
    connection.value = info;
    closed.value = false;
  } catch (e) {
    if (current === generation) emit("error", e);
  } finally {
    if (current === generation) busy.value = false;
  }
}
async function disconnect() {
  if (connection.value) await api("CloseConnection", connection.value.id);
  closed.value = true;
}
watch(
  () => props.server.id,
  () => void connect(),
  { immediate: true },
);
onBeforeUnmount(() => {
  generation++;
  if (connection.value)
    void api("CloseConnection", connection.value.id).catch(() => {});
});
</script>
<template>
  <section class="server-workspace">
    <v-progress-linear v-if="busy" indeterminate color="primary" height="2" />
    <div class="connection-body">
      <div class="terminal-pane">
        <header class="connection-toolbar">
          <v-menu
            ><template #activator="{ props: menuProps }"
              ><v-btn
                v-bind="menuProps"
                size="small"
                append-icon="mdi-chevron-down"
                prepend-icon="mdi-format-list-bulleted"
                >{{ mode === "enhanced" ? "增强终端" : "交互终端" }}</v-btn
              ></template
            ><v-list density="compact"
              ><v-list-item
                title="增强终端"
                :active="mode === 'enhanced'"
                @click="mode = 'enhanced'" /><v-list-item
                title="交互终端"
                :active="mode === 'terminal'"
                @click="mode = 'terminal'" /></v-list
          ></v-menu>
          <span
            class="connection-address"
            :title="server.user + '@' + server.host + ':' + server.port"
            >{{ server.user }}@{{ server.host }}</span
          ><span
            :class="closed ? 'text-grey' : 'text-success'"
            :title="busy ? '连接中' : closed ? '已断开' : '已连接'"
            >●</span
          ><span class="spacer" />
          <IconButton
            icon="mdi-transit-connection-variant"
            label="SSH 隧道 / 端口转发"
            size="x-small"
            :disabled="busy || !connection"
            @click="tunnelsOpen = true"
          />
          <IconButton
            icon="mdi-refresh"
            label="重新连接"
            size="x-small"
            :disabled="busy"
            @click="connect"
          />
          <IconButton
            icon="mdi-power-plug-off-outline"
            label="断开当前连接"
            size="x-small"
            :disabled="busy || closed"
            @click="disconnect().catch((e) => emit('error', e))"
          />
          <IconButton
            icon="mdi-folder-network-outline"
            label="显示或隐藏文件管理"
            size="x-small"
            @click="filesOpen = !filesOpen"
          />
        </header>
        <TerminalView
          ref="interactiveTerminal"
          v-if="connection"
          v-show="mode === 'terminal'"
          :key="connection.id"
          :connection="connection.id"
          :initial-directory="connection.directory"
          :server-id="server.id"
          :closed="closed"
          @error="emit('error', $event)"
          @closed="closed = true"
        />
        <EnhancedTerminal
          ref="enhancedTerminal"
          v-if="connection"
          v-show="mode === 'enhanced'"
          :key="'enhanced-' + connection.id"
          :connection="connection.id"
          :initial-directory="connection.directory"
          :server="server"
          :providers="providers"
          :closed="closed"
          @error="emit('error', $event)"
        />
        <div v-else class="terminal-empty">
          {{ busy ? "正在建立 SSH 连接…" : "连接未建立，点击重新连接。" }}
        </div>
      </div>
      <div v-if="connection" class="file-boundary">
        <button type="button" class="file-collapse-button"
          :title="filesOpen ? '收起文件列表' : '展开文件列表'"
          :aria-label="filesOpen ? '收起文件列表' : '展开文件列表'"
          :aria-expanded="filesOpen" :aria-controls="'remote-files-' + connection.id"
          @click="filesOpen = !filesOpen">
          <v-icon size="18">{{ filesOpen ? 'mdi-chevron-right' : 'mdi-chevron-left' }}</v-icon>
        </button>
      </div>
      <RemoteFiles
        v-if="connection"
        v-show="filesOpen"
        :key="connection.id"
        :id="'remote-files-' + connection.id"
        :connection="connection.id"
        :initial-directory="connection.directory"
        :closed="closed"
        :server-id="server.id"
        @terminal-directory="terminalDirectory"
        @error="emit('error', $event)"
      />
      <ServerMonitor v-if="connection" :key="'monitor-' + connection.id" :connection="connection.id"
        :server="server" :active="active" :closed="closed" />
    </div>
    <TunnelManager v-if="connection" :key="'tunnels-' + connection.id" v-model="tunnelsOpen"
      :connection="connection.id" :server-id="server.id" :server-name="server.name" :closed="closed" />
  </section>
</template>

<style scoped>
.connection-body { position: relative; }
.file-boundary { position: relative; flex: 0 0 8px; background: #202224; border-left: 1px solid var(--border); }
.file-collapse-button { position: absolute; z-index: 10; top: 50%; left: -12px; transform: translateY(-50%); width: 20px; height: 48px; border: 1px solid #42464c; border-radius: 5px; background: #292c30; color: #b8c5d8; cursor: pointer; }
.file-collapse-button:hover, .file-collapse-button:focus-visible { color: #9fc3ff; border-color: #7699cb; background: #323a46; outline: none; }
.terminal-pane {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  background: #191a1b;
}
.connection-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 38px;
  padding: 3px 6px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.connection-toolbar > .v-btn {
  font-size: 11px;
  height: 28px;
  min-width: 0;
  padding: 0 5px;
}
.connection-address {
  color: var(--muted);
  font:
    10px Consolas,
    monospace;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  max-width: 40%;
}
</style>
