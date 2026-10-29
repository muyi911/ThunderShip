import { mockWailsBindings } from "./mock-wails-bindings.mjs";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
const require = createRequire(import.meta.url);
const { build } = require("../frontend/node_modules/esbuild");
const { parse, compileScript } = require("../frontend/node_modules/@vue/compiler-sfc");
const built = await build({
  stdin: { contents: 'export {default as TunnelManager} from "./src/TunnelManager.vue"; export {createRenderer,h,reactive,nextTick} from "vue";', resolveDir: fileURLToPath(new URL("../frontend", import.meta.url)) },
  bundle: true, platform: "node", format: "esm", write: false,
  plugins: [mockWailsBindings,{ name: "scripts", setup(build) {
    build.onLoad({ filter: /\.vue$/ }, async ({ path }) => {
      if (!path.endsWith("TunnelManager.vue")) return { contents: "export default {}", loader: "js" };
      const { descriptor } = parse(await readFile(path, "utf8"));
      return { contents: compileScript(descriptor, { id: path, genDefaultAs: "Component" }).content + "\nexport default Component;", loader: "ts" };
    });
  } }],
});
const { TunnelManager, createRenderer, h, reactive, nextTick } = await import("data:text/javascript;base64," + Buffer.from(built.outputFiles[0].text).toString("base64"));
let rules = [], statuses = [], failSave = false;
const calls = [];
globalThis.window = { chrome: { webview: {} } };
globalThis.backend = {
  async GetTunnelRules(server) { calls.push(["get", server]); return structuredClone(rules); },
  async ListTunnels(id) { calls.push(["list", id]); return structuredClone(statuses); },
  async SaveTunnelRule(server, rule) {
    calls.push(["save", server, rule]);
    if (failSave) throw new Error("disk full");
    const saved = { ...rule, id: rule.id || "rule-1" };
    rules = [...rules.filter((r) => r.id !== saved.id), saved]; statuses = [];
    return structuredClone(rules);
  },
  async DeleteTunnelRule(server, id) { calls.push(["delete", server, id]); rules = rules.filter((r) => r.id !== id); statuses = []; return structuredClone(rules); },
  async StartTunnel(id, rule) { calls.push(["start", id, rule]); statuses = [{ id: rule, running: true, connections: 0, error: "" }]; return statuses[0]; },
  async StopTunnel(id, rule) { calls.push(["stop", id, rule]); statuses = []; },
};
let state;
const renderer = createRenderer({ createComment: () => ({}), insert() {}, remove() {}, parentNode: () => null, nextSibling: () => null });
const props = reactive({ modelValue: false, connection: "c1", serverId: "s1", serverName: "服务器", closed: false });
const component = { ...TunnelManager, setup(p, c) { state = TunnelManager.setup(p, c); return () => null; } };
const app = renderer.createApp({ render: () => h(component, props) });
app.mount({});
const flush = async () => { await nextTick(); await new Promise((r) => setTimeout(r, 0)); };
assert.equal(calls.length, 0, "hidden manager should not poll");
props.modelValue = true; await flush();
assert.equal(state.editor.value.bindHost, "127.0.0.1");
assert.equal(state.editor.value.bindPort, 8800);
assert.equal(calls.some(([kind]) => kind === "start"), false, "opening manager must not start a tunnel");
failSave = true; await state.save();
assert.match(state.error.value, /disk full/);
assert.equal(state.editor.value.targetPort, 8800, "failed save lost draft");
failSave = false; await state.save();
assert.equal(state.rules.value.length, 1);
assert.equal(state.editor.value, null);
await state.toggle(state.rules.value[0]);
assert.equal(state.status("rule-1").running, true);
props.modelValue = false; await flush();
assert.equal(calls.some(([kind]) => kind === "stop"), false, "closing manager stopped forwarding");
props.modelValue = true; await flush();
await state.toggle(state.rules.value[0]);
assert.equal(calls.filter(([kind]) => kind === "stop").length, 1);
state.edit(state.rules.value[0]); state.editor.value.targetPort = 9900;
assert.equal(state.rules.value[0].targetPort, 8800, "editing mutated saved rule");
await state.save();
assert.equal(state.rules.value[0].targetPort, 9900);
props.closed = true; await flush();
const starts = calls.filter(([kind]) => kind === "start").length;
await state.toggle(state.rules.value[0]);
assert.equal(calls.filter(([kind]) => kind === "start").length, starts, "closed connection started a tunnel");
state.deleting.value = "rule-1";
assert.equal(calls.some(([kind]) => kind === "delete"), false, "delete must wait for confirmation");
await state.remove();
assert.equal(state.rules.value.length, 0);
assert.equal(state.deleting.value, "");
app.unmount();
console.log("PASS tunnel configuration, failure drafts, start/stop, closed connection and delete confirmation");
