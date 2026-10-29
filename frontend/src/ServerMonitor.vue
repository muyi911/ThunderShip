<script setup lang="ts">
import { computed, ref } from "vue";
import type { Server } from "./types";
import { useServerMetrics } from "./composables/useServerMetrics";
import ResizablePanel from "./ResizablePanel.vue";
import IconButton from "./IconButton.vue";

const props = defineProps<{ connection: string; server: Server; active: boolean; closed: boolean }>();
const open = ref(false), allCores = ref(false), processLimit = ref(5);
const { data, metrics, samples, loading, error, refresh } = useServerMetrics(
  () => props.connection, () => props.active && !props.closed,
);
const available = computed(() => !props.closed && !error.value && !!data.value);
const cores = computed(() => allCores.value ? metrics.value?.cores : metrics.value?.cores.slice(0, 8));
const processes = computed(() => [...(data.value?.processes || [])].sort((a, b) => b.cpu - a.cpu).slice(0, processLimit.value));
const networkMax = computed(() => Math.max(1, ...samples.value.flatMap(s => [s.send || 0, s.receive || 0])));
const chartLines = computed(() => ([
  { key: "cpu", label: "CPU", color: "#3671e8" },
  { key: "memory", label: "内存", color: "#26a69a" },
  { key: "send", label: "上传", color: "#ffa000" },
  { key: "receive", label: "下载", color: "#4caf50" },
] as const).map(series => {
  let path = "", drawing = false;
  samples.value.forEach((sample, i) => {
    const value = sample[series.key];
    if (value === null) { drawing = false; return; }
    const maximum = series.key === "cpu" || series.key === "memory" ? 100 : networkMax.value;
    const x = 320 * i / Math.max(1, samples.value.length - 1), y = 115 - Math.min(1, value / maximum) * 110;
    path += `${drawing ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)} `;
    drawing = true;
  });
  return { ...series, path };
}));
function bytes(value: number | null | undefined, compact = false) {
  if (value == null) return "—";
  const unit = Math.min(4, Math.floor(Math.log(Math.max(1, value)) / Math.log(1024)));
  return (value / 1024 ** unit).toFixed(unit ? 1 : 0).replace(/\.0$/, "") + (compact ? ["B", "K", "M", "G", "T"] : [" B", " KB", " MB", " GB", " TB"])[unit];
}
function percent(value: number | null | undefined) { return value == null ? "—" : value.toFixed(1) + "%"; }
function rate(value: number | null | undefined) { return value == null ? "—" : bytes(value) + "/s"; }
function uptime(value: number) {
  return `${Math.floor(value / 86400)}d ${Math.floor(value % 86400 / 3600)}h ${Math.floor(value % 3600 / 60)}m`;
}
const rail = computed(() => [
  { label: "CPU", value: available.value ? String(data.value!.cpuCount) : "—", color: "#607dad", title: "逻辑 CPU 数量", fill: null },
  { label: "上传", value: bytes(available.value ? metrics.value?.network.sendRate : null, true), color: "#b65d00", title: "上传速率：" + rate(metrics.value?.network.sendRate), fill: null },
  { label: "下载", value: bytes(available.value ? metrics.value?.network.receiveRate : null, true), color: "#5c852f", title: "下载速率：" + rate(metrics.value?.network.receiveRate), fill: null },
  { label: "CPU", value: available.value ? percent(metrics.value?.cpu?.usage) : "—", color: "#247c22", title: "CPU 使用率", fill: metrics.value?.cpu?.usage ?? 0 },
  { label: "内存", value: bytes(available.value ? data.value?.memory.available : null, true), color: "#445989", title: "可用内存 / 总内存：" + bytes(data.value?.memory.available) + " / " + bytes(data.value?.memory.total), fill: metrics.value?.memoryPercent ?? 0 },
  { label: "磁盘", value: bytes(available.value ? metrics.value?.diskAvailable : null, true), color: "#258c84", title: "可用磁盘 / 总容量（按设备去重）：" + bytes(metrics.value?.diskAvailable) + " / " + bytes(metrics.value?.diskTotal), fill: metrics.value?.diskTotal ? (1 - metrics.value.diskAvailable / metrics.value.diskTotal) * 100 : 0 },
]);
</script>

