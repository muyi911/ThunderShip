import { mockWailsBindings } from "./mock-wails-bindings.mjs";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { build } = require("../frontend/node_modules/esbuild");
const bundle = await build({plugins: [mockWailsBindings],
  stdin: {
    contents: 'export {computeMetrics,useServerMetrics} from "./src/composables/useServerMetrics"; export {createRenderer,ref,nextTick} from "vue";',
    resolveDir: fileURLToPath(new URL("../frontend", import.meta.url)),
  },
  bundle: true, platform: "node", format: "esm", write: false,
});
const { computeMetrics, useServerMetrics, createRenderer, ref, nextTick } = await import("data:text/javascript;base64," + Buffer.from(bundle.outputFiles[0].text).toString("base64"));

function snapshot(step = 0) {
  const cpu = { name: "cpu", user: 100 + step * 40, system: 50 + step * 20, idle: 300 + step * 120, iowait: 50 + step * 20, total: 500 + step * 200 };
  return {
    timestamp: 1000 + step * 3000, hostname: "fixture", os: "Linux", uptime: 500 + step * 3, cpuCount: 1,
    cpu, cores: [{ ...cpu, name: "cpu0" }],
    memory: { total: 4096, available: 1024, swapTotal: 1024, swapFree: 512 },
    networks: [
      { name: "eth0", received: 1000 + step * 600, sent: 2000 + step * 300, addresses: ["127.0.0.1"] },
      ...["lo", "veth1", "br-123", "docker0"].map((name) => ({ name, received: 10000 + step * 90000, sent: 20000 + step * 90000, addresses: [] })),
    ],
    disks: [{ source: "/dev/vda", mount: "/", total: 20000, available: 5000 }],
    processes: [{ pid: 10, name: "test", cpu: 10, memory: 512 }], warnings: [],
  };
}
const first = snapshot(), second = snapshot(1);
const baseline = computeMetrics(first, null);
assert.equal(baseline.cpu, null);
assert.equal(baseline.cores[0].usage, null);
assert.equal(baseline.network.sendRate, null);
const delta = computeMetrics(second, first);
assert.deepEqual(delta.cpu, { usage: 30, user: 20, system: 10, iowait: 10 });
assert.equal(delta.cores[0].usage, 30);
assert.deepEqual(delta.network, { received: 1600, sent: 2300, receiveRate: 200, sendRate: 100 });
assert.equal(delta.networks.length, 5, "virtual interfaces remain visible in detail");
assert.equal(delta.memoryUsed, 3072);
assert.equal(delta.memoryPercent, 75);
assert.equal(delta.swapPercent, 50);
const duplicateDisks = snapshot();
duplicateDisks.disks.push({ ...duplicateDisks.disks[0], mount: "/alias" }, { source: "tmpfs", mount: "/dev/shm", total: 2000, available: 1000 });
assert.equal(computeMetrics(duplicateDisks, null).diskTotal, 20000, "disk rail excludes RAM disks and duplicate mounts");
duplicateDisks.disks = [{ source: "overlay", mount: "/", total: 20000, available: 5000 }, { source: "tmpfs", mount: "/dev/shm", total: 2000, available: 1000 }];
assert.equal(computeMetrics(duplicateDisks, null).diskAvailable, 5000, "container root mount is the fallback");
for (const timestamp of [0, first.timestamp, Infinity]) {
  const result = computeMetrics({ ...second, timestamp }, first);
  assert.equal(result.cpu, null);
  assert.equal(result.network.sendRate, null);
}
const reset = snapshot(1);
reset.cpu.user = 1;
reset.cores[0].idle = 1;
reset.networks[0].sent = 1;
assert.equal(computeMetrics(reset, first).cpu, null);
assert.equal(computeMetrics(reset, first).cores[0].usage, null);
assert.equal(computeMetrics(reset, first).network.sendRate, null);
const added = snapshot(1);
added.networks.push({ name: "ens4", received: 200, sent: 50, addresses: [] });
assert.equal(computeMetrics(added, first).network.receiveRate, null, "new interface needs its own baseline");
const noMemory = snapshot(1);
noMemory.memory.total = noMemory.memory.swapTotal = 0;
assert.equal(computeMetrics(noMemory, first).memoryPercent, null);
assert.equal(computeMetrics(noMemory, first).swapPercent, null);

