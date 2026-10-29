import { computed, ref } from "vue";
import type { Server } from "../types";
import { clone, uid } from "../api";

export function useConnectionTabs() {
  const connections = ref<{ id: string; server: Server; number: number }[]>([]);
  const activeConnection = ref("");
  const connectedServer = computed(
    () =>
      connections.value.find((t) => t.id === activeConnection.value)?.server ||
      null,
  );
  function openConnection(server: Server) {
    const number =
      Math.max(
        0,
        ...connections.value
          .filter((t) => t.server.id === server.id)
          .map((t) => t.number),
      ) + 1;
    const tab = { id: uid(), server: clone(server), number };
    connections.value.push(tab);
    activeConnection.value = tab.id;
  }
  function closeConnectionTab(id: string) {
    const index = connections.value.findIndex((t) => t.id === id);
    if (index < 0) return;
    connections.value.splice(index, 1);
    if (activeConnection.value === id)
      activeConnection.value =
        connections.value[Math.min(index, connections.value.length - 1)]?.id ||
        "";
  }
  return {
    connections,
    activeConnection,
    connectedServer,
    openConnection,
    closeConnectionTab,
  };
}
