<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { api } from "./api";
import type { TunnelRule, TunnelStatus } from "./types";
import IconButton from "./IconButton.vue";

const props = defineProps<{ modelValue: boolean; connection: string; serverId: string; serverName: string; closed: boolean }>();
const emit = defineEmits<{ "update:modelValue": [boolean] }>();
const rules = ref<TunnelRule[]>([]), statuses = ref<TunnelStatus[]>([]);
const busy = ref(false), loading = ref(false), error = ref(""), deleting = ref("");
const editor = ref<TunnelRule | null>(null);
const local = computed(() => editor.value?.type !== "remote");
let generation = 0, timer: ReturnType<typeof setTimeout> | undefined;
const status = (id: string) => statuses.value.find((s) => s.id === id);
function newRule() {
  deleting.value = "";
  editor.value = { id: "", name: "本地转发 8800", type: "local", bindHost: "127.0.0.1", bindPort: 8800, targetHost: "127.0.0.1", targetPort: 8800 };
}
function edit(rule: TunnelRule) { deleting.value = ""; editor.value = { ...rule }; }
async function refresh() {
  if (props.closed) { statuses.value = []; return; }
  statuses.value = await api("ListTunnels", props.connection);
}
async function act(task: () => Promise<void>) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try { await task(); } catch (e) { error.value = String(e); }
  finally { busy.value = false; }
}
async function save() {
  if (!editor.value || loading.value) return;
  const rule = { ...editor.value };
  await act(async () => {
    rules.value = await api("SaveTunnelRule", props.serverId, rule);
    editor.value = null;
    await refresh();
  });
}
async function remove() {
  const id = deleting.value;
  if (!id) return;
  await act(async () => {
    rules.value = await api("DeleteTunnelRule", props.serverId, id);
    deleting.value = "";
    if (editor.value?.id === id) editor.value = null;
    await refresh();
  });
}
async function toggle(rule: TunnelRule) {
  if (props.closed || loading.value) return;
  await act(async () => {
    if (status(rule.id)?.running) await api("StopTunnel", props.connection, rule.id);
    else await api("StartTunnel", props.connection, rule.id);
    await refresh();
  });
}
watch(() => [props.modelValue, props.closed] as const, async () => {
  const current = ++generation;
  clearTimeout(timer);
  if (!props.modelValue) return;
  loading.value = true;
  error.value = "";
  try {
    const saved = await api("GetTunnelRules", props.serverId);
    if (current !== generation) return;
    rules.value = saved;
    if (!saved.length && !editor.value) newRule();
  } catch (e) { if (current === generation) error.value = String(e); }
  finally { if (current === generation) loading.value = false; }
  async function poll() {
    if (current !== generation) return;
    try { await refresh(); }
    catch (e) { if (current === generation) error.value = String(e); }
    if (current === generation && !props.closed) timer = setTimeout(poll, 2000);
  }
  if (current === generation) await poll();
}, { immediate: true });
onBeforeUnmount(() => { generation++; clearTimeout(timer); });
</script>

