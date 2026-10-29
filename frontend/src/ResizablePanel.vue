<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
const props = withDefaults(
  defineProps<{
    label: string;
    storageKey: string;
    defaultSize: number;
    minSize?: number;
    maxSize?: number;
    maxRatio?: number;
    axis?: "width" | "height";
  }>(),
  { minSize: 280, maxSize: 960, maxRatio: 0.75, axis: "width" },
);
const panel = ref<HTMLElement | null>(null);
const available = ref(Infinity);
const preferred = ref(props.defaultSize);
const dimension = computed(() => props.axis === "height" ? "高度" : "宽度");
const maximum = computed(() =>
  Math.max(0, Math.min(props.maxSize, available.value * props.maxRatio)),
);
const minimum = computed(() => Math.min(props.minSize, maximum.value));
const size = computed(() =>
  Math.min(maximum.value, Math.max(minimum.value, preferred.value)),
);
let observer: ResizeObserver | undefined;
let drag: { id: number; position: number; size: number } | null = null;
const position = (event: PointerEvent) => props.axis === "height" ? event.clientY : event.clientX;
function save() {
  try {
    localStorage.setItem(props.storageKey, String(preferred.value));
  } catch {
    /* Layout preferences are optional. */
  }
}
function setSize(value: number) {
  preferred.value = Math.round(
    Math.min(maximum.value, Math.max(minimum.value, value)),
  );
}
function start(event: PointerEvent) {
  if (event.button !== 0 || drag) return;
  event.preventDefault();
  const handle = event.currentTarget as HTMLElement;
  handle.focus();
  handle.setPointerCapture(event.pointerId);
  drag = { id: event.pointerId, position: position(event), size: size.value };
}
function move(event: PointerEvent) {
  if (drag?.id === event.pointerId)
    setSize(drag.size + drag.position - position(event));
}
function finish(event: PointerEvent) {
  if (drag?.id !== event.pointerId) return;
  drag = null;
  const handle = event.currentTarget as HTMLElement;
  if (handle.hasPointerCapture(event.pointerId))
    handle.releasePointerCapture(event.pointerId);
  save();
}
function reset() {
  preferred.value = props.defaultSize;
  save();
}
function keyResize(event: KeyboardEvent) {
  const decrease = props.axis === "height" ? "ArrowDown" : "ArrowRight";
  const increase = props.axis === "height" ? "ArrowUp" : "ArrowLeft";
  if (![increase, decrease, "Home", "End"].includes(event.key)) return;
  event.preventDefault();
  setSize(
    event.key === "Home"
      ? minimum.value
      : event.key === "End"
        ? maximum.value
        : size.value + (event.key === increase ? 20 : -20),
  );
  save();
}
onMounted(() => {
  try {
    const saved = Number(localStorage.getItem(props.storageKey));
    if (Number.isFinite(saved) && saved > 0) preferred.value = saved;
  } catch {
    /* Use the default width when storage is unavailable. */
  }
  const parent = panel.value?.parentElement;
  if (parent) {
    const measure = () => {
      available.value = parent.getBoundingClientRect()[props.axis];
    };
    measure();
    observer = new ResizeObserver(measure);
    observer.observe(parent);
  }
});
onBeforeUnmount(() => observer?.disconnect());
</script>
<template>
  <aside ref="panel" class="resizable-panel" :class="{ 'height-panel': axis === 'height' }" :style="{ [axis]: size + 'px' }">
    <div
      class="panel-resize-handle"
      role="separator"
      tabindex="0"
      :aria-orientation="axis === 'height' ? 'horizontal' : 'vertical'"
      :aria-label="label + dimension"
      :aria-valuenow="Math.round(size)"
      :aria-valuemin="Math.round(minimum)"
      :aria-valuemax="Math.round(maximum)"
      :title="'拖动调整' + label + dimension + '；双击恢复默认，方向键微调'"
      @pointerdown="start"
      @pointermove="move"
      @pointerup="finish"
      @pointercancel="finish"
      @lostpointercapture="finish"
      @keydown="keyResize"
      @dblclick="reset"
    />
    <div class="resizable-panel-content"><slot /></div>
  </aside>
</template>
<style scoped>
.resizable-panel {
  position: relative;
  flex-shrink: 0;
  min-width: 0;
  overflow: hidden;
}
.resizable-panel-content {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
}
.panel-resize-handle {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 7px;
  z-index: 5;
  cursor: col-resize;
  touch-action: none;
  user-select: none;
}
.panel-resize-handle::after {
  content: "";
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 2px;
  background: var(--border);
  transition: background 0.15s;
}
.panel-resize-handle:hover::after,
.panel-resize-handle:focus-visible::after {
  width: 3px;
  background: rgb(var(--v-theme-primary));
}
.panel-resize-handle:focus-visible {
  outline: 1px solid rgb(var(--v-theme-primary));
  outline-offset: -1px;
}
.height-panel { padding-top: 7px; min-height: 0; }
.height-panel > .panel-resize-handle { bottom: auto; right: 0; width: auto; height: 7px; cursor: row-resize; }
.height-panel > .panel-resize-handle::after { right: 0; bottom: auto; width: auto; height: 2px; }
.height-panel > .panel-resize-handle:hover::after,
.height-panel > .panel-resize-handle:focus-visible::after { width: auto; height: 3px; }
</style>
