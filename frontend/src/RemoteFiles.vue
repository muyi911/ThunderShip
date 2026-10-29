<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from "vue";
import type { RemoteEntry, RemoteFileOperation, FileMapping, UploadPreview } from "./types";
import { api } from "./api";
import IconButton from "./IconButton.vue";
import ResizablePanel from "./ResizablePanel.vue";
import CodeEditorDialog from "./CodeEditorDialog.vue";
import RemoteFileMenu from "./RemoteFileMenu.vue";
import UploadConflictDialog from "./UploadConflictDialog.vue";
import { remoteFileClipboard } from "./remoteFileClipboard";
import { appendLocalFiles } from "./composables/useLocalPicker";
const props = defineProps<{
  connection: string;
  serverId?: string;
  initialDirectory: string;
  closed: boolean;
}>();
const emit = defineEmits<{ error: [unknown]; terminalDirectory: [string] }>();
const directory = ref(props.initialDirectory),
  input = ref(props.initialDirectory),
  entries = ref<RemoteEntry[]>([]),
  busy = ref(false),
  sudo = ref(false),
  upload = ref(false),
  executable = ref(false);
const history = ref<string[]>([]),
  recent = ref<string[]>([props.initialDirectory]),
  search = ref(""),
  searchOpen = ref(false),
  showHidden = ref(true),
  selected = ref(""),
  selectedNames = ref<string[]>([]),
  sort = ref<keyof RemoteEntry>("name"),
  descending = ref(false),
  menu = ref(false),
  menuX = ref(0),
  menuY = ref(0),
  contextEntry = ref<RemoteEntry | null>(null),
  properties = ref(false);
const editor = ref<{ path: string; content: string; version: string; sudo: boolean } | null>(null),
  editorOpen = ref(false),
  saving = ref(false),
  editorError = ref(""),
  saved = ref(false);
