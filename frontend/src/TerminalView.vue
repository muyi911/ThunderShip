<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch, nextTick } from "vue";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { api } from "./api";
import { interactiveInput, pathCompletionQuery, quoteCompletion } from "./terminalCompletion";
import { useCommandHistory } from "./composables/useCommandHistory";
import "@xterm/xterm/css/xterm.css";
const props = defineProps<{
  connection?: string;
  output?: string;
  closed?: boolean;
  initialDirectory?: string;
  serverId?: string;
  commandId?: string;
  commandConnection?: string;
  running?: boolean;
  interactive?: boolean;
}>();
const emit = defineEmits<{ closed: []; error: [unknown]; interactive: [boolean] }>();
defineExpose({ changeDirectory: async (path: string) => {
  if (!props.connection || props.closed) return;
  await api("WriteTerminal", props.connection, "\x15cd -- " + quoteCompletion(path) + "\r");
  terminal?.focus();
} });
const host = ref<HTMLElement>();
const candidates = ref<{ text: string; label: string; directory: boolean }[]>([]),
  selected = ref(0), menu = ref<HTMLElement>(),
  menuStyle = ref({ left: "0px", top: "0px", maxHeight: "240px" });
const history = props.serverId ? useCommandHistory(props.serverId) : undefined;
let completionTimer: ReturnType<typeof setTimeout> | undefined,
  revision = 0, typed = false, completionText = "",
  directory = props.initialDirectory || "/", pendingDirectory = "";
const home = directory;
let terminal: Terminal | undefined,
  observer: ResizeObserver | undefined,
  timer: ReturnType<typeof setTimeout> | undefined,
  disposed = false;