const calls = [], timers = new Map(), listeners = new Map();
let timerID = 0;
const realSetTimeout = globalThis.setTimeout, realClearTimeout = globalThis.clearTimeout;
globalThis.setTimeout = (callback, delay) => { const id = ++timerID; timers.set(id, { callback, delay }); return id; };
globalThis.clearTimeout = (id) => timers.delete(id);
globalThis.document = {
  visibilityState: "visible",
  addEventListener: (name, callback) => listeners.set(name, callback),
  removeEventListener: (name, callback) => { if (listeners.get(name) === callback) listeners.delete(name); },
};
globalThis.window = { chrome: { webview: {} } };
globalThis.backend = {
  ReadServerMetrics: (connection) => new Promise((resolve, reject) => calls.push({ connection, resolve, reject })),
};
const renderer = createRenderer({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  insert() {}, remove() {}, setText() {}, setElementText() {}, patchProp() {},
  parentNode: () => null, nextSibling: () => null,
});
const connection = ref("a"), enabled = ref(true);
let state;
const app = renderer.createApp({ setup() { state = useServerMetrics(connection, enabled); return () => null; } });
const flush = async () => { for (let i = 0; i < 5; i++) await Promise.resolve(); await nextTick(); };
const resolveLast = async (value) => { calls.at(-1).resolve(value); await flush(); };
const tick = () => {
  assert.equal(timers.size, 1, "exactly one next poll is scheduled");
  const [id, timer] = [...timers][0];
  assert.equal(timer.delay, 3000);
  timers.delete(id);
  void timer.callback();
};
try {
  app.mount({});
  assert.equal(calls.length, 1);
  void state.refresh();
  void state.refresh();
  assert.equal(calls.length, 1, "manual refresh never overlaps a request");
  await resolveLast(first);
  assert.equal(calls.length, 2, "refreshes during a request coalesce into one retry");
  await resolveLast(second);
  assert.equal(state.metrics.value.cpu.usage, 30);
  assert.equal(state.samples.value.length, 2);
  tick();
  enabled.value = false;
  await resolveLast(snapshot(2));
  assert.equal(state.data.value.timestamp, second.timestamp, "inactive tab discards its in-flight response");
  assert.equal(timers.size, 0);
  enabled.value = true;
  assert.equal(calls.length, 4, "reactivating polls immediately");
  await resolveLast(snapshot(3));
  assert.equal(state.metrics.value.cpu, null, "resuming starts a fresh rate baseline");
  document.visibilityState = "hidden";
  listeners.get("visibilitychange")();
  assert.equal(timers.size, 0);
  void state.refresh();
  assert.equal(calls.length, 4, "hidden windows do not poll, including manual refresh");
  document.visibilityState = "visible";
  listeners.get("visibilitychange")();
  assert.equal(calls.length, 5);
  connection.value = "b";
  assert.equal(state.data.value, null);
  assert.equal(state.samples.value.length, 0);
  assert.equal(calls.length, 5, "connection switch waits for the prior in-flight poll");
  await resolveLast(snapshot(4));
  assert.equal(calls.length, 6);
  assert.equal(calls.at(-1).connection, "b");
  assert.equal(state.data.value, null, "old connection response cannot replace new connection data");
  await resolveLast(snapshot(5));
  tick();
  calls.at(-1).reject(new Error("channel unavailable"));
  await flush();
  assert.equal(state.error.value, "channel unavailable");
  assert.equal(state.data.value.timestamp, snapshot(5).timestamp, "errors retain the last real snapshot");
  tick();
  await resolveLast(snapshot(6));
  assert.equal(state.error.value, "");
  assert.equal(state.metrics.value.cpu, null);
  for (let step = 7; step < 70; step++) { tick(); await resolveLast(snapshot(step)); }
  assert.equal(state.samples.value.length, 60, "chart history remains bounded");
  tick();
  const lastTimestamp = state.data.value.timestamp;
  app.unmount();
  await resolveLast(snapshot(70));
  assert.equal(state.data.value.timestamp, lastTimestamp);
  assert.equal(timers.size, 0);
  assert.equal(listeners.size, 0);
  console.log("server metrics: counter deltas, virtual-interface totals, pause/reconnect/unmount guards, errors and bounded history passed");
} finally {
  app.unmount();
  globalThis.setTimeout = realSetTimeout;
  globalThis.clearTimeout = realClearTimeout;
}