<template>
  <v-dialog :model-value="modelValue" :persistent="busy" max-width="1040" scrollable @update:model-value="emit('update:modelValue', $event)">
    <v-card class="tunnel-manager">
      <v-card-title class="d-flex align-center ga-2">
        <v-icon size="20">mdi-transit-connection-variant</v-icon> SSH 隧道
        <span class="tunnel-server">{{ serverName }}</span><v-spacer />
        <IconButton icon="mdi-close" label="关闭隧道管理" :disabled="busy" @click="emit('update:modelValue', false)" />
      </v-card-title>
      <v-card-text>
        <p class="tunnel-hint">配置按服务器保存，启动后复用当前 SSH 连接。关闭此窗口会保持转发；断开、重连或关闭连接标签会停止转发，需手动重新启动。</p>
        <v-alert v-if="closed" type="warning" variant="tonal" density="compact" class="mb-3">当前 SSH 已断开，可以编辑配置，重新连接后再启动。</v-alert>
    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mb-3" role="alert">{{ error }}</v-alert>
        <div class="tunnel-heading"><span>转发规则 · {{ rules.length }}</span><v-btn size="small" prepend-icon="mdi-plus" :disabled="busy || loading" @click="newRule">添加隧道</v-btn></div>
        <v-progress-linear v-if="loading" indeterminate height="2" />
        <div class="tunnel-rules">
          <div v-if="!rules.length && !loading" class="tunnel-empty">暂无隧道，添加一条端口转发规则即可开始。</div>
          <article v-for="rule in rules" :key="rule.id" class="tunnel-rule">
            <div class="tunnel-rule-main">
              <div class="tunnel-rule-name"><span :class="status(rule.id)?.running ? 'text-success' : 'text-grey'">●</span><strong>{{ rule.name }}</strong><span class="tunnel-type">{{ rule.type === 'local' ? '本地转发' : '远程转发' }}</span></div>
              <div class="tunnel-route">{{ rule.type === 'local' ? '本机' : '服务器' }} {{ rule.bindHost }}:{{ rule.bindPort }} <span>→ SSH →</span> {{ rule.type === 'local' ? '服务器侧' : '本机侧' }} {{ rule.targetHost }}:{{ rule.targetPort }}</div>
              <div class="tunnel-state" :class="status(rule.id)?.error ? 'text-error' : ''">
                {{ status(rule.id)?.running ? `转发中 · ${status(rule.id)?.connections ?? 0} 个活动连接` : '已停止' }}<span v-if="status(rule.id)?.error"> · {{ status(rule.id)?.error }}</span>
              </div>
            </div>
            <v-btn size="small" :color="status(rule.id)?.running ? 'warning' : 'primary'" variant="tonal" :disabled="busy || closed" @click="toggle(rule)">{{ status(rule.id)?.running ? '停止' : '启动' }}</v-btn>
            <IconButton icon="mdi-pencil-outline" label="编辑隧道" :disabled="busy" @click="edit(rule)" />
            <IconButton icon="mdi-delete-outline" label="删除隧道" :disabled="busy" @click="deleting = rule.id" />
          </article>
        </div>
        <div v-if="deleting" class="tunnel-delete">
          <span>删除这条规则并停止它在所有连接中的转发？</span><v-spacer />
          <v-btn size="small" :disabled="busy" @click="deleting = ''">取消</v-btn><v-btn size="small" color="error" :loading="busy" @click="remove">确认删除</v-btn>
        </div>
        <v-form v-if="editor" class="tunnel-editor" :disabled="busy" @submit.prevent="save">
          <div class="tunnel-editor-heading">{{ editor.id ? '编辑隧道' : '添加隧道' }}</div>
          <div class="tunnel-form-row">
            <v-text-field v-model="editor.name" label="隧道名称" maxlength="200" hide-details />
            <v-select v-model="editor.type" label="转发类型" :items="[{ title: '本地转发（-L）', value: 'local' }, { title: '远程转发（-R）', value: 'remote' }]" hide-details />
          </div>
          <div class="tunnel-flow">
            <div class="tunnel-endpoint"><div>{{ local ? '本机监听' : '服务器监听' }}</div><div class="tunnel-form-row"><v-text-field v-model="editor.bindHost" label="监听 IP" hide-details /><v-text-field v-model.number="editor.bindPort" label="监听端口" type="number" min="1" max="65535" hide-details /></div></div>
            <v-icon size="22">mdi-arrow-right</v-icon>
            <div class="tunnel-endpoint"><div>{{ local ? '从服务器访问目标' : '从本机访问目标' }}</div><div class="tunnel-form-row"><v-text-field v-model="editor.targetHost" label="目标主机 / IP" hide-details /><v-text-field v-model.number="editor.targetPort" label="目标端口" type="number" min="1" max="65535" hide-details /></div></div>
          </div>
          <p class="tunnel-hint">{{ local ? '例如：本机浏览器打开 http://127.0.0.1:8800，经 SSH 访问服务器的 127.0.0.1:8800。' : '服务器收到连接后，经 SSH 转发到你电脑可访问的目标。远程转发需要服务器允许 AllowTcpForwarding。' }}</p>
          <p v-if="editor.bindHost !== '127.0.0.1' && editor.bindHost !== '::1'" class="text-warning tunnel-hint">监听非回环地址可能允许其他设备访问此端口；远程监听地址还受服务器 GatewayPorts 设置影响。</p>
          <div class="tunnel-editor-actions"><span class="tunnel-hint">{{ editor.id ? '保存修改会停止这条规则的现有转发。' : '保存后点击「启动」建立隧道。' }}</span><v-spacer /><v-btn :disabled="busy" @click="editor = null">取消</v-btn><v-btn type="submit" color="primary" :loading="busy">保存规则</v-btn></div>
        </v-form>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<style scoped>
.tunnel-manager { background: #212224; }
.tunnel-manager :deep(.v-card-text) { padding: 16px 22px 22px; }
.tunnel-server { font-size: 12px; color: #98a4b5; overflow: hidden; text-overflow: ellipsis; }
.tunnel-hint { font-size: 12px; line-height: 1.7; color: #9aa6b8; margin: 0 0 14px; }
.tunnel-heading, .tunnel-editor-actions, .tunnel-delete { display: flex; align-items: center; gap: 8px; }
.tunnel-heading { font-size: 13px; justify-content: space-between; margin-bottom: 10px; }
.tunnel-rules { border: 1px solid var(--border); border-radius: 4px; overflow: hidden; }
.tunnel-rule { display: flex; align-items: center; gap: 6px; padding: 12px; border-bottom: 1px solid var(--border); }
.tunnel-rule:last-child { border-bottom: 0; }
.tunnel-rule-main { flex: 1; min-width: 0; }
.tunnel-rule-name { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.tunnel-rule-name strong { overflow: hidden; text-overflow: ellipsis; }
.tunnel-type { font-size: 11px; color: #9aa6b8; white-space: nowrap; }
.tunnel-route { font-family: var(--terminal-font-family, monospace); font-size: 12px; margin: 6px 0; overflow-wrap: anywhere; }
.tunnel-route span { color: #8294ab; }
.tunnel-state { font-size: 11px; color: #9aa6b8; overflow-wrap: anywhere; }
.tunnel-empty { color: #8995a6; padding: 20px; font-size: 12px; text-align: center; }
.tunnel-editor { margin-top: 16px; padding: 16px; border: 1px solid #465369; border-radius: 4px; }
.tunnel-editor-heading { font-size: 13px; margin-bottom: 16px; }
.tunnel-form-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(100px, .7fr); gap: 12px; }
.tunnel-flow { display: grid; grid-template-columns: minmax(0, 1fr) 22px minmax(0, 1fr); gap: 12px; align-items: center; margin: 18px 0 12px; }
.tunnel-endpoint > div:first-child { color: #98aac2; font-size: 12px; margin-bottom: 14px; }
.tunnel-editor-actions .tunnel-hint { margin: 0; }
.tunnel-delete { font-size: 12px; padding: 12px; margin-top: 12px; background: #39282a; }
@media (max-width: 750px) { .tunnel-flow { grid-template-columns: 1fr; } .tunnel-flow > .v-icon { transform: rotate(90deg); justify-self: center; } }
</style>
