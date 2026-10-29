import { mockWailsBindings } from "./mock-wails-bindings.mjs";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
const require = createRequire(import.meta.url);
const { build } = require("../frontend/node_modules/esbuild");
const built = await build({
  stdin: {
    contents:
      'export {useWorkspace} from "./src/useWorkspace"; export {createPinia} from "pinia"; export {createRenderer} from "vue";',
    resolveDir: fileURLToPath(new URL("../frontend", import.meta.url)),
  },
  bundle: true,
  platform: "node",
  format: "esm",
  write: false,
  plugins: [mockWailsBindings,
    {
      name: "theme",
      setup(build) {
        build.onResolve({ filter: /^vuetify$/ }, () => ({
          path: "theme",
          namespace: "test",
        }));
        build.onLoad({ filter: /.*/, namespace: "test" }, () => ({
          contents:
            "export const useTheme = () => ({change(){},current:{dark:true}});",
        }));
      },
    },
  ],
});
const { useWorkspace, createPinia, createRenderer } = await import(
  "data:text/javascript;base64," +
    Buffer.from(built.outputFiles[0].text).toString("base64")
);
const source = {
  id: "original",
  name: "部署前端",
  version: 7,
  servers: ["server"],
  concurrency: 4,
  resources: [],
  steps: [
    {
      id: "upload",
      type: "upload",
      artifactId: "artifact",
      files: [
        {
          id: "file",
          local: "D:/dist/index.html",
          remote: "/front/index.html",
        },
      ],
    },
  ],
  flow: { nodes: [{ id: "upload", ref: "upload", x: 10, y: 20 }], edges: [] },
};
const combination = {
  id: "combined",
  name: "发布全部",
  root: {
    mode: "parallel",
    workflowId: "",
    children: [{ workflowId: source.id, mode: "", children: [] }],
  },
};
let cfg = {
  servers: [],
  artifacts: [],
  providers: [],
  concurrency: 4,
  theme: "dark",
  workflows: [
    source,
    { ...source, id: "existing-copy", name: source.name + " - 副本" },
  ],
  combinations: [combination],
};
const snapshot = JSON.stringify(cfg);
const saved = [];
const runCalls = [];
let inputs = [];
let startError = false;
globalThis.window = { chrome: { webview: {} } };
globalThis.backend = {
        GetConfig: async () => structuredClone(cfg),
        ListRuns: async () => [],
        RunEvents: async () => [],
        RunInputs: async () => structuredClone(inputs),
        Preflight: async () => [],
        StartRun: async (node, name) => {
          runCalls.push({ node, name });
          if (startError) throw new Error("启动失败");
          return "run-1";
        },
        SaveWorkflow: async (draft) => {
          saved.push(draft);
          cfg = {
            ...cfg,
            workflows: [
              ...cfg.workflows,
              { ...structuredClone(draft), id: "new-copy", version: 1 },
            ],
          };
          return structuredClone(cfg);
        },
      };
