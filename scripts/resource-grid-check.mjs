import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
const require = createRequire(import.meta.url);
const { build } = require("../frontend/node_modules/esbuild");
const {
  parse,
  compileScript,
} = require("../frontend/node_modules/@vue/compiler-sfc");
const { descriptor } = parse(
  await readFile(
    new URL("../frontend/src/ResourceGrid.vue", import.meta.url),
    "utf8",
  ),
);
const script = compileScript(descriptor, {
  id: "grid-check",
  genDefaultAs: "ResourceGrid",
});
const built = await build({
  stdin: {
    contents:
      script.content +
      '\nexport { ResourceGrid }; export {orderedResources} from "./src/resourceOrder";',
    resolveDir: fileURLToPath(new URL("../frontend", import.meta.url)),
    loader: "ts",
  },
  bundle: true,
  platform: "node",
  format: "esm",
  write: false,
});
const { ResourceGrid, orderedResources } = await import(
  "data:text/javascript;base64," +
    Buffer.from(built.outputFiles[0].text).toString("base64")
);
const items = ["a", "b", "c"].map((id) => ({ id, name: id }));
assert.deepEqual(
  orderedResources(items, ["deleted", "c", "a"]).map((i) => i.id),
  ["c", "a", "b"],
);
assert.deepEqual(
  items.map((i) => i.id),
  ["a", "b", "c"],
  "sorting must not mutate resource data",
);
const props = { items, icon: "mdi-server", busy: false };
const calls = [];
const grid = ResourceGrid.setup(props, {
  expose() {},
  emit: (...args) => calls.push(args),
});
let prevented = 0;
const event = (x = 10) => ({
  clientX: x,
  preventDefault() {
    prevented++;
  },
  target: {
    closest() {
      return {};
    },
  },
  currentTarget: {
    getBoundingClientRect() {
      return { left: 0, width: 100 };
    },
    contains() {
      return true;
    },
  },
  dataTransfer: { setData() {}, setDragImage() {} },
});
grid.drop(event(), "a");
assert.equal(calls.length, 0, "external drags must not reorder resources");
grid.start(event(), "c");
grid.hover(event(), "a");
assert.equal(grid.over.value, "a");
grid.leave({ ...event(), relatedTarget: {} }, "a");
assert.equal(
  grid.over.value,
  "a",
  "crossing child elements must retain the drop marker",
);
grid.drop(event(), "a");
assert.deepEqual(calls.pop(), ["move", "c", "a"]);
assert.equal(grid.dragged.value, "");
grid.start(event(), "a");
grid.drop(event(90), "c");
assert.deepEqual(calls.pop(), ["move", "a", ""]);
grid.start(event(), "a");
grid.drop(event(90), "b");
assert.deepEqual(calls.pop(), ["move", "a", "c"]);
grid.start(event(), "b");
grid.drop(event(), "b");
assert.equal(calls.length, 0, "dropping onto itself must not save");
grid.reset();
grid.keyMove({ ...event(), altKey: true, key: "ArrowLeft" }, "c");
assert.deepEqual(calls.pop(), ["move", "c", "b"]);
grid.keyMove({ ...event(), altKey: true, key: "ArrowRight" }, "b");
assert.deepEqual(calls.pop(), ["move", "b", ""]);
grid.keyMove({ ...event(), altKey: true, key: "ArrowLeft" }, "a");
assert.equal(calls.length, 0);
props.busy = true;
grid.start(event(), "c");
grid.drop(event(), "a");
grid.keyMove({ ...event(), altKey: true, key: "ArrowLeft" }, "c");
assert.equal(calls.length, 0, "saving must disable concurrent moves");
assert.ok(prevented > 0);
console.log(
  "Resource grid checks passed: drag placement, keyboard sorting, busy/external-drag guards, and stable saved ordering.",
);
