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
const built = await build({
  stdin: {
    contents:
      'export {default as ResizablePanel} from "./src/ResizablePanel.vue"; export {default as CodeEditorDialog} from "./src/CodeEditorDialog.vue"; export {default as CommandEditor} from "./src/CommandEditor.vue"; export {createRenderer, h, reactive, nextTick} from "vue";',
    resolveDir: fileURLToPath(new URL("../frontend", import.meta.url)),
  },
  bundle: true,
  platform: "node",
  format: "esm",
  write: false,
  plugins: [
    {
      name: "component-scripts",
      setup(build) {
        build.onLoad({ filter: /\.vue$/ }, async ({ path }) => {
          if (path.endsWith("IconButton.vue"))
            return { contents: "export default {};", loader: "js" };
          const { descriptor } = parse(await readFile(path, "utf8"));
          const script = compileScript(descriptor, {
            id: path,
            genDefaultAs: "Component",
          });
          return {
            contents: script.content + "\nexport default Component;",
            loader: "ts",
          };
        });
      },
    },
  ],
});
const {
  ResizablePanel,
  CodeEditorDialog,
  CommandEditor,
  createRenderer,
  h,
  reactive,
  nextTick,
} = await import(
  "data:text/javascript;base64," +
    Buffer.from(built.outputFiles[0].text).toString("base64")
);
const renderer = createRenderer({
  createComment: () => ({}),
  insert() {},
  remove() {},
  parentNode: () => null,
  nextSibling: () => null,
});
function mount(component, values, initialize = () => {}) {
  const props = reactive(values);
  let state;
  const wrapper = {
    ...component,
    setup(p, context) {
      state = component.setup(p, context);
      initialize(state);
      return () => null;
    },
  };
  const app = renderer.createApp({ render: () => h(wrapper, props) });
  app.mount({});
  return { props, state, close: () => app.unmount() };
}
const saved = new Map([["width", "620"]]);
globalThis.localStorage = {
  getItem: (k) => saved.get(k) ?? null,
  setItem: (k, v) => saved.set(k, v),
};
let notifySize,
  disconnected = false,
  parentWidth = 1200;
globalThis.ResizeObserver = class {
  constructor(callback) {
    notifySize = callback;
  }
  observe() {}
  disconnect() {
    disconnected = true;
  }
};
const resized = mount(
  ResizablePanel,
  { label: "节点配置", storageKey: "width", defaultSize: 350 },
  (s) => {
    s.panel.value = {
      parentElement: { getBoundingClientRect: () => ({ width: parentWidth }) },
    };
  },
);
const size = resized.state;
assert.equal(size.size.value, 620, "restore the last selected width");
let captured = false,
  prevented = 0;
const event = (x, key) => ({
  pointerId: 1,
  button: 0,
  clientX: x,
  key,
  preventDefault() {
    prevented++;
  },
  currentTarget: {
    focus() {},
    setPointerCapture() {
      captured = true;
    },
    hasPointerCapture() {
      return captured;
    },
    releasePointerCapture() {
      captured = false;
    },
  },
});
size.start(event(800));
size.move(event(600));
assert.equal(
  size.size.value,
  820,
  "moving the left border left must widen the panel",
);
size.move({ ...event(500), pointerId: 2 });
assert.equal(size.size.value, 820, "ignore other pointers");
size.move(event(-200));
assert.equal(size.size.value, 900, "leave space for the adjacent editor");
size.finish(event(-200));
assert.equal(captured, false);
assert.equal(saved.get("width"), "900");
parentWidth = 400;
notifySize();
assert.equal(size.size.value, 300, "clamp on window or sibling panel resize");
parentWidth = 1200;
notifySize();
assert.equal(
  size.size.value,
  900,
  "temporary narrowing must not discard the preference",
);
size.keyResize(event(0, "Home"));
assert.equal(size.size.value, 280);
size.keyResize(event(0, "ArrowLeft"));
assert.equal(size.size.value, 300);
size.keyResize(event(0, "ArrowRight"));
assert.equal(size.size.value, 280);
size.keyResize(event(0, "End"));
assert.equal(size.size.value, 900);
size.reset();
assert.equal(size.size.value, 350);
parentWidth = 200;
notifySize();
assert.equal(
  size.size.value,
  150,
  "narrow layouts must not overflow the parent",
);
assert.ok(prevented >= 5);
resized.close();
assert.equal(disconnected, true);
let parentHeight = 900;
disconnected = false;
saved.set("height", "480");
const logPanel = mount(ResizablePanel, {
  label: "执行日志", storageKey: "height", axis: "height", defaultSize: 240,
  minSize: 120, maxSize: 1200, maxRatio: .8,
}, (s) => { s.panel.value = { parentElement: { getBoundingClientRect: () => ({ height: parentHeight }) } }; });
const height = logPanel.state;
const verticalEvent = (y, key) => ({ ...event(800, key), clientY: y });
assert.equal(height.size.value, 480, "restore log height independently of panel widths");
height.start(verticalEvent(600));
height.move(verticalEvent(400));
assert.equal(height.size.value, 680, "dragging the top edge upward must enlarge the log");
height.move({ ...verticalEvent(400), clientX: 100 });
assert.equal(height.size.value, 680, "horizontal movement must not change log height");
height.move(verticalEvent(-1000));
assert.equal(height.size.value, 720, "leave space for workspace content");
height.finish(verticalEvent(-1000));
assert.equal(saved.get("height"), "720");
parentHeight = 200; notifySize();
assert.equal(height.size.value, 160, "log must fit when window becomes smaller");
parentHeight = 900; notifySize();
assert.equal(height.size.value, 720, "restore preferred height after window expands");
height.keyResize(verticalEvent(0, "Home"));
assert.equal(height.size.value, 120);
height.keyResize(verticalEvent(0, "ArrowUp"));
assert.equal(height.size.value, 140);
height.keyResize(verticalEvent(0, "ArrowDown"));
assert.equal(height.size.value, 120);
height.keyResize(verticalEvent(0, "ArrowLeft"));
assert.equal(height.size.value, 120, "height handle must ignore horizontal keys");
height.reset();
assert.equal(height.size.value, 240);
assert.equal(saved.get("width"), "350", "log resizing must not overwrite panel width preferences");
logPanel.close();
assert.equal(disconnected, true);
globalThis.localStorage = {
  getItem() {
    throw Error("blocked");
  },
  setItem() {
    throw Error("blocked");
  },
};
parentWidth = 1200;
const blocked = mount(ResizablePanel, {
  label: "AI",
  storageKey: "blocked",
  defaultSize: 340,
});
blocked.state.reset();
assert.equal(blocked.state.size.value, 340);
blocked.close();