<template>
  <ResizablePanel v-if="open" class="server-overview" label="服务器概览" storage-key="thundership.server-overview-width"
    :default-size="380" :min-size="340" :max-size="700" :max-ratio="0.85">
    <header class="overview-header">
      <h2>服务器概览</h2><span class="spacer" />
      <IconButton icon="mdi-refresh" label="刷新服务器信息" size="x-small" :disabled="loading || closed" @click="refresh" />
      <IconButton icon="mdi-close" label="关闭服务器概览" size="x-small" @click="open = false" />
    </header>
    <v-progress-linear v-if="loading && !data" indeterminate color="primary" height="2" />
    <div class="overview-scroll">
      <p v-if="closed || error" class="metrics-message" role="status">{{ closed ? '连接已断开，信息已停止刷新。' : error }}{{ data ? ' 下方为最后一次采集结果。' : '' }}</p>
      <p v-else-if="!data" class="metrics-message">正在读取服务器信息…</p>
      <template v-if="data && metrics">
        <section class="info-card identity-card">
          <div><span><v-icon size="17">mdi-account-outline</v-icon>用户</span><strong :title="server.user">{{ server.user }}</strong></div>
          <div><span><v-icon size="17">mdi-clock-outline</v-icon>运行时间</span><strong>{{ uptime(data.uptime) }}</strong></div>
          <div><span><v-icon size="17">mdi-web</v-icon>Host</span><strong :title="`${server.host}:${server.port} · ${data.hostname}`">{{ server.host }}:{{ server.port }}</strong></div>
          <div><span><v-icon size="17">mdi-laptop</v-icon>系统</span><strong :title="data.os">{{ data.os }}</strong></div>
        </section>
        <section class="info-card cpu-summary">
          <div class="cpu-grid">
            <div><strong>{{ percent(metrics.cpu?.usage) }}</strong><span>平均 CPU</span></div>
            <div><strong>{{ percent(metrics.cpu?.system) }}</strong><span>内核态</span></div>
            <div><strong>{{ percent(metrics.cpu?.user) }}</strong><span>用户态</span></div>
            <div><strong>{{ percent(metrics.cpu?.iowait) }}</strong><span>I/O 等待</span></div>
            <div><strong>{{ percent(metrics.cpu ? metrics.cpu.usage * data.cpuCount : null) }}</strong><span>总 CPU</span></div>
            <div><strong>{{ percent(metrics.cpu ? metrics.cpu.system * data.cpuCount : null) }}</strong><span>内核态</span></div>
            <div><strong>{{ percent(metrics.cpu ? metrics.cpu.user * data.cpuCount : null) }}</strong><span>用户态</span></div>
            <div><strong>{{ percent(metrics.cpu ? metrics.cpu.iowait * data.cpuCount : null) }}</strong><span>I/O 等待</span></div>
          </div>
          <div class="memory-bars">
            <div><span>物理内存</span><div class="capacity-bar" :title="'已用 / 总内存'" role="meter" aria-label="物理内存使用率" :aria-valuenow="metrics.memoryPercent ?? 0" aria-valuemin="0" aria-valuemax="100"><i :style="{ width: (metrics.memoryPercent || 0) + '%' }" /><b>{{ bytes(metrics.memoryUsed, true) }}/{{ bytes(data.memory.total, true) }}</b></div></div>
            <div><span>Swap 内存</span><div class="capacity-bar swap" title="已用 / 总交换空间"><i :style="{ width: (metrics.swapPercent || 0) + '%' }" /><b>{{ bytes(metrics.swapUsed, true) }}/{{ bytes(data.memory.swapTotal, true) }}</b></div></div>
          </div>
        </section>
        <section class="info-card network-chart">
          <div class="traffic-grid" title="汇总排除 lo、veth、docker 和 br- 虚拟接口，减少重复统计；所有接口仍在下方展示。">
            <div><span>累计上行</span><strong>{{ bytes(metrics.network.sent, true) }}</strong></div>
            <div><span>累计下行</span><strong>{{ bytes(metrics.network.received, true) }}</strong></div>
            <div class="tx"><span>上行速率</span><strong>{{ rate(metrics.network.sendRate) }}</strong></div>
            <div class="rx"><span>下行速率</span><strong>{{ rate(metrics.network.receiveRate) }}</strong></div>
          </div>
          <div class="chart-legend"><span v-for="line in chartLines" :key="line.key"><i :style="{ background: line.color }" />{{ line.label }}</span></div>
          <svg class="metric-chart" viewBox="0 0 320 120" preserveAspectRatio="none" role="img" aria-label="CPU、内存、上传和下载趋势">
            <title>CPU 与内存为百分比；网络速率单独按 {{ rate(networkMax) }} 缩放</title>
            <path v-for="line in chartLines" :key="line.key" :d="line.path" :stroke="line.color" fill="none" stroke-width="1.5" vector-effect="non-scaling-stroke" />
          </svg>
          <small>{{ samples.length < 2 ? '正在采样，速率将在下次刷新后显示' : '最近 ' + samples.length + ' 次采样 · 网络速率独立缩放' }}</small>
        </section>
        <section class="info-card cores-card">
          <div v-for="(core, index) in cores" :key="core.name" class="core-row">
            <span>CPU{{ index + 1 }}</span><div class="core-ticks"><i :style="{ width: (core.usage || 0) + '%' }" /></div><span>{{ percent(core.usage) }}</span>
          </div>
          <button v-if="metrics.cores.length > 8" class="cores-toggle" @click="allCores = !allCores">{{ allCores ? '收起' : '展开全部 ' + metrics.cores.length + ' 核' }}<v-icon size="16">{{ allCores ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon></button>
        </section>
        <section class="info-card table-card">
          <header><h3>进程</h3><select v-model.number="processLimit" aria-label="显示进程数量"><option v-for="limit in [5, 10, 25, 50, 100]" :key="limit" :value="limit">{{ limit }}</option></select></header>
          <table class="process-table"><thead><tr><th>进程</th><th>进程 ID</th><th class="sorted" title="按 CPU 排序，最多 100 项；CPU 是进程自启动以来的平均使用率">%CPU ↓</th><th>内存</th></tr></thead>
            <tbody><tr v-for="process in processes" :key="process.pid"><td :title="process.name"><span class="name-chip">{{ process.name }}</span></td><td>{{ process.pid }}</td><td>{{ percent(process.cpu) }}</td><td>{{ bytes(process.memory, true) }}</td></tr></tbody></table>
          <p v-if="!processes.length" class="empty-metrics">暂无进程数据</p>
        </section>
        <section class="info-card table-card">
          <header><h3>网络接口</h3></header>
          <table class="interface-table"><thead><tr><th>名称</th><th>上/下行</th><th>总流量</th><th>IP</th></tr></thead><tbody>
            <tr v-for="network in metrics.networks" :key="network.name"><td :title="network.name"><span class="name-chip teal">{{ network.name }}</span></td><td><span class="tx">↑ {{ rate(network.sendRate) }}</span><br /><span class="rx">↓ {{ rate(network.receiveRate) }}</span></td><td>↑ {{ bytes(network.sent, true) }}<br />↓ {{ bytes(network.received, true) }}</td><td :title="network.addresses.join('\n')"><span class="name-chip teal">{{ network.addresses.join(', ') || '—' }}</span></td></tr>
          </tbody></table><p v-if="!metrics.networks.length" class="empty-metrics">暂无网络接口数据</p>
        </section>
        <section class="info-card table-card">
          <header><h3>磁盘挂载</h3></header>
          <table class="disk-table"><thead><tr><th>名称</th><th>容量 / 可用</th><th>Path</th></tr></thead><tbody>
            <tr v-for="disk in data.disks" :key="disk.source + disk.mount"><td :title="disk.source"><span class="name-chip teal">{{ disk.source }}</span></td><td><div class="capacity-bar" :title="bytes(disk.total) + ' / ' + bytes(disk.available)"><i :style="{ width: (disk.total ? (1 - disk.available / disk.total) * 100 : 0) + '%' }" /><b>{{ bytes(disk.total, true) }}/{{ bytes(disk.available, true) }}</b></div></td><td :title="disk.mount"><span class="name-chip">{{ disk.mount }}</span></td></tr>
          </tbody></table><p v-if="!data.disks.length" class="empty-metrics">暂无磁盘数据</p>
        </section>
        <p v-for="warning in data.warnings" :key="warning" class="metrics-message">{{ warning }}</p>
        <p class="updated-at">{{ server.name }} · {{ new Date(data.timestamp).toLocaleTimeString() }} 更新</p>
      </template>
    </div>
  </ResizablePanel>
  <aside class="metrics-rail" aria-label="服务器实时指标">
    <button class="overview-toggle" :aria-expanded="open" title="服务器概览" aria-label="显示或隐藏服务器概览" @click="open = !open"><v-icon size="18">mdi-console-network-outline</v-icon></button>
    <button v-for="item in rail" :key="item.label + item.title" class="rail-metric" :title="(closed ? '连接已断开' : error || item.title) + ' · 点击查看服务器概览'" :aria-label="item.label + ' ' + item.value + '，查看服务器概览'" @click="open = !open">
      <span class="rail-dash">-</span><span class="rail-label">{{ item.label }}</span>
      <span class="rail-value" :class="{ 'rail-bar': item.fill !== null, stale: !available }" :style="{ backgroundColor: item.fill === null ? item.color : item.color + '66' }"><i v-if="item.fill !== null && available" :style="{ height: item.fill + '%', background: item.fill > 90 ? '#b94040' : item.color }" /><b>{{ item.value }}</b></span>
    </button>
  </aside>
</template>

<style scoped>
.metrics-rail { width: 40px; flex: 0 0 40px; background: #222426; border-left: 1px solid #333638; overflow-y: auto; scrollbar-width: none; }
.overview-toggle, .rail-metric { width: 100%; display: flex; flex-direction: column; align-items: center; cursor: pointer; color: #d5dde4; }
.overview-toggle { padding: 5px 0 0; }
.rail-metric:hover, .overview-toggle:hover { background: #ffffff0b; }
.rail-metric:focus-visible, .overview-toggle:focus-visible { outline: 1px solid #80afff; outline-offset: -2px; }
.rail-dash { color: #9299a3; line-height: 9px; font-size: 9px; }
.rail-label { font-size: 10px; letter-spacing: 1px; line-height: 16px; }
.rail-value { width: 30px; min-height: 16px; margin: 2px 4px 15px; border-radius: 4px; font: 500 8px/16px var(--terminal-font-family); color: white; position: relative; overflow: hidden; }
.rail-value b { position: relative; font-weight: 500; z-index: 1; }
.rail-bar { height: 43px; line-height: 43px; }
.rail-bar i { position: absolute; bottom: 0; left: 0; right: 0; }
.rail-value.stale { background: #3d4147 !important; }
.server-overview { display: flex; flex-direction: column; position: absolute; top: 0; bottom: 0; right: 40px; z-index: 25; background: #222426; color: #d8dee5; box-shadow: -8px 0 20px #0003; }
.overview-header { display: flex; align-items: center; flex-shrink: 0; height: 36px; padding: 0 5px 0 12px; }
.overview-header h2 { font-size: 16px; font-weight: 600; }
.overview-scroll { min-height: 0; overflow: auto; padding: 0 6px 6px; scrollbar-width: thin; scrollbar-color: #44484c transparent; }
.info-card { margin-bottom: 10px; padding: 12px 14px; background: #191b1c; border-radius: 6px; font-size: 12px; }
.identity-card { display: grid; grid-template-columns: 1fr 1fr; gap: 7px 12px; }
.identity-card > div { min-width: 0; }
.identity-card span { display: flex; gap: 8px; color: #929ca9; align-items: center; }
.identity-card strong { display: block; padding-left: 25px; line-height: 20px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font: 500 11px/20px var(--terminal-font-family); }
.cpu-grid, .traffic-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px 4px; text-align: center; }
.cpu-grid strong, .traffic-grid strong { display: block; font: 400 12px/19px var(--terminal-font-family); }
.cpu-grid span { color: #8e98a6; font-size: 11px; }
.memory-bars { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; text-align: center; margin-top: 10px; }
.memory-bars span { display: block; margin-bottom: 4px; }
.capacity-bar { height: 14px; background: #435877; border-radius: 4px; position: relative; overflow: hidden; text-align: center; }
.capacity-bar i { position: absolute; inset: 0 auto 0 0; background: #83acfa; }
.capacity-bar b { position: relative; font: 500 11px/14px var(--terminal-font-family); color: #f7faff; white-space: nowrap; }
.capacity-bar.swap { background: #10554f; }.capacity-bar.swap i { background: #26a69a; }
.traffic-grid { gap: 4px; }.traffic-grid span { font-size: 11px; }.traffic-grid strong { font-size: 11px; }
.tx { color: #ef8500; }.rx { color: #73a73a; }
.chart-legend { display: flex; justify-content: center; gap: 7px; margin-top: 10px; color: #939aa4; font-size: 11px; }
.chart-legend span { display: flex; gap: 4px; align-items: center; }.chart-legend i { width: 12px; height: 12px; border-radius: 4px; }
.metric-chart { display: block; width: 100%; height: 140px; margin-top: 5px; }
.network-chart small { display: block; text-align: right; color: #78828e; font-size: 10px; }
.core-row { display: flex; gap: 9px; align-items: center; height: 25px; font: 400 11px var(--terminal-font-family); }
.core-row > span:first-child { width: 43px; }.core-row > span:last-child { width: 46px; text-align: right; }
.core-ticks { flex: 1; height: 10px; background: #cdd4d9; position: relative; mask-image: repeating-linear-gradient(to right, #000 0 2px, transparent 2px 6px); }
.core-ticks i { display: block; height: 100%; background: linear-gradient(to right, #6db634 0 60%, #d3a936 60% 80%, #d75c54 80%); }
.cores-toggle { display: block; width: 100%; color: #8fadd2; margin-top: 8px; font-size: 11px; }
.table-card { padding: 10px 10px 8px; }.table-card header { display: flex; justify-content: space-between; align-items: center; padding: 0 3px 7px; }.table-card h3 { font-size: 13px; font-weight: 500; }
.table-card select { color: #d8dee5; background: #25292d; padding: 0 7px; border-bottom: 1px solid #56606a; min-width: 58px; font-size: 12px; }.table-card option { background: #25292d; }
.table-card table { width: 100%; table-layout: fixed; border-collapse: collapse; }
.table-card th { font-size: 11px; color: #8c97a5; text-align: left; font-weight: 400; height: 35px; }
.table-card td { height: 35px; font: 400 11px/18px var(--terminal-font-family); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; padding: 0 3px; }
.process-table th:first-child { width: 30%; }.process-table th:nth-child(2) { width: 24%; }.process-table th:nth-child(3) { width: 22%; }
.table-card .sorted { color: #82acff; font-weight: 700; }.name-chip { display: inline-block; max-width: 100%; padding: 0 6px; background: #3b3e40; border-radius: 10px; overflow: hidden; text-overflow: ellipsis; vertical-align: middle; }
.name-chip.teal { background: #113b37; color: #2bc5b7; }.interface-table th:first-child { width: 22%; }.interface-table th:nth-child(2) { width: 27%; }.interface-table th:nth-child(3) { width: 27%; }.interface-table td { height: 48px; }
.disk-table th:first-child { width: 24%; }.disk-table th:nth-child(2) { width: 46%; }.disk-table td { padding-right: 6px; }
.metrics-message { color: #dfb774; padding: 8px; font-size: 12px; overflow-wrap: anywhere; }.empty-metrics { padding: 10px; color: #8c97a5; }.updated-at { padding: 2px 5px 8px; font-size: 10px; color: #84909c; text-align: right; }
</style>
