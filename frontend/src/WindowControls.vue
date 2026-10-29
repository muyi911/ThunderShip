<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { Application, Window } from "@wailsio/runtime";

import { isDesktop } from "./api";

const maximised = ref(false);
async function syncWindowState() {
  if (isDesktop()) maximised.value = await Window.IsMaximised();
}
onMounted(() => {
  void syncWindowState();
  window.addEventListener("resize", syncWindowState);
});
onUnmounted(() => window.removeEventListener("resize", syncWindowState));
</script>

<template>
  <div class="window-controls" aria-label="窗口操作">
    <button
      type="button"
      title="最小化"
      aria-label="最小化"
      @click="Window.Minimise()"
    >
      <v-icon size="17">mdi-minus</v-icon>
    </button>
    <button
      type="button"
      :title="maximised ? '还原窗口' : '最大化'"
      :aria-label="maximised ? '还原窗口' : '最大化'"
      @click="Window.ToggleMaximise()"
    >
      <v-icon size="15">{{
        maximised ? "mdi-window-restore" : "mdi-window-maximize"
      }}</v-icon>
    </button>
    <button
      type="button"
      class="window-close"
      title="关闭"
      aria-label="关闭"
      @click="Application.Quit()"
    >
      <v-icon size="18">mdi-close</v-icon>
    </button>
  </div>
</template>

<style scoped>
.window-controls {
  display: flex;
  height: 100%;
  margin-left: 4px;
  flex-shrink: 0;
}
.window-controls button {
  width: 44px;
  display: grid;
  place-items: center;
  color: inherit;
}
.window-controls button:hover {
  background: var(--hover);
}
.window-controls .window-close:hover {
  background: #c42b1c;
  color: white;
}
.window-controls button:focus-visible {
  outline-offset: -3px;
}
</style>
