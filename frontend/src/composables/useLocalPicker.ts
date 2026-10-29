import { computed, ref } from "vue";
import { api } from "../api";
import type { FileMapping, LocalDirectory, LocalEntry } from "../types";

let lastDirectory = "";
const pathKey = (path: string) => {
  const normalized = path.replaceAll("\\", "/").replace(/\/$/, "");
  return /^[a-z]:|^\/\//i.test(normalized)
    ? normalized.toLowerCase()
    : normalized;
};

export function appendLocalFiles(
  existing: FileMapping[],
  added: FileMapping[],
) {
  const seen = new Set(existing.map((f) => pathKey(f.local)));
  for (const file of added) {
    const key = pathKey(file.local);
    if (!seen.has(key)) {
      existing.push(file);
      seen.add(key);
    }
  }
}

export function useLocalPicker(accept: (files: FileMapping[]) => void) {
  const opened = ref(false);
  const loading = ref(false);
  const confirming = ref(false);
  const error = ref("");
  const address = ref("");
  const directory = ref<LocalDirectory>({ path: "", parent: "", entries: [] });
  const selected = ref<LocalEntry[]>([]);
  let revision = 0;
  const selectable = computed(() =>
    directory.value.entries.filter((e) => e.selectable),
  );
  const has = (entry: LocalEntry) =>
    selected.value.some((s) => pathKey(s.path) === pathKey(entry.path));
  const all = computed(
    () => !!selectable.value.length && selectable.value.every(has),
  );
  function toggle(entry: LocalEntry) {
    if (!entry.selectable) return;
    selected.value = has(entry)
      ? selected.value.filter((s) => pathKey(s.path) !== pathKey(entry.path))
      : [...selected.value, entry];
  }
  function toggleAll() {
    const remove = all.value;
    for (const entry of selectable.value)
      if (has(entry) === remove) toggle(entry);
  }
  async function browse(path: string) {
    const request = ++revision;
    loading.value = true;
    error.value = "";
    try {
      const result = await api("ListLocalDirectory", path);
      if (request !== revision) return;
      directory.value = result;
      address.value = lastDirectory = result.path;
    } catch (e) {
      if (request === revision) error.value = String(e);
    } finally {
      if (request === revision) loading.value = false;
    }
  }
  function close() {
    opened.value = false;
    ++revision;
    loading.value = false;
  }
  async function open(initialPath: string) {
    selected.value = [];
    opened.value = true;
    await browse(lastDirectory || initialPath);
  }
  async function confirm() {
    confirming.value = true;
    error.value = "";
    try {
      const files = await api(
        "ResolveLocalPaths",
        selected.value.map((s) => s.path),
      );
      accept(files);
      close();
    } catch (e) {
      error.value = String(e);
    } finally {
      confirming.value = false;
    }
  }
  return {
    opened,
    loading,
    confirming,
    error,
    address,
    directory,
    selected,
    all,
    has,
    toggle,
    toggleAll,
    browse,
    close,
    open,
    confirm,
  };
}
