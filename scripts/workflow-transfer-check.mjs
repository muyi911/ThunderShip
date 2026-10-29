import { mockWailsBindings } from "./mock-wails-bindings.mjs";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
const require = createRequire(import.meta.url);
const { build } = require("../frontend/node_modules/esbuild");
const built = await build({plugins: [mockWailsBindings],
  stdin: {
    contents:
      'export {useWorkflowTransfer} from "./src/composables/useWorkflowTransfer"; export {ref} from "vue";',
    resolveDir: fileURLToPath(new URL("../frontend", import.meta.url)),
  },
  bundle: true,
  platform: "node",
  format: "esm",
  write: false,
});
const { useWorkflowTransfer, ref } = await import(
  "data:text/javascript;base64," +
    Buffer.from(built.outputFiles[0].text).toString("base64")
);
const cfg = ref({
  servers: [{ id: "local-server", name: "server" }],
  artifacts: [
    {
      id: "local",
      name: "build",
      files: [
        { id: "folder", name: "assets", kind: "folder" },
        { id: "index", name: "index.html", kind: "file" },
      ],
    },
    {
      id: "ambiguous",
      name: "duplicates",
      files: [
        { id: "one", name: "index.html", kind: "file" },
        { id: "two", name: "index.html", kind: "file" },
      ],
    },
  ],
  workflows: [{ id: "existing" }],
});
const bundle = {
  format: "thundership-workflow",
  version: 1,
  root: "foreign-workflow",
  workflows: [{ id: "foreign-workflow", name: "deploy" }],
  combinations: [],
  servers: [{ id: "foreign-server", name: "server" }],
  artifacts: [
    {
      id: "foreign-asset",
      name: "build",
      files: [
        { id: "foreign-folder", name: "assets", kind: "folder" },
        { id: "foreign-index", name: "index.html", kind: "file" },
      ],
    },
  ],
};
const imported = [];
let cancelPicker = false,
  failImport = false,
  cancelExport = false;
globalThis.window = { chrome: { webview: {} } };
globalThis.backend = {
        ReadWorkflowImport: async () =>
          cancelPicker ? null : structuredClone(bundle),
        ImportWorkflows: async (p, bindings) => {
          imported.push({ p, bindings });
          if (failImport) throw new Error("save failed");
          return {
            ...cfg.value,
            workflows: [...cfg.value.workflows, { id: "new-workflow" }],
          };
        },
        ExportWorkflow: async () => !cancelExport,
      };
const notice = ref("");
const transfer = useWorkflowTransfer(cfg, {
  notice,
  task: async (fn) => {
    try {
      await fn();
    } catch (e) {
      notice.value = e.message;
    }
  },
});
await transfer.openWorkflowImport();
assert.equal(transfer.importReady.value, false);
assert.equal(
  transfer.importBindings.value.servers["foreign-server"],
  "",
  "matching names must not silently bind a deployment server",
);
transfer.importBindings.value.servers["foreign-server"] = "local-server";
transfer.bindImportArtifact("foreign-asset", "local");
assert.deepEqual(
  transfer.importBindings.value.artifacts["foreign-asset"].files,
  { "foreign-folder": "folder", "foreign-index": "index" },
);
assert.equal(transfer.importReady.value, true);
transfer.importBindings.value.artifacts["foreign-asset"].files[
  "foreign-folder"
] = "index";
assert.equal(
  transfer.importReady.value,
  false,
  "a file cannot satisfy a folder requirement",
);
await transfer.finishWorkflowImport();
assert.equal(imported.length, 0);
transfer.bindImportArtifact("foreign-asset", "ambiguous");
assert.deepEqual(
  transfer.importBindings.value.artifacts["foreign-asset"].files,
  {},
  "changing asset clears stale mappings and does not guess duplicate names",
);
transfer.bindImportArtifact("foreign-asset", "local");
const originalConfig = cfg.value;
failImport = true;
await transfer.finishWorkflowImport();
assert.equal(notice.value, "save failed");
assert.ok(
  transfer.importPackage.value,
  "a failed import must retain the dialog for correction",
);
assert.equal(cfg.value, originalConfig);
failImport = false;
await transfer.finishWorkflowImport();
assert.equal(cfg.value.workflows.length, 2);
assert.equal(cfg.value.workflows[0].id, "existing");
assert.equal(transfer.importPackage.value, null);
assert.equal(imported[1].bindings.servers["foreign-server"], "local-server");
assert.equal(imported[1].p.root, "foreign-workflow");
cancelPicker = true;
await transfer.openWorkflowImport();
assert.equal(transfer.importPackage.value, null);
notice.value = "unchanged";
cancelExport = true;
await transfer.exportWorkflow("existing");
assert.equal(
  notice.value,
  "unchanged",
  "canceling export must not report success",
);
console.log(
  "Workflow transfer checks passed: explicit server binding, file/folder mapping, stale and ambiguous matches, failed-save preservation, and cancellation.",
);
