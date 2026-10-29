<script setup lang="ts">
import { computed, onBeforeUnmount, ref, nextTick, watch } from "vue";
import type { Provider, Server, TerminalSuggestion } from "./types";
import { api, uid } from "./api";
import TerminalView from "./TerminalView.vue";
import IconButton from "./IconButton.vue";
import CodeEditorDialog from "./CodeEditorDialog.vue";
import { useCommandHistory } from "./composables/useCommandHistory";
import { pathCompletionQuery, quoteCompletion } from "./terminalCompletion";
const props = defineProps<{
  connection: string;
  initialDirectory: string;
  server: Server;
  providers: Provider[];
  closed: boolean;
}>();
const emit = defineEmits<{ error: [unknown] }>();
defineExpose({ changeDirectory: (path: string) => { directory.value = path; input.value?.focus(); } });
type Block = {
  id: string;
  command: string;
  directory: string;
  output: string;
  running: boolean;
  background: boolean;
  duration: number;
  exitCode: number;
  error: string;
  collapsed: boolean;
  interactive: boolean;
};
const blocks = ref<Block[]>([]),
  command = ref(""),
  directory = ref(props.initialDirectory),
  mode = ref("command"),
  provider = ref(props.providers[0]?.id || ""),
  generating = ref(false),
  includeOutput = ref(false),
  suggestion = ref<TerminalSuggestion | null>(null),
  editorOpen = ref(false),
  historyOpen = ref(false),
  scroll = ref<HTMLElement>();
const references = ref<Block[]>([]),
  images = ref<{ id: string; name: string; data: string; size: number }[]>([]),
  imagePicker = ref<HTMLInputElement>(),
  input = ref<HTMLTextAreaElement>(),
  addingImages = ref(false);
const previewImage = ref("");
const { commands: historyCommands, remember, clear: clearHistory } = useCommandHistory(props.server.id);
const completionOpen = ref(false), completionIndex = ref(0),
  inputFocused = ref(false), cursorAtEnd = ref(true), ghostSuppressed = ref(false),
  composing = ref(false),
  inputScroll = ref({ left: 0, top: 0 });
let historyIndex = -1, historyDraft = "";
const pathCompletions = ref<string[]>([]);
let completionRevision = 0, completionTimer: ReturnType<typeof setTimeout> | undefined;
watch([command, directory, mode, () => props.closed], () => {
  const revision = ++completionRevision;
  if (completionTimer) clearTimeout(completionTimer);
  pathCompletions.value = [];
  if (mode.value === "ai" || props.closed || !inputFocused.value || composing.value || !cursorAtEnd.value) return;
  const query = pathCompletionQuery(command.value);
  if (!query) return;
  const text = command.value;
  completionTimer = setTimeout(async () => {
    try {
      const paths = await api("CompleteTerminalPath", props.connection, directory.value, query.word, query.directoriesOnly);
      if (disposed || revision !== completionRevision || command.value !== text || !inputFocused.value || !cursorAtEnd.value) return;
      pathCompletions.value = paths.filter(path => !/[\x00-\x1f\x7f]/.test(path)).map(path => query.before + quoteCompletion(path)).filter(value => value !== text);
      completionIndex.value = 0;
    } catch { /* Missing paths and permission errors must not interrupt typing. */ }
  }, 180);
});
const completions = computed(() => mode.value !== "ai" && command.value.trim() && !props.closed
  ? [...new Set([...historyCommands.value.filter((text) => text.startsWith(command.value) && text !== command.value), ...pathCompletions.value])].slice(0, 20)
  : []);
const preview = computed(() => completions.value.find(text => text.startsWith(command.value) && !text.includes("\n")));
const ghost = computed(() => inputFocused.value && cursorAtEnd.value && !composing.value && !completionOpen.value && !ghostSuppressed.value
  && !command.value.includes("\n")
  ? preview.value?.slice(command.value.length) || "" : "");