const script = 'set -eu\nprintf "%s\\n" "a very long command"\n';
const changes = [],
  closes = [];
const dialog = mount(CodeEditorDialog, {
  open: false,
  modelValue: script,
  title: "编辑脚本",
  "onUpdate:modelValue": (text) => changes.push(text),
  "onUpdate:open": (open) => closes.push(open),
});
dialog.props.open = true;
await nextTick();
assert.equal(dialog.state.draft.value, script);
dialog.state.draft.value = "discard these changes";
dialog.props.open = false;
await nextTick();
assert.deepEqual(changes, [], "closing the popup must not modify the node");
dialog.props.open = true;
await nextTick();
assert.equal(
  dialog.state.draft.value,
  script,
  "reopening must discard the canceled buffer",
);
dialog.state.wrap.value = true;
assert.equal(
  dialog.state.draft.value,
  script,
  "visual wrapping must never insert newlines",
);
const edited = script + 'echo "完成"\n';
dialog.state.draft.value = edited;
dialog.state.apply();
assert.deepEqual(changes, [edited]);
assert.deepEqual(closes, [false]);
dialog.close();

const remoteSaves = [], remoteCloses = [];
const remoteDialog = mount(CodeEditorDialog, {
  open: true, modelValue: "PORT=80\n", title: "编辑文件", saveLabel: "保存到服务器",
  "onSave": text => remoteSaves.push(text),
  "onUpdate:open": open => remoteCloses.push(open),
});
remoteDialog.state.draft.value = "PORT=81\n";
remoteDialog.state.apply();
assert.deepEqual(remoteSaves, ["PORT=81\n"]);
assert.deepEqual(remoteCloses, [], "save closed before server confirmed");
remoteDialog.props.error = "conflict";
await nextTick();
assert.equal(remoteDialog.state.draft.value, "PORT=81\n", "save error lost draft");
remoteDialog.state.close();
assert.equal(remoteDialog.state.discard.value, true);
assert.deepEqual(remoteCloses, [], "dirty close discarded changes without confirmation");
remoteDialog.props.saving = true;
await nextTick();
remoteDialog.state.apply();
assert.equal(remoteSaves.length, 1, "duplicate save while pending");
remoteDialog.props.saving = false;
remoteDialog.props.modelValue = "PORT=81\n";
await nextTick();
assert.equal(remoteDialog.state.dirty.value, false);
remoteDialog.state.close();
assert.deepEqual(remoteCloses, [false]);
remoteDialog.close();

const command = mount(CommandEditor, {
  step: {
    id: "step-a",
    type: "command",
    commandMode: "lines",
    commands: ["sh restart.sh"],
    command: script,
  },
});
command.state.editing.value = 0;
command.state.editorText.value = "sudo supervisorctl restart api";
assert.equal(command.props.step.commands[0], "sudo supervisorctl restart api");
assert.equal(command.props.step.command, script);
command.state.editing.value = "script";
command.state.editorText.value = edited;
assert.equal(command.props.step.command, edited);
assert.equal(command.props.step.commands[0], "sudo supervisorctl restart api");
command.props.step = {
  id: "step-b",
  type: "command",
  commandMode: "lines",
  commands: ["echo other"],
  command: "",
};
await nextTick();
assert.equal(
  command.state.editing.value,
  null,
  "changing nodes must close the old popup",
);
command.close();
console.log(
  "Editor and panel checks passed: draft cancel/apply, exact script text, node isolation, pointer capture, width bounds, keyboard resize, persistence, and observer cleanup.",
);