let workspace;
const renderer = createRenderer({
  createElement: () => ({}),
  createComment: () => ({}),
  createText: () => ({}),
  insert() {},
  remove() {},
  setElementText() {},
  setText() {},
  patchProp() {},
  parentNode: () => null,
  nextSibling: () => null,
});
const app = renderer.createApp({
  setup() {
    workspace = useWorkspace();
    return () => null;
  },
});
app.use(createPinia());
app.mount({});
try {
  await new Promise((resolve) => setImmediate(resolve));
  workspace.duplicateWorkflow(workspace.cfg.workflows[0]);
  assert.equal(workspace.wf.id, "");
  assert.equal(workspace.wf.version, 0);
  assert.equal(workspace.wf.name, "部署前端 - 副本 2");
  assert.equal(
    workspace.dirty,
    true,
    "an unsaved copy must be protected immediately",
  );
  workspace.wf.steps[0].files[0].remote = "/different/index.html";
  assert.equal(
    JSON.stringify(cfg),
    snapshot,
    "editing the copy must not mutate saved workflows or assets",
  );
  const draft = workspace.wf;
  workspace.duplicateWorkflow(workspace.cfg.workflows[0]);
  assert.equal(workspace.wf, draft, "copying must not overwrite a dirty draft");
  assert.equal(typeof workspace.pendingDraft, "function");
  workspace.pendingDraft = null;
  assert.equal(workspace.wf, draft, "canceling must keep the draft");
  workspace.duplicateWorkflow(workspace.cfg.workflows[0]);
  workspace.discardDraft();
  assert.notEqual(workspace.wf, draft);
  assert.equal(workspace.wf.steps[0].files[0].remote, "/front/index.html");
  workspace.duplicateWorkflow(workspace.cfg.combinations[0]);
  assert.equal(workspace.cmb.id, "");
  assert.equal(workspace.cmb.name, "发布全部 - 副本");
  assert.equal(workspace.editorKind, "reference");
  assert.equal(workspace.dirty, true);
  assert.ok(
    workspace.cmb.root.graph.nodes.some((node) => node.ref === "original"),
    "copies must retain existing workflow references",
  );
  workspace.cmb.root.graph.nodes[0].x = 999;
  assert.equal(JSON.stringify(cfg), snapshot);
  assert.equal(
    saved.length,
    0,
    "copying opens a draft without saving or running",
  );
  workspace.resumeDraft("workflow");
  await workspace.saveWorkflow();
  assert.equal(saved[0].id, "");
  assert.equal(saved[0].version, 0);
  assert.equal(workspace.wf.id, "new-copy");
  assert.equal(workspace.dirty, false);
  assert.equal(
    workspace.cfg.workflows[0].version,
    7,
    "saving a copy must not replace or version the source",
  );
  const root = { workflowId: source.id, mode: "", children: [] };
  await workspace.execute(false, root, source.name);
  assert.equal(workspace.pendingRun.name, source.name);
  assert.equal(runCalls.length, 0, "clicking run must only open confirmation");
  root.workflowId = "changed-after-click";
  assert.equal(workspace.pendingRun.node.workflowId, source.id);
  workspace.pendingRun = null;
  await workspace.confirmRun();
  assert.equal(runCalls.length, 0, "canceling must never start a run");

  await workspace.execute(true, { ...root, workflowId: source.id });
  assert.deepEqual(workspace.preflight, []);
  assert.equal(
    workspace.pendingRun,
    null,
    "preflight needs no run confirmation",
  );
  assert.equal(runCalls.length, 0);

  await workspace.execute(
    false,
    { ...root, workflowId: source.id },
    source.name,
  );
  await Promise.all([workspace.confirmRun(), workspace.confirmRun()]);
  assert.equal(runCalls.length, 1, "double confirmation must not start twice");
  assert.equal(runCalls[0].name, source.name);
  assert.equal(runCalls[0].node.workflowId, source.id);
  assert.equal(workspace.pendingRun, null);
  assert.equal(workspace.logsOpen, true);

  await workspace.execute(false, combination.root, combination.name);
  assert.equal(workspace.pendingRun.name, combination.name);
  assert.equal(runCalls.length, 1, "combinations also require confirmation");
  await workspace.confirmRun();
  assert.equal(runCalls.length, 2);

  workspace.selected = [source.id, combination.id];
  workspace.quickMode = "parallel";
  await workspace.quickRun();
  assert.equal(workspace.pendingRun.node.mode, "parallel");
  assert.deepEqual(
    workspace.pendingRun.node.children.map((node) => node.workflowId),
    [source.id, combination.id],
  );
  assert.equal(runCalls.length, 2, "quick runs also require confirmation");
  workspace.pendingRun = null;

  inputs = [{ error: "文件不存在" }];
  await workspace.execute(false, combination.root, combination.name);
  await workspace.confirmRun();
  assert.equal(
    runCalls.length,
    2,
    "missing artifacts must still block execution",
  );
  assert.equal(workspace.missingInputs.length, 1);
  inputs = [];
  startError = true;
  await workspace.execute(false, combination.root, combination.name);
  await workspace.confirmRun();
  assert.equal(workspace.notice, "启动失败");
  assert.equal(workspace.pendingRun, null);
  assert.equal(workspace.busy, false);
} finally {
  app.unmount();
}
console.log(
  "Workflow copy and run confirmation checks passed: independent drafts, cancellation, single execution, combinations, quick runs, and artifact validation.",
);