function updateCursor() {
  const element = input.value;
  cursorAtEnd.value = !element || (element.selectionStart === command.value.length && element.selectionEnd === command.value.length);
  if (!cursorAtEnd.value) completionOpen.value = false;
}
function commandInput() {
  historyIndex = -1;
  historyDraft = "";
  completionOpen.value = false;
  completionIndex.value = 0;
  ghostSuppressed.value = false;
  updateCursor();
}
function fillCommand(text: string) {
  completionRevision++;
  pathCompletions.value = [];
  command.value = text;
  completionOpen.value = false;
  ghostSuppressed.value = true;
  void nextTick(() => {
    input.value?.focus();
    input.value?.setSelectionRange(text.length, text.length);
    updateCursor();
  });
}
function complete(text: string) {
  historyIndex = -1;
  historyDraft = "";
  fillCommand(text);
}
function status(block: Block) {
  return block.running ? "running" : block.error || block.exitCode !== 0 ? "error" : "success";
}
function referenceBlock(block: Block) {
  if (!references.value.includes(block)) {
    if (references.value.length >= 4) {
      emit("error", "最多引用 4 个命令块，请先移除不需要的引用");
      return;
    }
    references.value.push(block);
  }
  mode.value = "ai";
  void nextTick(() => input.value?.focus());
}
async function addImages(files: File[]) {
  if (addingImages.value || generating.value) return;
  addingImages.value = true;
  try {
    for (const file of files) {
      if (images.value.length >= 4) throw new Error("最多附加 4 张图片");
      if (!["image/png", "image/jpeg", "image/gif"].includes(file.type))
        throw new Error("请使用 PNG、JPEG 或 GIF 图片");
      if (file.size > 8 * 1024 * 1024) throw new Error("每张图片不能超过 8 MB");
      if (images.value.reduce((n, image) => n + image.size, 0) + file.size > 16 * 1024 * 1024)
        throw new Error("图片合计不能超过 16 MB");
      const data = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result));
        reader.onerror = () => reject(new Error("读取图片失败"));
        reader.onabort = () => reject(new Error("读取图片已取消"));
        reader.readAsDataURL(file);
      });
      if (disposed) return;
      images.value.push({ id: uid(), name: file.name || "截图", data, size: file.size });
      mode.value = "ai";
    }
  } catch (e) {
    emit("error", e);
  } finally {
    addingImages.value = false;
  }
}
function paste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files || []);
  if (!files.length) return;
  event.preventDefault();
  void addImages(files);
}
function drop(event: DragEvent) {
  const files = Array.from(event.dataTransfer?.files || []);
  if (!files.length) return;
  event.preventDefault();
  void addImages(files);
}
function pickImages(event: Event) {
  const picker = event.target as HTMLInputElement;
  void addImages(Array.from(picker.files || []));
  picker.value = "";
}
const foreground = computed(() =>
  blocks.value.some((b) => b.running && !b.background),
);
const activeCount = computed(
  () => blocks.value.filter((b) => b.running).length,
);
let disposed = false,
  request = "";