const operation = shallowRef<{
  action: RemoteFileOperation["action"];
  source: string;
  directory: string;
  sudo: boolean;
  isDirectory: boolean;
  clipboard: typeof remoteFileClipboard.value;
  items: { source: string; name: string; directory: boolean }[];
} | null>(null);
const operationValue = ref(""), operationError = ref("");
const uploadItems = ref<FileMapping[]>([]), uploadDirectory = ref(""), uploadSudo = ref(false), uploadError = ref("");
const uploadProtocol = ref(""), conflict = ref<UploadPreview | null>(null), localPaste = ref(false);
type ConflictDecision = { action: "overwrite" | "skip" | "cancel"; all: boolean };
let resolveConflict: ((decision: ConflictDecision) => void) | undefined;
function decideConflict(action: ConflictDecision["action"], all: boolean) {
  resolveConflict?.({ action, all });
  resolveConflict = undefined;
  conflict.value = null;
}
const favoritesKey = "thundership.file-favorites." + (props.serverId || props.connection);
const favorites = ref<string[]>([]);
try { const stored = JSON.parse(localStorage.getItem(favoritesKey) || "[]"); if (Array.isArray(stored)) favorites.value = stored.filter(item => typeof item === "string" && item.startsWith("/")); } catch { /* Ignore invalid local preferences. */ }
function toggleFavorite() {
  const path = directory.value;
  favorites.value = favorites.value.includes(path) ? favorites.value.filter(item => item !== path) : [...favorites.value, path];
  try { localStorage.setItem(favoritesKey, JSON.stringify(favorites.value)); } catch (e) { emit("error", e); }
}
const operationTitles: Record<RemoteFileOperation["action"], string> = {
  mkdir: "新建文件夹", create: "新建文件", rename: "重命名", copy: "粘贴副本", move: "移动到当前目录", delete: "删除", chmod: "修改权限", compress: "压缩为 tar.gz", extract: "解压到新文件夹",
};
const clipboardServer = computed(() => props.serverId || props.connection);
const canPaste = computed(() => !!remoteFileClipboard.value && remoteFileClipboard.value.server === clipboardServer.value && remoteFileClipboard.value.sudo === sudo.value);
const permissionValue = computed(() => /^[0-7]{3,4}$/.test(operationValue.value) ? parseInt(operationValue.value, 8) : 0);
let disposed = false;
onBeforeUnmount(() => { disposed = true; decideConflict("cancel", false); });
const columns: { key: keyof RemoteEntry; title: string }[] = [
  { key: "name", title: "名称" },
  { key: "modified", title: "修改时间" },
  { key: "directory", title: "类型" },
  { key: "size", title: "大小" },
  { key: "mode", title: "权限" },
  { key: "owner", title: "用户/组" },
];
const visible = computed(() =>
  entries.value
    .filter(
      (e) =>
        (showHidden.value || !e.name.startsWith(".")) &&
        e.name.toLowerCase().includes(search.value.toLowerCase()),
    )
    .sort((a, b) => {
      if (a.directory !== b.directory) return a.directory ? -1 : 1;
      const left = sort.value === "directory" ? type(a) : (a[sort.value] ?? ""),
        right = sort.value === "directory" ? type(b) : (b[sort.value] ?? "");
      const compared =
        typeof left === "number" && typeof right === "number"
          ? left - right
          : String(left).localeCompare(String(right), undefined, {
              numeric: true,
            });
      return descending.value ? -compared : compared;
    }),
);
const count = computed(() => ({
  files: visible.value.filter((e) => !e.directory).length,
  directories: visible.value.filter((e) => e.directory).length,
  size: visible.value
    .filter((e) => !e.directory)
    .reduce((sum, e) => sum + e.size, 0),
}));
function size(value: number) {
  if (value < 1024) return value + " B";
  const unit = Math.min(3, Math.floor(Math.log(value) / Math.log(1024)));
  return (
    (value / 1024 ** unit).toFixed(unit > 1 ? 1 : 0) +
    " " +
    ["B", "KB", "MB", "GB"][unit]
  );
}
function type(entry: RemoteEntry) {
  return entry.directory
    ? "文件夹"
    : entry.link
      ? "符号链接"
      : entry.name.includes(".")
        ? entry.name.split(".").at(-1)!.toUpperCase()
        : "文件";
}
function modified(value: string) {
  return value.replace("T", " ").slice(0, 16);
}
async function list(dir = directory.value, record = true) {
  if (busy.value || props.closed) return false;
  busy.value = true;
  try {
    const files = await api("ListRemote", props.connection, dir, sudo.value);
    if (record && directory.value !== dir) history.value.push(directory.value);
    recent.value = [dir, ...recent.value.filter((p) => p !== dir)].slice(0, 20);
    entries.value = files;
    directory.value = input.value = dir;
    selected.value = "";
    selectedNames.value = [];
    menu.value = false;
    return true;
  } catch (e) {
    emit("error", e);
    input.value = directory.value;
    return false;
  } finally {
    busy.value = false;
  }
}
function child(name: string) {
  return (directory.value === "/" ? "" : directory.value) + "/" + name;
}
function parent() {
  return list(directory.value.split("/").slice(0, -1).join("/") || "/");
}
async function back() {
  const path = history.value.at(-1);
  if (path && (await list(path, false))) history.value.pop();
}
async function download(file: RemoteEntry, protocol = "sftp") {
  if (busy.value || props.closed) return;
  busy.value = true;
  try {
    await api("DownloadRemoteWithProtocol", props.connection, child(file.name), sudo.value, protocol);
  } catch (e) {
    emit("error", e);
  } finally {
    busy.value = false;
  }
}
function open(entry: RemoteEntry) {
  if (entry.directory || entry.link) void list(child(entry.name));
  else void edit(entry);
}
async function edit(entry: RemoteEntry) {
  if (busy.value || props.closed || entry.directory || entry.link) return;
  const path = child(entry.name), useSudo = sudo.value;
  busy.value = true;
  menu.value = false;
  try {
    const text = await api("ReadRemoteText", props.connection, path, useSudo);
    if (disposed) return;
    editor.value = { path, content: text.content.replace(/\r\n/g, "\n"), version: text.version, sudo: useSudo };
    editorError.value = "";
    saved.value = false;
    editorOpen.value = true;
  } catch (e) {
    if (!disposed) emit("error", e);
  } finally {
    busy.value = false;
  }
}
async function saveText(content: string) {
  if (!editor.value || saving.value) return;
  if (props.closed) { editorError.value = "连接已关闭，请先复制并保留当前草稿。"; return; }
  const file = editor.value;
  saving.value = true;
  editorError.value = "";
  saved.value = false;
  try {
    const result = await api("SaveRemoteText", props.connection, file.path, content, file.version, file.sudo);
    if (disposed) return;
    // Keep the submitted draft as the baseline; the server retains its original line endings.
    file.content = content;
    file.version = result.version;
    saved.value = true;
    void list();
  } catch (e) {
    if (!disposed) editorError.value = String(e);
  } finally {
    saving.value = false;
  }
}
function sortBy(key: keyof RemoteEntry) {
  if (sort.value === key) descending.value = !descending.value;
  else {
    sort.value = key;
    descending.value = false;
  }
}
function context(event: MouseEvent, entry: RemoteEntry | null) {
  if (busy.value || props.closed) return;
  contextEntry.value = entry;
  selected.value = entry?.name || "";
  if (!entry || !selectedNames.value.includes(entry.name)) selectedNames.value = entry ? [entry.name] : [];
  menuX.value = event.clientX;
  menuY.value = event.clientY;
  menu.value = true;
  localPaste.value = false;
  void api("GetClipboardFiles").then(paths => { if (!disposed) localPaste.value = paths.length > 0; }).catch(() => {});
}
function menuAction(action: string) {
  menu.value = false;
  const entry = contextEntry.value;
  if (action === "terminal") emit("terminalDirectory", directory.value);
  else if (action === "favorite") toggleFavorite();
  else if (action === "open" && entry) open(entry);
  else if (action === "edit" && entry) void edit(entry);
  else if ((action === "download" || action === "scp-download") && entry) void download(entry, action === "scp-download" ? "scp" : "sftp");
  else if (action === "upload" || action === "scp-upload" || action === "upload-folder") void chooseUpload(action === "upload-folder", action === "scp-upload" ? "scp" : "");
  else if (action === "refresh") void list();
  else if (action === "paste") void pasteFiles();
  else if (action === "copy" || action === "cut") fileClipboard(action === "cut");
  else if (action === "path") void copyPath();
  else if (action === "properties") properties.value = true;
  else beginOperation(action as RemoteFileOperation["action"]);
}
function fileClipboard(cut: boolean, entry = contextEntry.value) {
  if (!entry || busy.value || props.closed) return;
  const items = (selectedNames.value.includes(entry.name) ? entries.value.filter(item => selectedNames.value.includes(item.name)) : [entry])
    .map(item => ({ name: item.name, path: child(item.name), directory: item.directory }));
  remoteFileClipboard.value = { server: clipboardServer.value, path: child(entry.name), name: entry.name, cut, sudo: sudo.value, items };
  void navigator.clipboard.writeText(items.map(item => item.path).join("\n")).catch(e => emit("error", e));
  menu.value = false;
}
function selectEntry(event: MouseEvent, entry: RemoteEntry) {
  if (event.shiftKey && selected.value) {
    const names = visible.value.map(item => item.name), start = names.indexOf(selected.value), end = names.indexOf(entry.name);
    selectedNames.value = names.slice(Math.min(start, end), Math.max(start, end) + 1);
  } else if (event.ctrlKey || event.metaKey) {
    selectedNames.value = selectedNames.value.includes(entry.name) ? selectedNames.value.filter(name => name !== entry.name) : [...selectedNames.value, entry.name];
  } else selectedNames.value = [entry.name];
  selected.value = entry.name;
  (event.currentTarget as HTMLElement).focus();
}
function modeNumber(mode: string) {
  let value = 0;
  for (let i = 0; i < 9; i++) {
    const char = mode[i + 1];
    if (char && char !== "-" && char !== "S" && char !== "T") value |= 1 << (8 - i);
  }
  if (/[sS]/.test(mode[3] || "")) value |= 0o4000;
  if (/[sS]/.test(mode[6] || "")) value |= 0o2000;
  if (/[tT]/.test(mode[9] || "")) value |= 0o1000;
  return value.toString(8).padStart(3, "0");
}
function setPermission(bit: number, value: boolean) {
  operationValue.value = (value ? permissionValue.value | bit : permissionValue.value & ~bit).toString(8).padStart(3, "0");
}
function beginOperation(action: RemoteFileOperation["action"], entry = contextEntry.value) {
  if (busy.value || props.closed) return;
  if (["rename", "delete", "chmod", "compress", "extract"].includes(action) && !entry) return;
  if (action === "chmod" && entry?.link) return;
  const clipboard = remoteFileClipboard.value;
  if ((action === "copy" || action === "move") && !canPaste.value) return;
  let name = entry?.name || "";
  if (action === "copy" || action === "move") name = clipboard!.name;
  if (action === "copy" && clipboard!.path === child(name)) {
    const dot = name.lastIndexOf(".");
    name = dot > 0 ? name.slice(0, dot) + "-副本" + name.slice(dot) : name + "-副本";
  }
  operationValue.value = action === "chmod" ? modeNumber(entry!.mode || "") : action === "mkdir" || action === "create" ? "" : name;
  if (action === "compress") operationValue.value = name + ".tar.gz";
  if (action === "extract") operationValue.value = name.replace(/\.(tar\.gz|tgz|tar|zip)$/i, "") + "-解压";
  operationError.value = "";
  const items = action === "copy" || action === "move"
    ? (clipboard!.items || [{ path: clipboard!.path, name: clipboard!.name, directory: false }]).map(item => {
        let name = item.name;
        if (action === "copy" && item.path === child(name)) {
          const dot = name.lastIndexOf('.');
          name = !item.directory && dot > 0 ? name.slice(0, dot) + '-副本' + name.slice(dot) : name + '-副本';
        }
        return { source: item.path, name, directory: item.directory };
      })
    : (action === "delete" && entry && selectedNames.value.includes(entry.name)
      ? entries.value.filter(item => selectedNames.value.includes(item.name)) : entry ? [entry] : [])
      .map(item => ({ source: child(item.name), name: item.name, directory: item.directory }));
  operation.value = { action, source: action === "copy" || action === "move" ? clipboard!.path : entry ? child(entry.name) : "", directory: directory.value, sudo: sudo.value, isDirectory: !!entry?.directory, clipboard, items };
  menu.value = false;
}
function paste() {
  beginOperation(remoteFileClipboard.value?.cut ? "move" : "copy", null);
}
async function applyOperation() {
  const pending = operation.value;
  if (!pending || busy.value || props.closed) return;
  const action = pending.action, name = operationValue.value;
  if (pending.items.length <= 1 && !["delete", "chmod"].includes(action) && (!name.trim() || name === "." || name === ".." || /[/\\\0\r\n]/.test(name))) {
    operationError.value = "请输入单个文件或文件夹名称，不能包含路径分隔符。";
    return;
  }
  if (action === "chmod" && !/^[0-7]{3,4}$/.test(name)) {
    operationError.value = "权限请输入 3 或 4 位八进制数字，例如 644、755。";
    return;
  }
  const destination = (pending.directory === "/" ? "" : pending.directory) + "/" + name;
  const request: RemoteFileOperation = { action, source: action === "mkdir" || action === "create" ? destination : pending.source, target: ["copy", "move", "rename", "compress", "extract"].includes(action) ? destination : "", mode: action === "chmod" ? name : "" };
  busy.value = true;
  operationError.value = "";
  let completed = 0;
  try {
    for (const item of pending.items.length > 1 ? pending.items : [null]) {
      const batchRequest = item ? { ...request, source: item.source, target: ["copy", "move"].includes(action) ? (pending.directory === '/' ? '' : pending.directory) + '/' + item.name : "" } : request;
      await api("ManageRemoteFile", props.connection, batchRequest, pending.sudo);
      completed++;
    }
    if (disposed) return;
    if ((action === "move" && remoteFileClipboard.value === pending.clipboard) || (["rename", "delete"].includes(action) && remoteFileClipboard.value?.server === clipboardServer.value && [pending.source, ...pending.items.map(item => item.source)].includes(remoteFileClipboard.value.path))) remoteFileClipboard.value = null;
    operation.value = null;
  } catch (e) {
    if (!disposed) {
      operationError.value = (completed ? `已完成 ${completed} 项；剩余项目未完成：` : '') + String(e);
      if (completed && pending.items.length > 1) {
        const remaining = pending.items.slice(completed);
        operation.value = { ...pending, source: remaining[0]!.source, items: remaining };
        operationValue.value = remaining[0]!.name;
        if (action === 'move' || action === 'delete') remoteFileClipboard.value = null;
      }
      busy.value = false;
      await list(pending.directory, false);
    }
    return;
  } finally {
    busy.value = false;
  }
  if (!disposed) {
    await list(pending.directory, false);
    if (action === "create") {
      const created = entries.value.find(entry => entry.name === name);
      if (created) await edit(created);
    }
  }
}
function fileKeys(event: KeyboardEvent) {
  if (["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes((event.target as HTMLElement)?.tagName)) return;
  const entry = entries.value.find(item => item.name === selected.value && (!selectedNames.value.length || selectedNames.value.includes(item.name))) || entries.value.find(item => selectedNames.value.includes(item.name)) || null;
  const command = event.ctrlKey || event.metaKey;
  const key = event.key.toLowerCase();
  if (command && key === 'a') { selectedNames.value = visible.value.map(item => item.name); selected.value = selectedNames.value[0] || ''; }
  else if (event.key === "F2" && entry && selectedNames.value.length <= 1) beginOperation("rename", entry);
  else if ((event.key === "Delete" || event.key === "Backspace") && entry) beginOperation("delete", entry);
  else if (command && event.altKey && key === "c") { contextEntry.value = entry; void copyPath(); }
  else if (command && event.altKey && key === "e" && entry) void edit(entry);
  else if (command && event.altKey && key === "n") beginOperation("create", null);
  else if (command && event.altKey && key === "m" && entry) beginOperation("chmod", entry);
  else if (command && (key === "c" || key === "x") && entry) fileClipboard(key === "x", entry);
  else if (command && key === "v") void pasteFiles();
  else if (command && event.shiftKey && key === "n") beginOperation("mkdir", null);
  else if (event.key === "F5") void list();
  else if (event.altKey && event.key === "ArrowUp") void parent();
  else return;
  event.preventDefault();
  event.stopPropagation();
}
async function copyPath() {
  try {
    await navigator.clipboard.writeText(
      contextEntry.value ? child(contextEntry.value.name) : directory.value,
    );
  } catch (e) {
    emit("error", e);
  }
}
function prepareUpload(items: FileMapping[], destination: string, useSudo: boolean) {
  if (!items.length || disposed) return;
  if (upload.value && uploadDirectory.value === destination && uploadSudo.value === useSudo) appendLocalFiles(uploadItems.value, items);
  else uploadItems.value = items;
  uploadDirectory.value = destination;
  uploadSudo.value = useSudo;
  uploadError.value = "";
  menu.value = false;
  upload.value = true;
}
async function chooseUpload(isFolder: boolean, protocol?: string) {
  if (busy.value || props.closed) return;
  const destination = directory.value, useSudo = sudo.value;
  busy.value = true;
  try { if (protocol !== undefined) uploadProtocol.value = protocol; else if (!upload.value) uploadProtocol.value = ""; prepareUpload(await api("PickFiles", isFolder), destination, useSudo); }
  catch (e) { if (!disposed) emit("error", e); }
  finally { busy.value = false; }
}
async function stageLocal(paths: string[], destination = directory.value) {
  if (busy.value || props.closed || !paths.length) return;
  const useSudo = sudo.value;
  busy.value = true;
  try { if (!upload.value) uploadProtocol.value = ""; prepareUpload(await api("ResolveLocalPaths", paths), destination, useSudo); }
  catch (e) { if (!disposed) emit("error", e); }
  finally { busy.value = false; }
}
function localDrop(event: Event) {
  const { paths, directory: destination } = (event as CustomEvent<{paths: string[]; directory?: string}>).detail;
  void stageLocal(paths, destination || directory.value);
}
async function pasteFiles() {
  if (busy.value || props.closed) return;
  menu.value = false;
  try {
    const paths = await api("GetClipboardFiles");
    if (disposed) return;
    if (paths.length) await stageLocal(paths);
    else if (canPaste.value) paste();
    else emit("error", "请先在资源管理器或服务器文件列表中复制文件。");
  } catch (e) { if (!disposed) emit("error", e); }
}
async function send() {
  if (busy.value || props.closed || !uploadItems.value.length) return;
  const destination = uploadDirectory.value;
  busy.value = true;
  uploadError.value = "";
  try {
    const preview = await api("PreviewRemoteUpload", props.connection, destination, uploadItems.value.map(item => item.local), uploadSudo.value);
    const files: FileMapping[] = [];
    let all: ConflictDecision["action"] | undefined;
    for (const item of preview) {
      if (disposed) return;
      if (item.remote) {
        const decision: ConflictDecision = all ? { action: all, all: true } : await new Promise(resolve => { conflict.value = item; resolveConflict = resolve; });
        if (decision.action === "cancel") return;
        if (decision.all) all = decision.action;
        if (decision.action === "skip") continue;
      }
      files.push({ ...item.file, executable: executable.value });
    }
    if (disposed) return;
    if (files.length)
    await api(
      "UploadRemoteSelection",
      props.connection,
      files,
      uploadSudo.value,
      uploadProtocol.value,
    );
    upload.value = false;
    uploadItems.value = [];
  } catch (e) {
    if (!disposed) uploadError.value = String(e);
  } finally {
    busy.value = false;
    if (!disposed) await list();
  }
}
onMounted(() => void list());
</script>
<template>
  <ResizablePanel
    class="remote-files"
    label="服务器文件管理"
    storage-key="thundership.remote-files-width"
    :default-size="600"
    :min-size="320"
    :max-size="1600"
    :max-ratio="0.8"
  >
    <header class="file-toolbar">
      <IconButton
        icon="mdi-chevron-left"
        label="返回上一目录"
        size="x-small"
        :disabled="busy || closed || !history.length"
        @click="back"
      />
      <IconButton
        icon="mdi-refresh"
        label="刷新"
        size="x-small"
        :disabled="busy || closed"
        @click="list()"
      />
      <form class="file-path" @submit.prevent="list(input)">
        <input
          v-model="input"
          aria-label="远端路径，回车打开"
          spellcheck="false"
          :disabled="closed"
        />
      </form>
      <IconButton icon="mdi-upload" label="上传文件" size="x-small" :disabled="busy || closed" @click="chooseUpload(false)" />
      <IconButton
        icon="mdi-magnify"
        label="搜索当前目录"
        size="x-small"
        @click="
          searchOpen = !searchOpen;
          if (!searchOpen) search = '';
        "
      />
      <v-menu
        ><template #activator="{ props: activator }"
          ><span v-bind="activator"
            ><IconButton
              icon="mdi-clock-outline"
              label="最近目录"
              size="x-small" /></span></template
        ><v-list density="compact"
          ><v-list-item
            v-for="path in favorites" :key="'favorite-' + path" :title="path" prepend-icon="mdi-star" @click="list(path)" />
          <v-divider v-if="favorites.length" /><v-list-item
            v-for="path in recent"
            :key="path"
            :title="path"
            @click="list(path)" /></v-list
      ></v-menu>
      <v-menu :close-on-content-click="false"
        ><template #activator="{ props: activator }"
          ><span v-bind="activator"
            ><IconButton
              icon="mdi-dots-vertical"
              label="文件操作与显示设置"
              size="x-small" /></span></template
        ><v-list density="compact"
          ><v-list-item
            title="上一级目录"
            prepend-icon="mdi-arrow-up"
            :disabled="busy || closed || directory === '/'"
            @click="parent" /><v-list-item
            title="上传文件…"
            prepend-icon="mdi-upload"
            :disabled="busy || closed"
            @click="chooseUpload(false)" /><v-list-item title="上传文件夹…" prepend-icon="mdi-folder-upload-outline" :disabled="busy || closed" @click="chooseUpload(true)" /><v-list-item title="粘贴 / 上传剪贴板文件 (Ctrl+V)" prepend-icon="mdi-content-paste" :disabled="busy || closed" @click="pasteFiles" /><v-list-item
            title="新建文件夹" prepend-icon="mdi-folder-plus-outline" :disabled="busy || closed" @click="beginOperation('mkdir', null)" /><v-list-item
            title="新建文件" prepend-icon="mdi-file-document-plus-outline" :disabled="busy || closed" @click="beginOperation('create', null)" /><v-list-item
            title="复制当前路径"
            prepend-icon="mdi-content-copy"
            @click="
              contextEntry = null;
              copyPath();
            " /><v-divider /><v-checkbox
            v-model="showHidden"
            label="显示隐藏文件"
            hide-details
            density="compact"
            class="px-3" /><v-checkbox
            v-model="sudo"
            label="sudo 文件访问"
            hide-details
            density="compact"
            class="px-3"
            :disabled="busy || closed"
            @update:model-value="list()" /></v-list
      ></v-menu>
    </header>
    <div v-if="searchOpen" class="file-search">
      <v-icon size="16">mdi-magnify</v-icon
      ><input
        v-model="search"
        placeholder="筛选当前目录"
        aria-label="筛选当前目录"
      /><IconButton
        icon="mdi-close"
        label="关闭搜索"
        size="x-small"
        @click="
          searchOpen = false;
          search = '';
        "
      />
    </div>
    <div class="file-summary">
      <span v-if="sudo" class="text-warning">sudo · </span>共
      {{ count.files }} 个文件，{{ count.directories }} 个文件夹，{{
        size(count.size)
      }}<span v-if="selectedNames.length"> · 已选 {{ selectedNames.length }} 项</span><span v-if="busy"> · 处理中</span>
    </div>
    <v-progress-linear v-if="busy" indeterminate color="primary" height="2" />
    <div
      class="remote-file-scroll" data-file-drop-target
      tabindex="0"
      aria-label="服务器文件列表"
      @keydown="fileKeys"
      @local-files-drop="localDrop"
      @paste.prevent.stop="pasteFiles"
      @click.self="($event.currentTarget as HTMLElement).focus()"
      @contextmenu.prevent="context($event, null)"
    >
      <table class="remote-file-table">
        <thead>
          <tr>
            <th v-for="column in columns" :key="column.key">
              <button @click="sortBy(column.key)">
                {{ column.title
                }}<v-icon size="12">{{
                  sort === column.key
                    ? descending
                      ? "mdi-chevron-down"
                      : "mdi-chevron-up"
                    : "mdi-unfold-more-horizontal"
                }}</v-icon>
              </button>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-if="directory !== '/'"
            class="file-row"
            tabindex="0"
            @dblclick="parent"
            @keydown.enter.prevent="parent"
          >
            <td>
              <span class="file-name"
                ><v-icon size="17" color="#e5bb5c">mdi-folder</v-icon>..</span
              >
            </td>
            <td>—</td>
            <td>文件夹</td>
            <td>—</td>
            <td>—</td>
            <td>—</td>
          </tr>
          <tr
            v-for="entry in visible"
            :key="entry.name"
            class="file-row"
            :class="{ selected: selectedNames.includes(entry.name), 'file-cut': remoteFileClipboard?.cut && remoteFileClipboard.server === clipboardServer && (remoteFileClipboard.items?.some(item => item.path === child(entry.name)) || remoteFileClipboard.path === child(entry.name)) }"
            tabindex="0"
            :aria-label="entry.name"
            :data-remote-directory="entry.directory ? child(entry.name) : undefined"
            @click="selectEntry($event, entry)"
            @focus="selected = entry.name"
            @dblclick="open(entry)"
            @keydown.enter.prevent="open(entry)"
            @contextmenu.stop.prevent="context($event, entry)"
          >
            <td>
              <span class="file-name" :title="entry.name"
                ><v-icon
                  size="17"
                  :color="
                    entry.directory
                      ? '#e5bb5c'
                      : entry.link
                        ? '#7cbfc7'
                        : '#a5c2dd'
                  "
                  >{{
                    entry.directory
                      ? "mdi-folder"
                      : entry.link
                        ? "mdi-file-link-outline"
                        : entry.name.endsWith(".html")
                          ? "mdi-language-html5"
                          : "mdi-file-outline"
                  }}</v-icon
                ><span>{{ entry.name }}</span></span
              >
            </td>
            <td>{{ modified(entry.modified) }}</td>
            <td>{{ type(entry) }}</td>
            <td>{{ entry.directory ? "—" : size(entry.size) }}</td>
            <td>{{ entry.mode }}</td>
            <td :title="entry.owner + (entry.ownerId ? ' · UID/GID：' + entry.ownerId : '')">{{ entry.owner || "—" }}</td>
          </tr>
        </tbody>
      </table>
      <div v-if="!visible.length && !busy" class="file-empty">
        {{ search ? "没有匹配的文件" : "目录为空" }}
      </div>
    </div>
  <v-menu v-model="menu" :target="[menuX, menuY]" location="bottom start" :close-on-content-click="false" max-height="calc(100vh - 28px)">
    <RemoteFileMenu :entry="contextEntry" :multiple="selectedNames.length > 1" :favorite="favorites.includes(directory)" :can-paste="canPaste || localPaste" :disabled="busy || closed" @action="menuAction" />
  </v-menu>
  <UploadConflictDialog :file="conflict" @decide="decideConflict" />
  <v-dialog v-model="properties" max-width="540"
    ><v-card title="文件属性"
      ><v-card-text v-if="contextEntry"
        ><dl class="file-properties">
          <dt>名称</dt>
          <dd>{{ contextEntry.name }}</dd>
          <dt>路径</dt>
          <dd>{{ child(contextEntry.name) }}</dd>
          <dt>类型</dt>
          <dd>{{ type(contextEntry) }}</dd>
          <dt>大小</dt>
          <dd>{{ contextEntry.directory ? "—" : size(contextEntry.size) }}</dd>
          <dt>修改时间</dt>
          <dd>{{ modified(contextEntry.modified) }}</dd>
          <dt>权限</dt>
          <dd>{{ contextEntry.mode }}</dd>
          <dt>用户/组</dt>
          <dd>{{ contextEntry.owner || "—" }}</dd>
          <dt>UID/GID</dt>
          <dd>{{ contextEntry.ownerId || "—" }}</dd>
        </dl></v-card-text
      ><v-card-actions
        ><v-spacer /><v-btn @click="properties = false"
          >关闭</v-btn
        ></v-card-actions
      ></v-card
    ></v-dialog
  >
  <v-dialog v-model="upload" max-width="540" :persistent="busy"
    ><v-card :title="uploadProtocol === 'scp' ? 'SCP 上传到当前目录' : '上传到当前目录'"
      ><v-card-text
        ><p class="code operation-path">{{ uploadDirectory }}</p>
        <ul class="upload-selection"><li v-for="item in uploadItems" :key="item.local" :title="item.local"><v-icon size="16">{{ item.kind === 'folder' ? 'mdi-folder-outline' : 'mdi-file-outline' }}</v-icon> {{ item.name }}</li></ul>
        <v-checkbox
          label="可执行文件"
          v-model="executable"
          :disabled="busy"
        /><v-checkbox label="sudo 上传" v-model="uploadSudo" :disabled="busy" />
        <p class="muted">
          同名文件上传前询问是否覆盖，通过临时文件和原子替换更新。sudo
          上传需要服务器允许非交互提权。
        </p><p v-if="uploadError" role="alert" class="text-error mt-3">{{ uploadError }}</p><p v-if="busy" class="muted mt-3">正在上传，完成后将自动刷新文件列表…</p></v-card-text
      ><v-card-actions
        ><v-btn :disabled="busy" @click="chooseUpload(false)">添加文件</v-btn><v-btn :disabled="busy" @click="chooseUpload(true)">选择文件夹</v-btn><v-spacer /><v-btn :disabled="busy" @click="upload = false">取消</v-btn
        ><v-btn color="primary" :loading="busy" :disabled="closed || !uploadItems.length" @click="send">确认上传</v-btn></v-card-actions
      ></v-card
    ></v-dialog
  >
  <v-dialog :model-value="!!operation" max-width="500" :persistent="busy"
    @update:model-value="!$event && !busy && (operation = null)">
    <v-card v-if="operation" :title="operationTitles[operation.action]">
      <v-card-text>
        <p class="operation-path">{{ ['copy', 'move', 'rename', 'delete', 'chmod', 'compress', 'extract'].includes(operation.action) ? operation.source : operation.directory }}</p>
        <ul v-if="operation.items.length > 1" class="upload-selection"><li v-for="item in operation.items" :key="item.source">{{ item.name }}</li></ul>
        <p v-if="operation.action === 'delete'" class="my-3">{{ operation.items.length > 1 ? '将永久删除所选项目，包括文件夹中的全部内容。' : operation.isDirectory ? '将永久删除此文件夹及其全部内容。' : '将永久删除此文件。' }}</p>
        <template v-else>
          <p v-if="operation.action === 'extract'" class="muted mb-3">解压到新文件夹，不覆盖已有目录。支持 ZIP、TAR、TAR.GZ、TGZ，需要服务器提供 Python 3。</p>
          <p v-if="operation.action === 'copy' || operation.action === 'move'" class="muted mb-3">目标目录：{{ operation.directory }}</p>
          <v-text-field v-if="operation.items.length <= 1" v-model="operationValue" :label="operation.action === 'chmod' ? '八进制权限' : '名称'" :placeholder="operation.action === 'chmod' ? '例如 644、755' : ''" autofocus :disabled="busy" spellcheck="false" :error-messages="operationError" @keydown.enter.prevent="applyOperation" />
          <div v-if="operation.action === 'chmod'" class="permissions-grid">
            <span /><span>读取</span><span>写入</span><span>执行</span>
            <template v-for="(label, row) in ['用户', '用户组', '其他']" :key="label">
              <span>{{ label }}</span><v-checkbox v-for="(permission, col) in ['读取', '写入', '执行']" :key="permission"
                :model-value="!!(permissionValue & (1 << (8 - row * 3 - col)))" :aria-label="label + permission" hide-details density="compact" :disabled="busy"
                @update:model-value="setPermission(1 << (8 - row * 3 - col), !!$event)" />
            </template>
          </div>
        </template>
        <p v-if="(operation.action === 'delete' || operation.items.length > 1) && operationError" class="text-error mt-3" role="alert">{{ operationError }}</p>
        <p v-if="operation.sudo" class="text-warning mt-2">使用 sudo 文件访问</p>
      </v-card-text>
      <v-card-actions><v-spacer /><v-btn :disabled="busy" @click="operation = null">取消</v-btn><v-btn :color="operation.action === 'delete' ? 'error' : 'primary'" :loading="busy" :disabled="closed" @click="applyOperation">{{ operation.action === 'delete' ? '确认删除' : '确定' }}</v-btn></v-card-actions>
    </v-card>
  </v-dialog>
  <CodeEditorDialog
    v-if="editor"
    v-model:open="editorOpen"
    :model-value="editor.content"
    :title="'编辑文件 · ' + editor.path.split('/').at(-1)"
    :hint="editor.path + (editor.sudo ? ' · sudo' : '') + ' · UTF-8'"
    save-label="保存到服务器"
    :saving="saving"
    :saved="saved"
    :error="editorError"
    @save="saveText"
  />
  </ResizablePanel>
</template>
<style scoped>
.remote-files {
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--border);
  background: rgb(var(--v-theme-surface));
  font:
    12px/1.4 Consolas,
    "Microsoft YaHei",
    monospace;
}
.file-toolbar {
  height: 38px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 3px 6px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.file-path {
  flex: 1;
  min-width: 0;
}
.file-path input {
  width: 100%;
  height: 26px;
  padding: 3px 9px;
  outline: none;
  border: 1px solid var(--border);
  border-radius: 5px;
  background: rgba(var(--v-theme-on-surface), 0.025);
  color: inherit;
  font: inherit;
}
.file-path input:focus {
  border-color: rgb(var(--v-theme-primary));
}
.file-summary {
  height: 28px;
  display: flex;
  gap: 4px;
  align-items: center;
  justify-content: center;
  border-bottom: 1px solid var(--border);
  font-size: 11px;
  flex-shrink: 0;
}
.remote-file-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  user-select: none;
}
.remote-file-scroll.file-drop-target-active { outline: 2px dashed rgb(var(--v-theme-primary)); outline-offset: -3px; background: rgba(var(--v-theme-primary), .06); }
.upload-selection { max-height: 200px; overflow: auto; list-style: none; padding: 12px 0; }
.upload-selection li { padding: 4px 0; overflow-wrap: anywhere; }
.remote-file-table {
  border-collapse: collapse;
  width: 100%;
  table-layout: fixed;
  min-width: 700px;
  font-size: 11px;
}
.remote-file-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: rgb(var(--v-theme-surface));
  border-bottom: 1px solid var(--border);
  font-weight: 400;
  text-align: left;
  color: var(--muted);
  height: 26px;
}
.remote-file-table th button {
  width: 100%;
  display: flex;
  justify-content: space-between;
  align-items: center;
  text-align: left;
  gap: 5px;
}
.remote-file-table th,
.remote-file-table td {
  padding: 3px 9px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  height: 27px;
}
.remote-file-table th:nth-child(1) {
  width: 26%;
}
.remote-file-table th:nth-child(2) {
  width: 21%;
}
.remote-file-table th:nth-child(3) {
  width: 10%;
}
.remote-file-table th:nth-child(4) {
  width: 10%;
}
.remote-file-table th:nth-child(5) {
  width: 15%;
}
.remote-file-table th:nth-child(6) {
  width: 18%;
}
.file-row:nth-child(even) {
  background: rgba(var(--v-theme-on-surface), 0.025);
}
.file-row {
  cursor: default;
  outline: none;
}
.file-row:hover {
  background: rgba(var(--v-theme-on-surface), 0.055);
}
.file-row.selected,
.file-row:focus-visible {
  background: rgba(var(--v-theme-primary), 0.13);
}
.file-name {
  display: flex;
  align-items: center;
  gap: 7px;
  overflow: hidden;
  min-width: 0;
}
.file-name > span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-name .v-icon {
  flex-shrink: 0;
}
.file-search {
  display: flex;
  align-items: center;
  padding: 3px 9px;
  gap: 8px;
  border-bottom: 1px solid var(--border);
}
.file-search input {
  min-width: 0;
  flex: 1;
  outline: none;
}
.file-empty {
  padding: 24px;
  color: var(--muted);
  text-align: center;
  font-size: 12px;
}
.file-properties {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr);
  gap: 10px;
}
.file-properties dt {
  color: var(--muted);
}
.file-properties dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.file-row.file-cut { opacity: 0.5; }
.context-actions { display: flex; align-items: center; gap: 2px; padding: 2px 10px 6px; font-size: 12px; color: var(--muted); }
.operation-path { font-family: var(--terminal-font-family); font-size: 12px; overflow-wrap: anywhere; margin-bottom: 16px; }
.permissions-grid { display: grid; grid-template-columns: 70px repeat(3, 1fr); align-items: center; text-align: center; gap: 2px 8px; font-size: 12px; }
.permissions-grid :deep(.v-selection-control) { justify-content: center; }
</style>
