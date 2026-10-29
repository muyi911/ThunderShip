import { onMounted, onUnmounted, ref, shallowRef, toValue, watch, type MaybeRefOrGetter } from "vue";
import { api } from "../api";
import type { ServerMetrics } from "../types";

type CPU = ServerMetrics["cpu"];
export type CPUUsage = { usage: number; user: number; system: number; iowait: number };
export type MetricSample = { timestamp: number; cpu: number | null; memory: number | null; send: number | null; receive: number | null };

function cpuUsage(current: CPU, previous: CPU | undefined): CPUUsage | null {
  if (!previous) return null;
  const total = current.total - previous.total;
  const deltas = (["user", "system", "idle", "iowait"] as const).map((key) => current[key] - previous[key]);
  if (!Number.isFinite(total) || total <= 0 || deltas.some((value) => !Number.isFinite(value) || value < 0)) return null;
  const [user, system, idle, iowait] = deltas;
  if (idle + iowait > total) return null;
  const percent = (value: number) => Math.min(100, Math.max(0, value / total * 100));
  return { usage: percent(total - idle - iowait), user: percent(user), system: percent(system), iowait: percent(iowait) };
}

export function computeMetrics(current: ServerMetrics, previous: ServerMetrics | null) {
  const elapsed = previous ? (current.timestamp - previous.timestamp) / 1000 : 0;
  const validPrevious = Number.isFinite(elapsed) && elapsed > 0 ? previous : null;
  const previousCores = new Map(validPrevious?.cores.map((core) => [core.name, core]));
  const previousNetworks = new Map(validPrevious?.networks.map((network) => [network.name, network]));
  const rate = (value: number, before: number | undefined) => before !== undefined && Number.isFinite(elapsed) && elapsed > 0 && value >= before ? (value - before) / elapsed : null;
  const networks = current.networks.map((network) => {
    const before = previousNetworks.get(network.name);
    return { ...network, receiveRate: rate(network.received, before?.received), sendRate: rate(network.sent, before?.sent) };
  });
  // Virtual bridge and container counters repeat traffic already counted at the host interface.
  const mainNetworks = networks.filter((network) => !/^(lo$|veth|docker|br-)/.test(network.name));
  const totalRate = (key: "sendRate" | "receiveRate") => mainNetworks.length && mainNetworks.every((network) => network[key] !== null)
    ? mainNetworks.reduce((sum, network) => sum + network[key]!, 0) : null;
  const memoryUsed = Math.max(0, current.memory.total - current.memory.available);
  const swapUsed = Math.max(0, current.memory.swapTotal - current.memory.swapFree);
  const deviceDisks = current.disks.filter((disk) => disk.source.startsWith("/dev/"));
  const disks = [...new Map((deviceDisks.length ? deviceDisks : current.disks.filter((disk) => disk.mount === "/")).map((disk) => [disk.source, disk])).values()];
  return {
    cpu: cpuUsage(current.cpu, validPrevious?.cpu),
    cores: current.cores.map((core) => ({ name: core.name, usage: cpuUsage(core, previousCores.get(core.name))?.usage ?? null })),
    networks,
    network: {
      received: mainNetworks.reduce((sum, network) => sum + network.received, 0),
      sent: mainNetworks.reduce((sum, network) => sum + network.sent, 0),
      receiveRate: totalRate("receiveRate"),
      sendRate: totalRate("sendRate"),
    },
    memoryUsed,
    memoryPercent: current.memory.total > 0 ? Math.min(100, memoryUsed / current.memory.total * 100) : null,
    swapUsed,
    swapPercent: current.memory.swapTotal > 0 ? Math.min(100, swapUsed / current.memory.swapTotal * 100) : null,
    diskTotal: disks.reduce((sum, disk) => sum + disk.total, 0),
    diskAvailable: disks.reduce((sum, disk) => sum + disk.available, 0),
  };
}

export type CalculatedMetrics = ReturnType<typeof computeMetrics>;

export function useServerMetrics(connection: MaybeRefOrGetter<string>, enabled: MaybeRefOrGetter<boolean>) {
  const data = shallowRef<ServerMetrics | null>(null);
  const metrics = shallowRef<CalculatedMetrics | null>(null);
  const samples = ref<MetricSample[]>([]);
  const loading = ref(false);
  const error = ref("");
  const visible = ref(document.visibilityState !== "hidden");
  let previous: ServerMetrics | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let generation = 0, inFlight = false, rerun = false, disposed = false;
  const allowed = () => !disposed && !!toValue(connection) && toValue(enabled) && visible.value;
  function clearTimer() {
    clearTimeout(timer);
    timer = undefined;
  }
  async function refresh() {
    clearTimer();
    if (!allowed()) return;
    if (inFlight) { rerun = true; return; }
    inFlight = true;
    loading.value = true;
    const id = toValue(connection), requestGeneration = generation;
    try {
      const snapshot = await api("ReadServerMetrics", id);
      if (requestGeneration !== generation || !allowed()) return;
      const calculated = computeMetrics(snapshot, previous);
      data.value = snapshot;
      metrics.value = calculated;
      previous = snapshot;
      samples.value = [...samples.value.slice(-59), { timestamp: snapshot.timestamp, cpu: calculated.cpu?.usage ?? null, memory: calculated.memoryPercent, send: calculated.network.sendRate, receive: calculated.network.receiveRate }];
      error.value = "";
    } catch (cause) {
      if (requestGeneration !== generation || !allowed()) return;
      error.value = String(cause instanceof Error ? cause.message : cause);
      previous = null;
    } finally {
      inFlight = false;
      if (requestGeneration === generation) loading.value = false;
      if (allowed()) {
        if (rerun) { rerun = false; void refresh(); }
        else timer = setTimeout(refresh, 3000);
      }
    }
  }
  watch(() => [toValue(connection), toValue(enabled), visible.value] as const, (current, old) => {
    generation++;
    clearTimer();
    previous = null;
    rerun = false;
    loading.value = false;
    if (!old || current[0] !== old[0]) {
      data.value = null;
      metrics.value = null;
      samples.value = [];
      error.value = "";
    }
    if (allowed()) void refresh();
  }, { immediate: true, flush: "sync" });
  const visibilityChanged = () => { visible.value = document.visibilityState !== "hidden"; };
  onMounted(() => { document.addEventListener("visibilitychange", visibilityChanged); visibilityChanged(); });
  onUnmounted(() => {
    disposed = true;
    generation++;
    clearTimer();
    document.removeEventListener("visibilitychange", visibilityChanged);
  });
  return { data, metrics, samples, loading, error, refresh };
}
