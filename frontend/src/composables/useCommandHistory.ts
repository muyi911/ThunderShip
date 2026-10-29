import { ref, type Ref } from "vue";

const histories = new Map<string, Ref<string[]>>();
export function useCommandHistory(server: string) {
  const key = "thundership.command-history." + server;
  let commands = histories.get(key);
  if (!commands) {
    commands = ref<string[]>([]);
    try {
      const saved: unknown = JSON.parse(localStorage.getItem(key) || "[]");
      if (Array.isArray(saved))
        commands.value = saved.filter((item): item is string => typeof item === "string" && !!item.trim()).slice(0, 500);
    } catch { /* History remains available in this session if storage is unavailable. */ }
    histories.set(key, commands);
  }
  const entries = commands;
  function save() {
    try { localStorage.setItem(key, JSON.stringify(entries.value)); }
    catch { /* Storage quotas must not prevent a terminal command from running. */ }
  }
  function remember(text: string) {
    text = text.replace(/[\r\n]+$/g, "");
    if (!text.trim()) return;
    entries.value = [text, ...entries.value.filter((item) => item !== text)].slice(0, 500);
    // Bound persisted history as well as its entry count; output stays in command blocks.
    let size = 0;
    entries.value = entries.value.filter((item) => (size += item.length) <= 262144);
    save();
  }
  function clear() { entries.value = []; save(); }
  return { commands: entries, remember, clear };
}