const timers = new Set<ReturnType<typeof setTimeout>>();
async function bottom() {
  await nextTick();
  if (scroll.value) scroll.value.scrollTop = scroll.value.scrollHeight;
}
function defer(callback: () => void) {
  const timer = setTimeout(() => {
    timers.delete(timer);
    callback();
  }, 150);
  timers.add(timer);
}
async function run(background = mode.value === "background") {
  if (
    props.closed ||
    !command.value.trim() ||
    (!background && foreground.value)
  )
    return;
  const text = command.value;
  const block: Block = {
    id: "",
    command: text,
    directory: directory.value,
    output: "",
    running: true,
    background,
    duration: 0,
    exitCode: -1,
    error: "",
    collapsed: false,
    interactive: false,
  };
  blocks.value.push(block);
  // Keep running blocks; completed history is bounded to avoid unbounded terminal memory.
  if (blocks.value.length > 100) {
    const index = blocks.value.findIndex((b) => !b.running);
    if (index >= 0) blocks.value.splice(index, 1);
  }
  const current = blocks.value.at(-1)!;
  command.value = "";
  void bottom();
  try {
    current.id = await api(
      "StartCommand",
      props.connection,
      text,
      current.directory,
    );
    remember(text);
    historyIndex = -1;
    completionOpen.value = false;
    if (disposed) {
      await api("CancelCommand", props.connection, current.id);
      return;
    }
    const decoder = new TextDecoder();
    const poll = async () => {
      try {
        const result = await api("ReadCommand", props.connection, current.id);
        if (disposed) return;
        const stick =
          !scroll.value ||
          scroll.value.scrollHeight -
            scroll.value.scrollTop -
            scroll.value.clientHeight <
            80;
        current.output += decoder.decode(
          Uint8Array.from(atob(result.data), (c) => c.charCodeAt(0)),
          { stream: !result.closed },
        );
        if (current.output.length > 262144)
          current.output =
            "[较早输出已截断]\r\n" + current.output.slice(-240000);
        current.duration = result.duration;
        if (result.closed) {
          current.running = false;
          current.exitCode = result.exitCode;
          current.error = result.error;
          if (!background && result.directory.startsWith("/"))
            directory.value = result.directory;
        } else defer(() => void poll());
        if (stick) void bottom();
      } catch (e) {
        if (!disposed) {
          current.running = false;
          current.error = String(e);
          emit("error", e);
        }
      }
    };
    void poll();
  } catch (e) {
    if (!disposed) {
      current.running = false;
      current.error = String(e);
      emit("error", e);
    }
  }
}
async function generate() {
  if (
    generating.value ||
    (!command.value.trim() && !images.value.length && !references.value.length) ||
    !provider.value ||
    addingImages.value ||
    props.closed
  )
    return;
  generating.value = true;
  suggestion.value = null;
  request = uid();
  try {
    const last = blocks.value.at(-1);
    const selected = [...references.value];
    if (includeOutput.value && last && !selected.includes(last)) selected.push(last);
    const text = selected.map((block, index) =>
      `--- 引用命令块 ${index + 1} ---\n目录: ${block.directory}\n状态: ${status(block)}; 退出码: ${block.exitCode}\n命令:\n${block.command.slice(0, 2000)}\n输出（末尾）:\n${block.output.slice(-5000)}\n${block.error}`,
    ).join("\n\n");
    const bytes = new TextEncoder().encode(text);
    const output = bytes.length > 30000
      ? new TextDecoder().decode(bytes.slice(0, 29900)) + "\n[引用内容已截断]"
      : text;
    const result = await api(
      "SuggestTerminalCommand",
      props.connection,
      provider.value,
      command.value.trim() || "请分析附带的命令块或截图，解释问题并给出建议。",
      directory.value,
      output,
      request,
      images.value.map((image) => image.data),
    );
    if (!disposed) suggestion.value = result;
  } catch (e) {
    if (!disposed) emit("error", e);
  } finally {
    generating.value = false;
  }
}
function submit() {
  if (mode.value === "ai") void generate();
  else void run();
}
function key(event: KeyboardEvent) {
  if (event.isComposing || event.shiftKey || event.ctrlKey || event.altKey || event.metaKey) return;
  updateCursor();
  if (mode.value !== "ai" && completionOpen.value && completions.value.length) {
    if (event.key === "Escape") {
      event.preventDefault(); completionRevision++; completionOpen.value = false; ghostSuppressed.value = true; return;
    }
    if (event.key === "ArrowUp" || event.key === "ArrowDown") {
      event.preventDefault();
      completionIndex.value = (completionIndex.value + (event.key === "ArrowDown" ? 1 : -1) + completions.value.length) % completions.value.length;
      void nextTick(() => input.value?.parentElement?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" }));
      return;
    }
    if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault(); complete(completions.value[completionIndex.value] || completions.value[0]!); return;
    }
  }
  if (mode.value !== "ai" && cursorAtEnd.value) {
    if (event.key === "ArrowRight" && ghost.value) {
      event.preventDefault(); complete(preview.value!); return;
    }
    if (event.key === "Tab") {
      event.preventDefault();
      if (completions.value.length) { completionIndex.value = 0; completionOpen.value = true; }
      return;
    }
  }
  if ((mode.value === "command" || mode.value === "background") && (event.key === "ArrowUp" || event.key === "ArrowDown")) {
    const element = input.value;
    const start = element?.selectionStart ?? command.value.length;
    const end = element?.selectionEnd ?? start;
    const boundary = start === end && (event.key === "ArrowUp"
      ? !/[\r\n]/.test(command.value.slice(0, start))
      : !/[\r\n]/.test(command.value.slice(end)));
    if (historyIndex >= 0 || boundary) {
      if (event.key === "ArrowUp" && historyCommands.value.length) {
        event.preventDefault();
        if (historyIndex < 0) historyDraft = command.value;
        historyIndex = Math.min(historyIndex + 1, historyCommands.value.length - 1);
        fillCommand(historyCommands.value[historyIndex]!); return;
      }
      if (event.key === "ArrowDown" && historyIndex >= 0) {
        event.preventDefault(); historyIndex--;
        fillCommand(historyIndex < 0 ? historyDraft : historyCommands.value[historyIndex]!); return;
      }
    }
  }
  if (
    event.key === "Enter" &&
    !event.shiftKey &&
    !event.isComposing &&
    !event.ctrlKey &&
    !event.altKey
  ) {
    event.preventDefault();
    submit();
  }
}
async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (e) {
    emit("error", e);
  }
}
function useSuggestion() {
  if (!suggestion.value?.command) return;
  command.value = suggestion.value.command;
  mode.value = "command";
  suggestion.value = null;
}
onBeforeUnmount(() => {
  disposed = true;
  completionRevision++;
  if (completionTimer) clearTimeout(completionTimer);
  timers.forEach(clearTimeout);
  if (generating.value) void api("CancelGeneration", request).catch(() => {});
});
</script>
<template>
  <section class="enhanced-terminal" :class="{ 'ai-open': mode === 'ai', 'raw-open': blocks.some(block => block.interactive && block.running) }">
    <div ref="scroll" class="command-blocks" aria-label="增强终端命令结果">
      <article
        v-for="(block, index) in blocks"
        :key="index"
        class="command-block"
        :class="['is-' + status(block), { 'raw-command': block.interactive && block.running }]"
        :aria-label="status(block) === 'running' ? '命令运行中' : status(block) === 'error' ? '命令失败' : '命令成功'"
      >
        <header>
          <span class="command-directory" :title="block.directory">{{
            block.directory
          }}</span>
          <span class="command-duration"
            >({{
              block.duration < 1000
                ? block.duration + "ms"
                : (block.duration / 1000).toFixed(2) + "s"
            }})</span
          >
          <span v-if="block.background" class="muted">后台</span>
          <span v-if="block.interactive && block.running" class="muted">交互终端</span>
          <span v-if="block.running" class="text-primary">执行中</span>
          <span v-else-if="block.exitCode !== 0" class="text-error"
            >退出 {{ block.exitCode }}</span
          >
          <span v-else-if="block.error" class="text-error">失败</span>
          <span v-else class="text-success">成功</span>
          <span class="spacer" />
          <IconButton v-if="block.running && !block.interactive" icon="mdi-console" label="打开交互终端" size="x-small" @click="block.interactive = true; block.collapsed = false" />
          <div class="block-actions">
            <IconButton
              :icon="block.collapsed ? 'mdi-chevron-down' : 'mdi-chevron-up'"
              label="折叠或展开输出"
              :disabled="block.interactive && block.running"
              size="x-small"
              @click="block.collapsed = !block.collapsed"
            />
            <IconButton
              v-if="block.running"
              icon="mdi-stop"
              label="停止命令"
              size="x-small"
              @click="
                api('CancelCommand', connection, block.id).catch((e) =>
                  emit('error', e),
                )
              "
            />
            <IconButton
              icon="mdi-format-quote-open"
              label="引用命令块"
              size="x-small"
              :disabled="generating"
              @click="referenceBlock(block)"
            />
            <IconButton
              icon="mdi-content-copy"
              label="复制命令与输出"
              size="x-small"
              @click="copy(block.command + '\n' + block.output)"
            />
            <IconButton
              icon="mdi-replay"
              label="填回输入框"
              size="x-small"
              @click="
                command = block.command;
                mode = 'command';
              "
            />
          </div>
        </header>
        <pre class="block-command">{{ block.command }}</pre>
        <div
          v-if="block.id && !block.collapsed"
          class="block-output"
          :style="{
            height:
              Math.min(
                340,
                Math.max(38, (block.output.split('\n').length + 1) * 19),
              ) + 'px',
          }"
        >
          <TerminalView :output="block.output" :command-id="block.id" :command-connection="connection" :running="block.running" :interactive="block.interactive" @interactive="block.interactive = $event" @error="emit('error', $event)" />
        </div>
        <p v-if="block.error" class="text-error pa-3">{{ block.error }}</p>
      </article>
    </div>
    <div class="command-entry" @paste="paste" @dragover="($event.dataTransfer?.types.includes('Files')) && $event.preventDefault()" @drop="drop">
    <div v-if="mode === 'ai' && images.length" class="terminal-images" aria-label="待发送的截图">
      <figure v-for="(image, index) in images" :key="image.id">
        <button type="button" :title="'查看 ' + image.name" @click="previewImage = image.data"><img :src="image.data" :alt="image.name" /></button>
        <figcaption :title="image.name">{{ image.name }}</figcaption>
        <IconButton icon="mdi-close" label="移除截图" size="x-small" :disabled="generating" @click="images.splice(index, 1)" />
      </figure>
    </div>
    <p v-if="mode === 'ai' && !suggestion && !references.length && !images.length" class="terminal-ai-hint">引用命令块，或粘贴 / 拖入截图后提问。截图需选择支持图片的模型；内容仅在发送时提交。</p>
    <div v-if="suggestion" class="terminal-suggestion">
      <header>
        <v-icon size="18" color="primary">mdi-auto-fix</v-icon
        ><strong>AI 回复</strong><span class="spacer" /><IconButton
          icon="mdi-close"
          label="关闭建议"
          @click="suggestion = null"
        />
      </header>
      <p>{{ suggestion.explanation }}</p>
      <pre v-if="suggestion.command">{{ suggestion.command }}</pre>
      <v-btn
        v-if="suggestion.command"
        size="small"
        color="primary"
        @click="useSuggestion"
        >填入命令，核对后执行</v-btn
      >
    </div>
    <div class="terminal-composer">
      <div class="terminal-context">
        <IconButton
          icon="mdi-history"
          label="命令历史"
          size="small"
          @click="historyOpen = true"
        /><label class="terminal-cwd-chip"
          ><v-icon size="15">mdi-folder-outline</v-icon
          ><input
            v-model="directory"
            aria-label="增强终端工作目录"
            :disabled="foreground" /></label
        ><span class="terminal-prompt"
          >[{{ server.user }}@{{ server.name }}
          {{ directory === initialDirectory ? "~" : directory }}]$</span
        ><span class="spacer" /><IconButton
          icon="mdi-arrow-expand"
          label="弹窗编辑命令"
          @click="editorOpen = true"
        />
      </div>
      <div v-if="mode === 'ai' && references.length" class="terminal-references" aria-label="已引用的命令块">
        <div v-for="(block, index) in references" :key="index" class="terminal-reference" :class="'is-' + status(block)" :title="block.directory + '\n' + block.command">
          <v-icon size="14">mdi-console-line</v-icon><span>{{ block.command }}</span>
          <button type="button" class="terminal-reference-remove" aria-label="移除命令引用" :disabled="generating" @click="references.splice(index, 1)"><v-icon size="13">mdi-close</v-icon></button>
        </div>
      </div>
      <div class="terminal-input">
      <div v-if="completionOpen && completions.length" :id="'command-completions-' + connection" class="terminal-completions" role="listbox" aria-label="命令与远端路径补全">
        <button v-for="(text, index) in completions" :key="text" type="button" role="option"
          :id="'command-completion-' + connection + '-' + index" :aria-selected="index === completionIndex" :title="text"
          @mousedown.prevent @click="complete(text)">
          <v-icon size="14">{{ pathCompletions.includes(text) ? 'mdi-folder-outline' : 'mdi-history' }}</v-icon><span>{{ text }}</span>
        </button>
        <small>↑ ↓ 选择 · Tab / Enter 填入 · Esc 关闭</small>
      </div>
      <textarea
        ref="input"
        v-model="command"
        :placeholder="
          mode === 'ai'
            ? '输入问题，可粘贴截图；AI 不会自动执行命令'
            : '输入命令后回车执行'
        "
        spellcheck="false"
        wrap="off"
        aria-label="增强终端命令输入"
        :aria-expanded="completionOpen && !!completions.length"
        aria-autocomplete="both"
        :aria-controls="'command-completions-' + connection"
        :aria-activedescendant="completionOpen && completions.length ? 'command-completion-' + connection + '-' + completionIndex : undefined"
        @compositionstart="composing = true"
        @compositionend="composing = false; commandInput()"
        @input="commandInput"
        @focus="inputFocused = true; updateCursor()"
        @blur="inputFocused = false; completionOpen = false"
        @click="updateCursor"
        @keyup="updateCursor"
        @scroll="inputScroll = { left: input?.scrollLeft || 0, top: input?.scrollTop || 0 }"
        @keydown="key"
      />
      <div v-if="ghost" class="terminal-ghost" aria-hidden="true"><pre :style="{ transform: `translate(${-inputScroll.left}px, ${-inputScroll.top}px)` }"><span>{{ command }}</span>{{ ghost }} <kbd>→</kbd></pre></div>
      </div>
      <footer>
        <v-btn-toggle v-model="mode" mandatory density="compact" color="primary"
          ><v-btn value="command" size="small" prepend-icon="mdi-console"
            >执行</v-btn
          ><v-btn value="ai" size="small" prepend-icon="mdi-auto-fix">AI</v-btn
          ><v-btn
            value="background"
            size="small"
            prepend-icon="mdi-play-circle-outline"
            >后台执行</v-btn
          ></v-btn-toggle
        >
        <v-select
          v-model="provider"
          :items="
            providers.map((p) => ({ ...p, display: p.name + ' · ' + p.model }))
          "
          item-title="display"
          item-value="id"
          aria-label="选择模型"
          placeholder="添加模型"
          variant="solo-filled"
          flat
          hide-details
          density="compact"
          class="terminal-provider"
          :title="
            providers.find((p) => p.id === provider)?.model ||
            '在设置中配置模型'
          "
        />
        <IconButton
          v-if="mode === 'ai'"
          icon="mdi-image-plus-outline"
          label="添加截图（也可 Ctrl+V 粘贴）"
          :disabled="generating || addingImages"
          @click="imagePicker?.click()"
        />
        <input ref="imagePicker" type="file" accept="image/png,image/jpeg,image/gif" multiple hidden @change="pickImages" />
        <IconButton
          v-if="mode === 'ai'"
          icon="mdi-text-box-outline"
          :color="includeOutput ? 'primary' : undefined"
          :label="
            includeOutput
              ? '已附上最近命令和输出；点击取消'
              : '将最近命令和输出附给模型（当前关闭）'
          "
          @click="includeOutput = !includeOutput"
        />
        <span v-if="mode !== 'ai' && activeCount" class="muted"
          >{{ activeCount }} 个任务运行中</span
        ><span class="spacer" />
        <IconButton
          icon="mdi-broom"
          label="清空已完成的命令块"
          @click="blocks = blocks.filter((b) => b.running)"
        />
        <v-btn
          v-if="generating"
          size="small"
          @click="api('CancelGeneration', request)"
          >取消生成</v-btn
        >
        <v-btn
          color="primary"
          variant="flat"
          size="small"
          :loading="generating"
          icon="mdi-arrow-up"
          :title="mode === 'ai' ? '生成命令建议' : '执行命令'"
          :disabled="
            closed ||
            addingImages ||
            (!command.trim() && (mode !== 'ai' || (!images.length && !references.length))) ||
            (mode === 'ai' ? !provider : mode === 'command' && foreground)
          "
          @click="submit"
        />
      </footer>
    </div>
    </div>
    <CodeEditorDialog
      v-model:open="editorOpen"
      v-model="command"
      title="编辑终端命令"
    />
    <v-dialog :model-value="!!previewImage" max-width="1100" @update:model-value="!$event && (previewImage = '')">
      <v-card><v-card-actions><span>截图预览</span><v-spacer /><IconButton icon="mdi-close" label="关闭截图预览" @click="previewImage = ''" /></v-card-actions>
        <img :src="previewImage" alt="截图预览" style="max-width: 100%; max-height: 80vh; object-fit: contain" />
      </v-card>
    </v-dialog>
    <v-dialog v-model="historyOpen" max-width="720"
      ><v-card title="当前服务器命令历史"
        ><v-card-text class="terminal-history"
          ><button
            v-for="text in historyCommands"
            :key="text"
            @click="
              complete(text);
              mode = 'command';
              historyOpen = false;
            "
          >
            <pre>{{ text }}</pre>
          </button>
          <p v-if="!historyCommands.length" class="muted">暂无命令记录</p></v-card-text
        ><v-card-actions
          ><v-btn :disabled="!historyCommands.length" @click="clearHistory">清空历史</v-btn><v-spacer /><v-btn @click="historyOpen = false"
            >关闭</v-btn
          ></v-card-actions
        ></v-card
      ></v-dialog
    >
  </section>