let written = "";
const raw = ref(false);
watch(() => props.interactive, value => { if (value && props.running) setRaw(true); });
watch(() => props.running, running => { if (!running) setRaw(false); });
function setRaw(value: boolean) {
  if (raw.value === value) return;
  raw.value = value;
  if (terminal) {
    terminal.options.disableStdin = !value;
    terminal.options.cursorBlink = value;
    terminal.write(value ? "\x1b[?25h" : "\x1b[?25l");
  }
  emit("interactive", value);
  if (value) void nextTick(() => terminal?.focus());
}
function dismissCompletion() {
  revision++;
  if (completionTimer) clearTimeout(completionTimer);
  candidates.value = [];
}
function currentInput() {
  return terminal ? interactiveInput(terminal.buffer.active, terminal.rows) : null;
}
function syncDirectory(prompt: string) {
  // shortcut: custom prompts need OSC 7; otherwise infer cwd from Bash prompts and a confirmed cd.
  const shown = /\s(\/[^\]]*|~[^\]]*|[^\s\]]+)\][$#] $/.exec(prompt)?.[1];
  if (shown?.startsWith("/")) directory = shown;
  else if (shown === "~") directory = home;
  else if (shown?.startsWith("~/")) directory = home + shown.slice(1);
  else if (pendingDirectory && shown === pendingDirectory.split("/").filter(Boolean).at(-1)) directory = pendingDirectory;
  pendingDirectory = "";
}
function positionMenu() {
  if (!terminal || !host.value || !candidates.value.length) return;
  const screen = host.value.querySelector<HTMLElement>(".xterm-screen");
  if (!screen) return;
  const bounds = screen.getBoundingClientRect(), parent = host.value.getBoundingClientRect();
  const row = terminal.buffer.active.cursorY, height = bounds.height / terminal.rows;
  const below = parent.bottom - bounds.top - (row + 1) * height;
  const menuHeight = Math.min(240, candidates.value.length * 28 + 8);
  const above = below < Math.min(100, menuHeight);
  menuStyle.value = {
    left: `${Math.max(0, Math.min(bounds.left - parent.left + terminal.buffer.active.cursorX * bounds.width / terminal.cols, parent.width - 230))}px`,
    top: `${Math.max(0, bounds.top - parent.top + (above ? row * height - menuHeight : (row + 1) * height))}px`,
    maxHeight: `${above ? menuHeight : Math.max(28, Math.min(240, below))}px`,
  };
}
function scheduleCompletion() {
  dismissCompletion();
  if (!typed || !terminal || props.closed || document.activeElement !== terminal.textarea) return;
  const input = currentInput();
  if (!input?.text.trim()) return;
  const text = input.text, query = pathCompletionQuery(text), current = revision;
  completionTimer = setTimeout(async () => {
    let paths: string[] = [];
    if (query) {
      try { paths = await api("CompleteTerminalPath", props.connection!, directory, query.word, query.directoriesOnly); }
      catch { /* Shell completion remains available when SFTP cannot read a directory. */ }
    }
    if (disposed || props.closed || current !== revision || currentInput()?.text !== text || document.activeElement !== terminal?.textarea) return;
    completionText = text;
    const prefix = query?.word.slice(0, query.word.lastIndexOf("/") + 1) || "";
    const items = paths.filter(path => !/[\x00-\x1f\x7f]/.test(path)).map(path => ({text: query!.before + quoteCompletion(path), label: path.slice(prefix.length), directory: path.endsWith("/")}));
    for (const command of history?.commands.value || []) {
      if (command.startsWith(text) && command !== text && !/[\x00-\x1f\x7f]/.test(command)) items.push({text: command, label: command, directory: false});
    }
    candidates.value = items.filter((item, index) => item.text !== text && items.findIndex(other => other.text === item.text) === index).slice(0, 100);
    selected.value = 0;
    positionMenu();
  }, 180);
}
function acceptCompletion(index: number) {
  const item = candidates.value[index], input = currentInput();
  if (!item || !input || input.text !== completionText || !terminal || props.closed) { dismissCompletion(); return; }
  const query = pathCompletionQuery(input.text);
  const data = item.text.startsWith(input.text) ? item.text.slice(input.text.length)
    : query ? "\x7f".repeat(Array.from(input.text.slice(query.before.length)).length) + item.text.slice(query.before.length) : "";
  dismissCompletion();
  if (data) terminal.input(data, true);
  typed = false;
  terminal.focus();
}
function completionKey(event: KeyboardEvent) {
  if (event.type !== "keydown" || event.isComposing || event.ctrlKey || event.altKey || event.metaKey || event.shiftKey || !candidates.value.length) return true;
  if (event.key === "Escape") { typed = false; dismissCompletion(); }
  else if (event.key === "Enter" || event.key === "Tab") acceptCompletion(selected.value);
  else if (event.key === "ArrowUp" || event.key === "ArrowDown") {
    selected.value = (selected.value + (event.key === "ArrowDown" ? 1 : -1) + candidates.value.length) % candidates.value.length;
    void nextTick(() => menu.value?.querySelector('[aria-selected="true"]')?.scrollIntoView({block: "nearest"}));
  } else return true;
  event.preventDefault();
  return false;
}
watch([candidates, selected], () => {
  const input = terminal?.textarea;
  if (!input || !props.connection) return;
  input.setAttribute("aria-autocomplete", "list");
  input.setAttribute("aria-controls", "terminal-completions-" + props.connection);
  input.setAttribute("aria-expanded", String(!!candidates.value.length));
  if (candidates.value.length) input.setAttribute("aria-activedescendant", `terminal-option-${props.connection}-${selected.value}`);
  else input.removeAttribute("aria-activedescendant");
});
function updateOutput() {
  if (!terminal || props.connection) return;
  const text = props.output || "";
  if (!text.startsWith(written)) terminal.reset();
  terminal.write(text.startsWith(written) ? text.slice(written.length) : text, () => {
    if (!raw.value) terminal?.write("\x1b[?25l");
  });
  written = text;
}
watch(() => props.output, updateOutput);
onMounted(async () => {
  await document.fonts.load('500 14px "JetBrains Mono"');
  if (disposed) return;
  terminal = new Terminal({
    fontSize: 14,
    fontFamily: '"JetBrains Mono", "Microsoft YaHei", Consolas, monospace',
    fontWeight: 500,
    fontWeightBold: 700,
    lineHeight: 1.35,
    cursorBlink: !!props.connection,
    disableStdin: !props.connection,
    convertEol: !props.connection && !props.commandId,
    allowTransparency: true,
    scrollback: 5000,
    theme: {
      background: "#00000000",
      foreground: "#d7dce3",
      blue: "#77aaff",
      brightBlue: "#9bbfff",
    },
  });
  const fit = new FitAddon();
  terminal.loadAddon(fit);
  terminal.open(host.value!);
  if (props.commandId) {
    const fallback = () => { if (props.running) setRaw(true); };
    terminal.buffer.onBufferChange(buffer => { if (buffer.type === "alternate") fallback(); });
    terminal.parser.registerCsiHandler({prefix: "?", final: "h"}, params => {
      if (params.some(value => typeof value === "number" && [1, 47, 1047, 1049, 1000, 1002, 1003, 66].includes(value))) fallback();
      return false;
    });
    terminal.parser.registerEscHandler({final: "="}, () => { fallback(); return false; });
  }
  if (props.connection) {
    terminal.attachCustomKeyEventHandler(completionKey);
    terminal.parser.registerOscHandler(7, data => {
      try { const path = decodeURIComponent(new URL(data).pathname); if (path.startsWith("/") && !/[\x00\r\n]/.test(path)) directory = path; } catch { /* Ignore unsupported shell integration payloads. */ }
      return false;
    });
    terminal.onWriteParsed(() => {
      const input = currentInput();
      if (input && !input.text) syncDirectory(input.prompt);
      scheduleCompletion();
    });
    terminal.onScroll(dismissCompletion);
  }
  let input = Promise.resolve();
  terminal.onData((data) => {
    if (props.commandId) {
      if (raw.value && props.running && props.commandConnection) {
        input = input.then(() => api("WriteCommand", props.commandConnection!, props.commandId!, data)).catch(e => emit("error", e));
      }
      return;
    }
    if (!props.connection || props.closed) return;
    const command = currentInput()?.text;
    typed = !/[\r\n\x03\x04]/.test(data);
    if (data === "\r" && command) {
      history?.remember(command);
      const query = pathCompletionQuery(command);
      if (query?.directoriesOnly && query.before.trim() === "cd" && !/[$`]/.test(query.word)) {
        const target = query.word.startsWith("~") ? home + query.word.slice(1) : query.word.startsWith("/") ? query.word : directory + "/" + query.word;
        const parts: string[] = [];
        for (const part of target.split("/")) { if (part === "..") parts.pop(); else if (part && part !== ".") parts.push(part); }
        pendingDirectory = "/" + parts.join("/");
      }
    }
    scheduleCompletion();
    input = input
      .then(() => api("WriteTerminal", props.connection!, data))
      .catch((e) => emit("error", e));
  });
  terminal.onResize(({ cols, rows }) => {
    if (props.commandId) {
      if (props.running && props.commandConnection) void api("ResizeCommand", props.commandConnection, props.commandId, cols, rows).catch(e => { if (props.running) emit("error", e); });
      return;
    }
    dismissCompletion();
    if (!props.connection || props.closed) return;
    void api("ResizeTerminal", props.connection, cols, rows).catch((e) =>
      emit("error", e),
    );
  });
  observer = new ResizeObserver(() => {
    if (
      host.value &&
      host.value.clientWidth > 40 &&
      host.value.clientHeight > 40
    )
      fit.fit();
  });
  observer.observe(host.value!);
  if (props.connection) {
    terminal.focus();
    void poll();
  } else updateOutput();
});
async function poll() {
  if (props.closed || disposed) return;
  try {
    const output = await api("ReadTerminal", props.connection!);
    if (disposed) return;
    if (output.data)
      terminal?.write(
        Uint8Array.from(atob(output.data), (c) => c.charCodeAt(0)),
      );
    if (output.closed) {
      if (output.error) emit("error", output.error);
      terminal?.write("\r\n\x1b[90m[连接已结束]\x1b[0m\r\n");
      emit("closed");
      return;
    }
  } catch (e) {
    if (!disposed && !props.closed) {
      emit("error", e);
      emit("closed");
    }
    return;
  }
  if (!disposed) timer = setTimeout(() => void poll(), 0);
}
onBeforeUnmount(() => {
  disposed = true;
  dismissCompletion();
  if (timer) clearTimeout(timer);
  observer?.disconnect();
  terminal?.dispose();
});
</script>
<template>
  <div ref="host" class="ssh-terminal" :class="{ 'interactive-terminal': !!connection }" aria-label="SSH 交互终端" @focusout="dismissCompletion">
    <div v-if="candidates.length" :id="'terminal-completions-' + connection" ref="menu" class="interactive-completions" :style="menuStyle" role="listbox" aria-label="终端补全">
      <button v-for="(item, index) in candidates" :id="`terminal-option-${connection}-${index}`" :key="item.text" type="button" role="option" :aria-selected="selected === index" :title="item.text" @mousedown.prevent @click="acceptCompletion(index)">
        <v-icon size="14">{{ item.directory ? 'mdi-folder-outline' : 'mdi-file-outline' }}</v-icon><span>{{ item.label }}</span>
      </button>
    </div>
  </div>
</template>
<style scoped>
.ssh-terminal { position: relative; }
.interactive-completions { position: absolute; z-index: 30; min-width: 100px; max-width: min(360px, 100%); overflow: auto; padding: 4px; border: 1px solid #414345; border-radius: 5px; background: #222426; box-shadow: 0 4px 12px #0007; font: 500 14px/20px "JetBrains Mono", "Microsoft YaHei", monospace; }
.interactive-completions button { display: flex; align-items: center; gap: 6px; width: 100%; height: 28px; padding: 3px 6px; text-align: left; white-space: pre; color: #cfd2d4; }
.interactive-completions button > span:last-child { overflow: hidden; text-overflow: ellipsis; }
.interactive-completions button[aria-selected="true"], .interactive-completions button:hover { background: #353a40; }
</style>
