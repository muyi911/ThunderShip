import { mockWailsBindings } from "./mock-wails-bindings.mjs";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
const require = createRequire(import.meta.url);
const historyStorage = new Map();
globalThis.localStorage = { getItem: (key) => historyStorage.get(key) ?? null, setItem: (key, value) => historyStorage.set(key, value) };
Object.defineProperty(globalThis, "navigator", { configurable: true, value: { clipboard: { writeText: async () => {} } } });
const { build } = require("../frontend/node_modules/esbuild");
const {
  parse,
  compileScript,
} = require("../frontend/node_modules/@vue/compiler-sfc");
const result = await build({
  stdin: {
    contents:
      'export {interactiveInput, pathCompletionQuery, quoteCompletion} from "./src/terminalCompletion"; export {useCommandHistory} from "./src/composables/useCommandHistory"; export {useConnectionTabs} from "./src/composables/useConnectionTabs"; export {remoteFileClipboard} from "./src/remoteFileClipboard"; export {default as TerminalView} from "./src/TerminalView.vue"; export {default as EnhancedTerminal} from "./src/EnhancedTerminal.vue"; export {default as RemoteFiles} from "./src/RemoteFiles.vue"; export {createRenderer,h,reactive,nextTick} from "vue";',
    resolveDir: fileURLToPath(new URL("../frontend", import.meta.url)),
  },
  bundle: true,
  platform: "node",
  format: "esm",
  write: false,
  plugins: [mockWailsBindings,
    {
      name: "scripts",
      setup(build) {
        build.onResolve({ filter: /^@xterm\// }, ({ path }) => ({ path, namespace: "terminal-mock" }));
        build.onLoad({ filter: /.*/, namespace: "terminal-mock" }, ({ path }) => ({ contents: path.includes("addon-fit")
          ? "export class FitAddon { fit() {} }"
          : path.endsWith(".css") ? ""
          : `export class Terminal {
            constructor(options) { this.options = options; this.writes = []; this.rows = 24; this.cols = 80; this.textarea = {setAttribute() {}, removeAttribute() {}}; this.buffer = {active: {type: 'normal', viewportY: 0, baseY: 0, cursorY: 0, cursorX: 0, getLine() {return undefined;}}}; this.parser = {registerOscHandler: (code, fn) => {this.osc = fn;}}; globalThis.outputTerminal = this; }
            write(text, callback) { this.writes.push(text); callback?.(); }
            reset() { this.resetCalled = true; }
            loadAddon() {} open() {} onData(callback) { this.inputData = callback; } onResize() {} dispose() {} focus() {}
            onWriteParsed(fn) { this.parsed = fn; } onScroll() {} attachCustomKeyEventHandler(fn) { this.keyHandler = fn; }
            input(data) { this.inputData(data); }
          }` }));
        build.onLoad({ filter: /\.vue$/ }, async ({ path }) => {
          if (
            !path.endsWith("TerminalView.vue") &&
            !path.endsWith("EnhancedTerminal.vue") &&
            !path.endsWith("RemoteFiles.vue")
          )
            return { contents: "export default {}", loader: "js" };
          const { descriptor } = parse(await readFile(path, "utf8"));
          return {
            contents:
              compileScript(descriptor, { id: path, genDefaultAs: "Component" })
                .content + "\nexport default Component;",
            loader: "ts",
          };
        });
      },
    },
  ],
});
const { interactiveInput, pathCompletionQuery, quoteCompletion, TerminalView, reactive, nextTick, useCommandHistory, useConnectionTabs, remoteFileClipboard, EnhancedTerminal, RemoteFiles, createRenderer, h } =
  await import(
    "data:text/javascript;base64," +
    Buffer.from(result.outputFiles[0].text + "\n//# sourceURL=terminal-check-components.js").toString("base64")
  );
const tabs = useConnectionTabs(),
  server = { id: "a", name: "Server A" };
tabs.openConnection(server);
const first = tabs.activeConnection.value;
tabs.openConnection(server);
const second = tabs.activeConnection.value;
tabs.openConnection({ id: "b", name: "Server B" });
assert.equal(tabs.connections.value.length, 3);
assert.notEqual(first, second);
assert.equal(tabs.connections.value[1].number, 2);
tabs.activeConnection.value = first;
tabs.closeConnectionTab(second);
assert.equal(tabs.activeConnection.value, first);
tabs.closeConnectionTab(first);
assert.equal(tabs.connectedServer.value.id, "b");
assert.equal(server.name, "Server A");
tabs.closeConnectionTab(tabs.activeConnection.value);
assert.equal(tabs.connectedServer.value, null);

const calls = [],
  readers = new Map();
let counter = 0,
  suggestionCalls = 0,
  suggestions = [],
  state;
globalThis.window = { chrome: { webview: {} } };
globalThis.backend = {
        async StartCommand(connection, command, directory) {
          calls.push({ connection, command, directory });
          const id = String(++counter);
          readers.set(id, 0);
          return id;
        },
        async ReadCommand(connection, id) {
          const count = readers.get(id);
          readers.set(id, count + 1);
          return {
            data: Buffer.from(count === 0 ? "hello\n" : "done\n").toString(
              "base64",
            ),
            closed: count > 0,
            error: "",
            directory: "/tmp",
            duration: 20,
            exitCode: 0,
          };
        },
        async CancelCommand() {},
        async CancelGeneration() {},
        async SuggestTerminalCommand(...args) {
          suggestionCalls++;
          suggestions.push(args);
          return { command: "ls -la", explanation: "列出目录" };
        },
      };
const renderer = createRenderer({
  createComment: () => ({}),
  insert() {},
  remove() {},
  parentNode: () => null,
  nextSibling: () => null,
});
// Output-only terminals suppress the cursor after every chunk, including server cursor-show escapes.
globalThis.document = { fonts: { load: async () => [] } };
globalThis.ResizeObserver = class { observe() {} disconnect() {} };
const outputProps = reactive({ output: "hello\n" });
let terminalState;
const outputComponent = { ...TerminalView, setup(p, c) { terminalState = TerminalView.setup(p, c); return () => null; } };
const outputApp = renderer.createApp({ render: () => h(outputComponent, outputProps) });
outputApp.mount({});
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(outputTerminal.options.disableStdin, true);
assert.equal(outputTerminal.options.cursorBlink, false);
assert.equal(outputTerminal.writes.slice(-2).join(""), "hello\n\x1b[?25l");
outputProps.output += "\x1b[?25hmore";
await nextTick();
assert.equal(outputTerminal.writes.slice(-2).join(""), "\x1b[?25hmore\x1b[?25l");
outputProps.output = "replaced";
await nextTick();
assert.equal(outputTerminal.resetCalled, true);
assert.equal(outputTerminal.writes.slice(-2).join(""), "replaced\x1b[?25l");
outputApp.unmount();
const terminalInputs = [], terminalReads = [];
globalThis.backend.ReadTerminal = async () => new Promise(resolve => terminalReads.push(resolve));
globalThis.backend.WriteTerminal = async (...args) => terminalInputs.push(args);
const interactiveApp = renderer.createApp({ render: () => h(outputComponent, {connection: 'interactive', closed: false}) });
interactiveApp.mount({});
await new Promise(resolve => setTimeout(resolve, 0));
outputTerminal.inputData('\x7f');
await nextTick();
await new Promise(resolve => setTimeout(resolve, 0));
assert.deepEqual(terminalInputs, [['interactive', '\x7f']]);
terminalReads[0]({data: Buffer.from('\b \b').toString('base64'), closed: false});
await new Promise(resolve => setTimeout(resolve, 20));
assert.equal(terminalReads.length, 2, 'interactive output still waits 100ms before reading another response');
assert.equal(Buffer.from(outputTerminal.writes[0]).toString(), '\b \b');
const buffer = outputTerminal.buffer.active;
let promptLine = '[qa@host ~]$ cd /u';
buffer.cursorX = promptLine.length;
buffer.getLine = row => row === 0 ? {isWrapped: false, translateToString: (trim, start = 0, end) => promptLine.slice(start, end)} : undefined;
assert.deepEqual(interactiveInput(buffer, 24), {prompt: '[qa@host ~]$ ', text: 'cd /u'});
buffer.type = 'alternate';
assert.equal(interactiveInput(buffer, 24), null, 'fullscreen application received shell suggestions');
buffer.type = 'normal';
promptLine = 'Password: ';
buffer.cursorX = promptLine.length;
assert.equal(interactiveInput(buffer, 24), null, 'password entry was treated as a command');
promptLine = '[qa@host ~]$ cd /u';
buffer.cursorX = promptLine.length - 1;
assert.equal(interactiveInput(buffer, 24), null, 'suggestions opened while editing in the middle');
buffer.cursorX = promptLine.length;
document.activeElement = outputTerminal.textarea;
const interactivePathCalls = [];
globalThis.backend.CompleteTerminalPath = async (...args) => {interactivePathCalls.push(args); return ['/usr/', '/user data/', '/unsafe\rcommand/'];};
outputTerminal.inputData('u');
outputTerminal.parsed();
await new Promise(resolve => setTimeout(resolve, 220));
assert.deepEqual(interactivePathCalls.at(-1), ['interactive', '/', '/u', true]);
assert.deepEqual(terminalState.candidates.value.map(item => item.label), ['usr/', 'user data/']);
let completionPrevented = false;
assert.equal(outputTerminal.keyHandler({type: 'keydown', key: 'Enter', preventDefault() {completionPrevented = true;}}), false);
await new Promise(resolve => setTimeout(resolve, 0));
assert.equal(completionPrevented, true);
assert.deepEqual(terminalInputs.at(-1), ['interactive', 'sr/'], 'path selection sent Enter or replaced the entire command');
assert.equal(terminalState.candidates.value.length, 0);
outputTerminal.inputData('u'); outputTerminal.parsed();
await new Promise(resolve => setTimeout(resolve, 220));
terminalState.acceptCompletion(1);
await new Promise(resolve => setTimeout(resolve, 0));
assert.deepEqual(terminalInputs.at(-1), ['interactive', "\x7f\x7f'/user data/'"], 'space-containing path was not quoted or erased the prompt');
const wrappedLines = ['[qa@host ~]$ cd /', 'usr/'];
const wrappedBuffer = {...buffer, cursorY: 1, cursorX: 4, getLine: row => row < 2 ? {isWrapped: row === 1, translateToString: (trim, start = 0, end) => wrappedLines[row].slice(start, end)} : undefined};
assert.equal(interactiveInput(wrappedBuffer, 24).text, 'cd /usr/');
assert.equal(interactiveInput({...wrappedBuffer, viewportY: 1}, 24), null, 'completion covered scrolled-back output');
let resolveInteractiveOld;
globalThis.backend.CompleteTerminalPath = async () => new Promise(resolve => {resolveInteractiveOld = resolve;});
outputTerminal.inputData('u'); outputTerminal.parsed();
await new Promise(resolve => setTimeout(resolve, 220));
promptLine = 'Password: ';
buffer.cursorX = promptLine.length;
outputTerminal.parsed();
resolveInteractiveOld(['/unwanted/']);
await new Promise(resolve => setTimeout(resolve, 0));
assert.equal(terminalState.candidates.value.length, 0, 'late completion reopened over a password prompt');
interactiveApp.unmount();
terminalReads[1]({data: '', closed: false});
assert.deepEqual(pathCompletionQuery('cd /u'), {before: 'cd ', word: '/u', directoriesOnly: true});
assert.deepEqual(pathCompletionQuery('cat "my f'), {before: 'cat ', word: 'my f', directoriesOnly: false});
assert.deepEqual(pathCompletionQuery('echo ok && cd ../a'), {before: 'echo ok && cd ', word: '../a', directoriesOnly: true});
assert.equal(pathCompletionQuery('cat $(touch file)'), null);
assert.equal(quoteCompletion('~/a b/'), "~/'a b/'");
assert.equal(quoteCompletion("/a'b"), "'/a'\\''b'");
const component = {
  ...EnhancedTerminal,
  setup(p, c) {
    state = EnhancedTerminal.setup(p, c);
    return () => null;
  },
};
const app = renderer.createApp({
  render: () =>
    h(component, {
      connection: "c1",
      initialDirectory: "/",
      server,
      providers: [{ id: "p", name: "Mock" }],
      closed: false,
    }),
});
app.mount({});
state.command.value = "cd /tmp";
await state.run();
assert.equal(state.foreground.value, true);
state.command.value = "echo should-wait";
await state.run();
assert.equal(calls.length, 1);
state.command.value = "echo background";
await state.run(true);
assert.equal(calls.length, 2);
await new Promise((r) => setTimeout(r, 220));
assert.equal(state.blocks.value[0].output, "hello\ndone\n");
assert.equal(state.directory.value, "/tmp");
assert.equal(state.activeCount.value, 0);
state.command.value = "列出文件";
await state.generate();
assert.equal(suggestionCalls, 1);
assert.equal(calls.length, 2, "AI must not execute commands");
assert.equal(state.command.value, "列出文件");
state.useSuggestion();
assert.equal(state.command.value, "ls -la");
assert.equal(state.mode.value, "command");
state.clearHistory();
state.remember("docker ps");
state.remember("docker logs app");
state.remember("ls -la");
state.remember("docker ps");
assert.deepEqual(state.historyCommands.value, ["docker ps", "ls -la", "docker logs app"]);
assert.deepEqual(useCommandHistory("a").commands.value, state.historyCommands.value, "same-server sessions do not share history");
assert.deepEqual(useCommandHistory("b").commands.value, [], "history leaked to another server");
const saved = JSON.parse(historyStorage.get("thundership.command-history.a"));
assert.deepEqual(saved, ["docker ps", "ls -la", "docker logs app"]);
historyStorage.set("thundership.command-history.restored", JSON.stringify(["saved command"]));
assert.deepEqual(useCommandHistory("restored").commands.value, ["saved command"]);
function press(key, extra = {}) {
  let prevented = false;
  state.key({ key, preventDefault() { prevented = true; }, ...extra });
  return prevented;
}
state.command.value = "dock";
state.commandInput();
state.inputFocused.value = true;
assert.equal(state.ghost.value, "er ps");
assert.equal(press("ArrowRight"), true);
assert.equal(state.command.value, "docker ps");
state.command.value = "dock";
state.commandInput();
assert.equal(press("ArrowUp", { isComposing: true }), false, "history hijacked IME composition");
assert.equal(state.command.value, "dock");
press("Tab");
assert.equal(state.completionOpen.value, true);
press("ArrowDown");
press("Enter");
assert.equal(state.command.value, "docker logs app");
assert.equal(calls.length, 2, "accepting completion executed a command");
state.command.value = "unfinished draft";
state.commandInput();
press("ArrowUp");
assert.equal(state.command.value, "docker ps");
press("ArrowUp");
assert.equal(state.command.value, "ls -la");
press("ArrowDown");
press("ArrowDown");
assert.equal(state.command.value, "unfinished draft", "history navigation lost the draft");
state.command.value = "dock";
state.commandInput();
press("Tab");
press("Escape");
assert.equal(state.completionOpen.value, false);
assert.equal(state.ghost.value, "");
state.input.value = { selectionStart: 2, selectionEnd: 2 };
assert.equal(press("ArrowRight"), false, "completion replaced text while editing in the middle");
state.command.value = "first\nsecond";
state.commandInput();
state.input.value = { selectionStart: 8, selectionEnd: 8 };
assert.equal(press("ArrowUp"), false, "history hijacked multiline cursor movement");
state.input.value = undefined;
state.mode.value = "ai";
state.command.value = "dock";
state.commandInput();
assert.equal(press("Tab"), false, "shell history hijacked AI question editing");
assert.equal(state.completions.value.length, 0);
state.mode.value = "command";
const block = state.blocks.value[0];
assert.equal(state.status(block), "success");
assert.equal(state.status({ ...block, running: true }), "running");
assert.equal(state.status({ ...block, exitCode: 1 }), "error");
assert.equal(state.status({ ...block, error: "transport failed" }), "error");
state.referenceBlock(block);
state.referenceBlock(block);
assert.equal(state.mode.value, "ai");
assert.equal(state.references.value.length, 1, "duplicate reference");
assert.equal(suggestionCalls, 1, "referencing must not send automatically");
globalThis.FileReader = class {
  readAsDataURL(file) { this.result = file.data; this.onload(); }
};
const data = "data:image/png;base64,test-image";
let prevented = false;
state.paste({ clipboardData: { files: [{ name: "screenshot.png", type: "image/png", size: 20, data }] }, preventDefault() { prevented = true; } });
await new Promise((r) => setTimeout(r, 0));
assert.equal(prevented, true);
assert.equal(state.images.value.length, 1);
state.command.value = "解释截图里的错误";
await state.generate();
assert.equal(calls.length, 2, "AI with attachments must not execute");
assert.match(suggestions.at(-1)[4], /cd \/tmp/);
assert.match(suggestions.at(-1)[4], /hello/);
assert.deepEqual(suggestions.at(-1)[6], [data]);
await state.addImages([{ name: "bad.txt", type: "text/plain", size: 1, data: "bad" }]);
assert.equal(state.images.value.length, 1, "non-image accepted");
await state.addImages([{ name: "huge.png", type: "image/png", size: 9 * 1024 * 1024, data }]);
assert.equal(state.images.value.length, 1, "oversized image accepted");
state.references.value = [];
state.images.value = [];
state.command.value = "another question";
await state.generate();
assert.equal(suggestions.at(-1)[4], "", "removed reference was sent");
assert.deepEqual(suggestions.at(-1)[6], [], "removed image was sent");
const pathRequests = [];
globalThis.backend.CompleteTerminalPath = async (...args) => { pathRequests.push(args); return ['/usr/', '/user data/']; };
state.mode.value = 'command';
state.inputFocused.value = true;
state.input.value = undefined;
state.command.value = 'cd /u';
state.commandInput();
await nextTick();
await new Promise(resolve => setTimeout(resolve, 220));
assert.deepEqual(pathRequests.at(-1), ['c1', '/tmp', '/u', true]);
assert.deepEqual(state.completions.value, ['cd /usr/', "cd '/user data/'"]);
assert.equal(state.completionOpen.value, false, 'typing opened a list over the inline preview');
assert.equal(state.ghost.value, 'sr/');
press('ArrowRight');
assert.equal(state.command.value, 'cd /usr/');
assert.equal(calls.length, 2, 'accepting a path suggestion executed it');
let finishOldPath;
globalThis.backend.CompleteTerminalPath = async () => new Promise(resolve => { finishOldPath = resolve; });
state.command.value = 'cd /old';
state.commandInput();
await nextTick();
await new Promise(resolve => setTimeout(resolve, 220));
state.command.value = 'cd /new';
state.commandInput();
await nextTick();
finishOldPath(['/old-folder/']);
await nextTick();
assert.deepEqual(state.pathCompletions.value, [], 'old completion response replaced a newer command');
app.unmount();
const listing = [
  {
    name: "z",
    directory: true,
    size: 0,
    modified: "2026-10-23T12:00:00Z",
    mode: "drwxr-xr-x",
  },
  { name: ".hidden", directory: false, size: 2 },
  { name: "index.html", directory: false, size: 1024 },
  { name: "assets", directory: true, size: 0 },
];
const requests = [];
globalThis.backend.ListRemote = async (connection, dir) => {
  requests.push(dir);
  if (dir === "/missing") throw new Error("missing");
  return listing;
};
let files;
const filesComponent = {
  ...RemoteFiles,
  setup(p, c) {
    files = RemoteFiles.setup(p, c);
    return () => null;
  },
};
const filesApp = renderer.createApp({
  render: () =>
    h(filesComponent, {
      connection: "c1",
      serverId: "server-one",
      initialDirectory: "/",
      closed: false,
    }),
});
filesApp.mount({});
await new Promise((r) => setTimeout(r, 0));
assert.deepEqual(
  files.visible.value.map((e) => e.name),
  ["assets", "z", ".hidden", "index.html"],
);
assert.equal(files.count.value.files, 2);
assert.equal(files.count.value.directories, 2);
files.showHidden.value = false;
assert.equal(files.count.value.files, 1);
files.search.value = "index";
assert.equal(files.visible.value.length, 1);
files.search.value = "";
await files.list("/a");
await files.list("/a/b");
assert.deepEqual(files.history.value, ["/", "/a"]);
await files.back();
assert.equal(files.directory.value, "/a");
assert.deepEqual(files.history.value, ["/"]);
await files.list("/missing");
assert.equal(files.directory.value, "/a");
assert.deepEqual(files.history.value, ["/"]);
await files.parent();
assert.equal(files.directory.value, "/");
assert.equal(files.child("index.html"), "/index.html");
assert.equal(files.size(1048576), "1.0 MB");
const textReads = [], textSaves = [];
globalThis.backend.ReadRemoteText = async (...args) => {
  textReads.push(args);
  return { content: "PORT=80\r\n", version: "original" };
};
globalThis.backend.SaveRemoteText = async (...args) => {
  textSaves.push(args);
  if (args[2] === "fail") throw Error("server changed");
  return { content: args[2], version: "saved" };
};
files.sudo.value = true;
await files.edit(listing[2]);
assert.equal(files.editorOpen.value, true);
assert.equal(files.editor.value.content, "PORT=80\n");
assert.deepEqual(textReads[0], ["c1", "/index.html", true]);
// Navigating/toggling sudo must not change the file or credentials being edited.
files.sudo.value = false;
await files.list("/elsewhere");
await files.saveText("PORT=81\n");
assert.deepEqual(textSaves[0], ["c1", "/index.html", "PORT=81\n", "original", true]);
assert.equal(files.editor.value.version, "saved");
assert.equal(files.editor.value.content, "PORT=81\n");
assert.equal(files.editorOpen.value, true, "successful save unexpectedly closed editor");
await files.saveText("fail");
assert.match(files.editorError.value, /server changed/);
assert.equal(files.editor.value.content, "PORT=81\n", "failed save advanced the baseline");
assert.equal(files.editor.value.version, "saved");
assert.equal(files.editorOpen.value, true, "failed save discarded editor");
const fileOperations = [];
globalThis.backend.ManageRemoteFile = async (...args) => {
  fileOperations.push(args);
  if (args[1].target.endsWith("/collision")) throw Error("目标已存在");
};
await files.list("/files");
files.beginOperation("mkdir", null);
files.operationValue.value = "../escape";
await files.applyOperation();
assert.equal(fileOperations.length, 0, "invalid name reached backend");
assert.match(files.operationError.value, /名称/);
files.operationValue.value = "folder 'quoted";
await files.applyOperation();
assert.deepEqual(fileOperations.at(-1), ["c1", { action: "mkdir", source: "/files/folder 'quoted", target: "", mode: "" }, false]);
files.beginOperation("rename", listing[2]);
files.operationValue.value = "collision";
files.sudo.value = true;
await files.list("/changed-while-dialog-open");
await files.applyOperation();
assert.deepEqual(fileOperations.at(-1), ["c1", { action: "rename", source: "/files/index.html", target: "/files/collision", mode: "" }, false], "dialog changed path or sudo scope");
assert.match(files.operationError.value, /目标已存在/);
assert.equal(files.operation.value.action, "rename", "failed operation lost its draft");
files.operationValue.value = "renamed.html";
await files.applyOperation();
assert.equal(files.operation.value, null);
assert.equal(files.directory.value, "/files");
files.sudo.value = false;
files.fileClipboard(false, listing[2]);
assert.equal(remoteFileClipboard.value.server, "server-one");
files.paste();
assert.equal(files.operationValue.value, "index-副本.html", "same-directory copy should propose another name");
await files.applyOperation();
assert.deepEqual(fileOperations.at(-1)[1], { action: "copy", source: "/files/index.html", target: "/files/index-副本.html", mode: "" });
files.fileClipboard(true, listing[2]);
await files.list("/destination");
files.paste();
await files.applyOperation();
assert.deepEqual(fileOperations.at(-1)[1], { action: "move", source: "/files/index.html", target: "/destination/index.html", mode: "" });
assert.equal(remoteFileClipboard.value, null, "successful move left a stale cut clipboard");
files.fileClipboard(false, listing[2]);
remoteFileClipboard.value = { ...remoteFileClipboard.value, server: "another-server" };
assert.equal(files.canPaste.value, false, "clipboard allowed pasting onto another host");
files.operation.value = null;
files.paste();
assert.equal(files.operation.value, null);
const beforeDelete = fileOperations.length;
files.beginOperation("delete", listing[0]);
assert.equal(fileOperations.length, beforeDelete, "opening delete confirmation deleted immediately");
assert.equal(files.operation.value.isDirectory, true);
files.operation.value = null;
files.beginOperation("chmod", { ...listing[2], mode: "-rwsr-x---" });
assert.equal(files.operationValue.value, "4750");
files.setPermission(0o004, true);
assert.equal(files.operationValue.value, "4754", "checkbox changed unrelated permission bits");
await files.applyOperation();
assert.equal(fileOperations.at(-1)[1].mode, "4754");
remoteFileClipboard.value = null;
await files.list('/files');
files.sudo.value = false;
files.selectEntry({currentTarget: {focus() {}}}, listing[2]);
files.selectEntry({ctrlKey: true, currentTarget: {focus() {}}}, listing[3]);
assert.deepEqual(files.selectedNames.value, ['index.html', 'assets']);
files.fileClipboard(false, listing[2]);
assert.equal(remoteFileClipboard.value.items.length, 2);
await files.list('/batch');
files.paste();
await files.applyOperation();
assert.deepEqual(fileOperations.slice(-2).map(args => args[1].target).sort(), ['/batch/assets', '/batch/index.html']);
await files.list('/files');
files.selectEntry({currentTarget: {focus() {}}}, listing[2]);
let shortcutPrevented = false;
files.fileKeys({target: {tagName: 'TR'}, ctrlKey: true, key: 'c', preventDefault() {shortcutPrevented = true;}, stopPropagation() {}});
assert.equal(shortcutPrevented, true);
assert.equal(remoteFileClipboard.value.path, '/files/index.html');
const uploaded = [];
globalThis.backend.GetClipboardFiles = async () => ['C:/qa/index.html', 'C:/qa/assets'];
globalThis.backend.ResolveLocalPaths = async paths => paths.map((local, i) => ({id: String(i), name: local.split('/').at(-1), local, kind: local.endsWith('assets') ? 'folder' : 'file'}));
let uploadFailure = true;
const previewCalls = [];
globalThis.backend.PreviewRemoteUpload = async (...args) => {
  previewCalls.push(args);
  return args[2].map((local, i) => ({file: {id: String(i), local, remote: args[1] + '/' + local.split('/').at(-1), kind: 'file'}}));
};
globalThis.backend.UploadRemoteSelection = async (...args) => { uploaded.push(args); if (uploadFailure) throw Error('permission denied'); };
await files.pasteFiles();
assert.equal(files.upload.value, true);
assert.equal(files.uploadItems.value.length, 2, 'local clipboard lost mixed file/folder selection');
assert.equal(uploaded.length, 0, 'pasting uploaded without confirmation');
files.sudo.value = true;
await files.list('/different');
await files.send();
assert.deepEqual(previewCalls[0], ['c1', '/files', ['C:/qa/index.html', 'C:/qa/assets'], false], 'upload destination changed while confirmation was open');
assert.deepEqual(uploaded[0], ['c1', [{id: '0', local: 'C:/qa/index.html', remote: '/files/index.html', kind: 'file', executable: false}, {id: '1', local: 'C:/qa/assets', remote: '/files/assets', kind: 'file', executable: false}], false, '']);
assert.equal(files.upload.value, true, 'failed upload discarded confirmation');
assert.match(files.uploadError.value, /permission denied/);
assert.equal(files.uploadItems.value.length, 2);
uploadFailure = false;
await files.send();
assert.equal(files.upload.value, false);
assert.deepEqual(files.uploadItems.value, []);
files.localDrop({detail: {paths: ['C:/qa/index.html'], directory: '/drop-into-folder'}});
await new Promise(resolve => setTimeout(resolve, 0));
assert.equal(files.uploadDirectory.value, '/drop-into-folder');
assert.equal(uploaded.length, 2, 'dropping files uploaded without confirmation');
files.upload.value = false;
files.sudo.value = false;
globalThis.backend.GetClipboardFiles = async () => [];
await files.list('/source');
files.selectedNames.value = ['assets', 'index.html'];
files.fileClipboard(true, listing[2]);
await files.list('/destination');
files.paste();
let failedOnce = false;
globalThis.backend.ManageRemoteFile = async (...args) => {
  fileOperations.push(args);
  if (args[1].target.endsWith('/assets') && !failedOnce) { failedOnce = true; throw Error('目标已存在'); }
};
await files.applyOperation();
assert.match(files.operationError.value, /已完成 1 项/);
assert.equal(files.operation.value.items.length, 1, 'partial move did not preserve remaining items');
const beforeRetry = fileOperations.length;
await files.applyOperation();
assert.equal(fileOperations.length, beforeRetry + 1, 'retry repeated completed moves');
assert.equal(files.operation.value, null);
remoteFileClipboard.value = null;
filesApp.unmount();
console.log(
  "Terminal checks passed: independent tabs, streaming, AI references/images without execution, persisted per-server history, prefix completion, keyboard selection, draft restoration, multiline/IME boundaries and remote file save/conflict isolation.",
);