</template>
<style scoped>
.enhanced-terminal {
  position: relative;
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #191a1b;
  color: #d7dce3;
  font: 500 14px/1.35 var(--terminal-font-family);
}
.command-blocks {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0;
}
.command-block {
  position: relative;
  border-bottom: 1px solid #2b2c2e;
  padding: 8px 0 10px 8px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.008);
  box-shadow: inset 3px 0 transparent;
  transition: background .16s ease, box-shadow .16s ease;
}
.command-block:hover {
  background: rgba(255, 255, 255, 0.025);
}
.command-block.raw-command { position: absolute; inset: 0; z-index: 20; display: flex; flex-direction: column; background: #191a1b; padding: 8px; }
.command-block.raw-command:hover { background: #191a1b; }
.raw-open .command-entry { display: none; }
.raw-command .block-output { flex: 1; min-height: 0; height: auto !important; }
.raw-command .block-actions { opacity: 1; }
.command-block.is-running { box-shadow: inset 3px 0 rgb(var(--v-theme-primary)); }
.command-block.is-running:hover { background: rgba(var(--v-theme-primary), .08); }
.command-block.is-success { box-shadow: inset 3px 0 rgb(var(--v-theme-success)); }
.command-block.is-success:hover { background: rgba(var(--v-theme-success), .06); }
.command-block.is-error { box-shadow: inset 3px 0 rgb(var(--v-theme-error)); background: rgba(var(--v-theme-error), .07); }
.command-block.is-error:hover { background: rgba(var(--v-theme-error), .1); }
.block-output :deep(.xterm-screen) { background: transparent; }
.command-entry { display: contents; }
.ai-open .command-entry { display: flex; flex-direction: column; flex: 0 1 auto; max-height: 65%; min-height: 0; overflow: auto; border-top: 1px solid #303133; }
.terminal-references { padding: 0 12px 8px; display: flex; flex-wrap: wrap; gap: 8px; min-width: 0; }
.terminal-reference { position: relative; display: inline-flex; align-items: center; gap: 5px; max-width: min(360px, 100%); height: 26px; border: 1px solid rgba(var(--v-theme-on-surface), .08); border-radius: 999px; padding: 0 28px 0 8px; color: rgb(var(--v-theme-primary)); background: rgba(var(--v-theme-primary), .08); }
.terminal-reference.is-error { border-color: rgba(var(--v-theme-error), .2); background: rgba(var(--v-theme-error), .1); color: rgb(var(--v-theme-error)); }
.terminal-reference > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; line-height: 1; }
.terminal-reference-remove { position: absolute; right: 4px; width: 20px; height: 20px; display: grid; place-items: center; border-radius: 50%; }
.terminal-reference-remove:hover { background: rgba(var(--v-theme-on-surface), .08); }
.terminal-reference-remove:focus-visible { outline: 1px solid currentColor; }
.terminal-images { display: flex; flex-wrap: wrap; gap: 8px; padding: 8px; flex-shrink: 0; }
.terminal-images figure { position: relative; width: 110px; margin: 0; border: 1px solid #36383b; border-radius: 5px; overflow: hidden; }
.terminal-images img { display: block; width: 100%; height: 74px; object-fit: contain; background: #111; }
.terminal-images figcaption { padding: 2px 5px; font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.terminal-images figure > :deep(.v-btn) { position: absolute; top: 0; right: 0; background: #202122; }
.terminal-ai-hint { color: #999a9d; font-size: 12px; padding: 12px; }
.ai-open .terminal-suggestion { max-height: 240px; flex: 0 0 auto; }
.command-block > header,
.terminal-suggestion > header {
  min-height: 28px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px 0 0;
  font-size: 13px;
  color: #999a9d;
}
.command-directory {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 65%;
  font-weight: 600;
}
.command-duration {
  flex-shrink: 0;
  font-size: 12px;
}
.block-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  border: 1px solid #333537;
  border-radius: 5px;
  background: #202122;
}
.command-block:hover .block-actions,
.command-block:focus-within .block-actions {
  opacity: 1;
}
.block-command {
  padding: 0 8px 0 0;
  margin: 4px 0 8px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font: 700 14px/1.35 var(--terminal-font-family);
  color: #dcdcdc;
}
.block-output {
  display: flex;
  min-height: 0;
}
.block-output :deep(.ssh-terminal) {
  padding: 0 8px 0 0;
  background: transparent;
}
.terminal-composer {
  position: relative;
  flex-shrink: 0;
  margin: 0 5px 6px;
  border: 1px solid #303133;
  border-radius: 8px;
  background: #1d1e1f;
  overflow: visible;
}
.terminal-context {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 10px 12px 4px;
  color: #aaa;
  overflow: hidden;
}
.terminal-cwd-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 9px;
  border: 1px solid #333537;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.035);
  min-width: 0;
  max-width: 55%;
  flex: 0 1 auto;
}
.terminal-context input {
  width: 100%;
  field-sizing: content;
  min-width: 35px;
  outline: none;
  color: #c6c7ca;
  font: 600 14px var(--terminal-font-family);
}
.terminal-prompt {
  height: 28px;
  display: inline-flex;
  align-items: center;
  padding: 0 9px;
  min-width: 0;
  flex: 0 999 auto;
  color: #83b66c;
  border: 1px solid #303233;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.025);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: pre;
  font: 600 14px var(--terminal-font-family);
}
.terminal-input { position: relative; }
.terminal-input > textarea {
  display: block;
  width: 100%;
  height: 44px;
  min-height: 44px;
  max-height: 240px;
  padding: 8px 12px;
  outline: none;
  resize: vertical;
  white-space: pre;
  overflow: auto;
  font: 400 14px/1.45 var(--terminal-font-family);
  color: #e0e0e0;
}
.terminal-ghost { position: absolute; inset: 0; overflow: hidden; pointer-events: none; color: #71777e; }
.terminal-ghost pre { padding: 8px 12px; font: 400 14px/1.45 var(--terminal-font-family); white-space: pre; }
.terminal-ghost span { color: transparent; }
.terminal-ghost kbd { font: 11px Consolas, monospace; border: 1px solid #41464c; border-radius: 3px; padding: 0 3px; }
.terminal-completions { position: absolute; bottom: calc(100% + 6px); left: 8px; right: 8px; z-index: 30; max-height: 270px; overflow: auto; padding: 4px; background: #242628; border: 1px solid #45494e; border-radius: 5px; box-shadow: 0 4px 16px #0007; }
.terminal-completions button { display: flex; align-items: center; gap: 8px; width: 100%; text-align: left; min-height: 27px; padding: 3px 6px; color: #c9cdd2; }
.terminal-completions button span { overflow: hidden; text-overflow: ellipsis; white-space: pre; }
.terminal-completions button[aria-selected="true"], .terminal-completions button:hover { background: #354257; color: #dce9ff; }
.terminal-completions small { display: block; color: #91979f; padding: 4px 6px; font-size: 11px; }
.terminal-composer > footer {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  border-top: 1px solid #292b2c;
  padding: 4px 6px 5px;
  min-height: 36px;
}
.terminal-composer :deep(.v-btn-toggle) {
  height: 28px;
  border: 1px solid #303233;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.025);
  flex-shrink: 0;
}
.terminal-composer :deep(.v-btn-toggle .v-btn) {
  height: 28px;
  min-width: 0;
  padding: 0 7px;
  font-size: 12px;
  color: #979b9e;
}
.terminal-composer :deep(.v-btn--active) {
  color: #8cb3ee;
  background: rgba(135, 177, 239, 0.09);
}
.terminal-composer > footer > :deep(.v-btn--icon) {
  width: 28px;
  height: 28px;
  border-radius: 4px;
}
.terminal-provider {
  --v-input-control-height: 28px;
  min-width: 120px;
  max-width: 240px;
  flex: 1 1 120px;
}
.terminal-provider :deep(.v-field) {
  font-size: 11px;
  --v-input-control-height: 28px;
  --v-field-padding-top: 0;
  --v-field-padding-bottom: 0;
}
.terminal-provider :deep(.v-field__input) {
  min-height: 28px;
  padding-top: 2px;
  padding-bottom: 2px;
}
.terminal-provider :deep(.v-field-label) {
  font-size: 11px;
}
.terminal-suggestion {
  max-height: 40%;
  overflow: auto;
  margin: 0 6px 6px;
  border: 1px solid #363b43;
  border-radius: 6px;
  padding: 8px 12px;
  background: #202224;
}
.terminal-suggestion p {
  white-space: pre-wrap;
  margin: 8px 0;
  font-size: 12px;
}
.terminal-suggestion pre {
  white-space: pre;
  overflow: auto;
  font: 400 14px/1.45 var(--terminal-font-family);
  margin-bottom: 12px;
}
.terminal-history {
  max-height: 65vh;
  overflow: auto;
}
.terminal-history button {
  display: block;
  text-align: left;
  width: 100%;
  padding: 10px;
  border-bottom: 1px solid var(--border);
}
.terminal-history pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.terminal-history small {
  color: var(--muted);
}
</style>
