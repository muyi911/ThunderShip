<script
  setup
  lang="ts"
  generic="T extends { id: string; name: string; icon?: string }"
>
import { ref } from "vue";
const props = defineProps<{
  items: T[];
  icon: string;
  busy?: boolean;
  doubleClick?: boolean;
}>();
const emit = defineEmits<{
  open: [item: T];
  move: [id: string, beforeID: string];
}>();
const dragged = ref("");
const over = ref("");
const after = ref(false);
function reset() {
  dragged.value = over.value = "";
}
function leave(event: DragEvent, id: string) {
  if (
    over.value === id &&
    !(event.currentTarget as HTMLElement).contains(
      event.relatedTarget as Node | null,
    )
  )
    over.value = "";
}
function start(event: DragEvent, id: string) {
  if (props.busy || !event.dataTransfer) {
    event.preventDefault();
    return;
  }
  dragged.value = id;
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData("text/plain", id);
  const card = (event.target as HTMLElement).closest("article");
  if (card) event.dataTransfer.setDragImage(card, 28, 28);
}
function hover(event: DragEvent, id: string) {
  if (!dragged.value || dragged.value === id || props.busy) return;
  event.preventDefault();
  if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
  over.value = id;
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
  after.value = event.clientX >= rect.left + rect.width / 2;
}
function drop(event: DragEvent, id: string) {
  if (!dragged.value || dragged.value === id || props.busy) return;
  hover(event, id);
  const remaining = props.items.filter((item) => item.id !== dragged.value);
  const index = remaining.findIndex((item) => item.id === id);
  emit(
    "move",
    dragged.value,
    after.value ? remaining[index + 1]?.id || "" : id,
  );
  reset();
}
function keyMove(event: KeyboardEvent, id: string) {
  if (
    !event.altKey ||
    props.busy ||
    !["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)
  )
    return;
  event.preventDefault();
  const index = props.items.findIndex((item) => item.id === id);
  const previous = event.key === "ArrowLeft" || event.key === "ArrowUp";
  if (previous && index > 0) emit("move", id, props.items[index - 1]!.id);
  else if (!previous && index < props.items.length - 1)
    emit("move", id, props.items[index + 2]?.id || "");
}
</script>
<template>
  <div class="resource-grid" role="list" aria-label="资源卡片">
    <article
      v-for="item in items"
      :key="item.id"
      role="listitem"
      class="resource-card"
      :data-resource-id="item.id"
      :class="{
        dragging: dragged === item.id,
        'drop-before': over === item.id && !after,
        'drop-after': over === item.id && after,
      }"
      @dragover="hover($event, item.id)"
      @dragleave="leave($event, item.id)"
      @drop="drop($event, item.id)"
      @dblclick="
        doubleClick &&
        !($event.target as HTMLElement).closest(
          'footer, .resource-drag-handle',
        ) &&
        emit('open', item)
      "
    >
      <header class="resource-card-heading">
        <slot name="selection" :item="item" />
        <div class="resource-icon">
          <v-icon size="23">{{ item.icon || icon }}</v-icon>
        </div>
        <button
          class="resource-card-name"
          :title="item.name"
          @click="!doubleClick && emit('open', item)"
          @keydown.enter.prevent="doubleClick && emit('open', item)"
        >
          {{ item.name }}
        </button>
        <button
          class="resource-drag-handle"
          :disabled="busy"
          :draggable="!busy"
          :aria-label="'调整' + item.name + '的顺序'"
          title="拖拽排序；Alt + 方向键调整"
          @dragstart="start($event, item.id)"
          @dragend="reset"
          @keydown="keyMove($event, item.id)"
        >
          <v-icon size="19">mdi-drag</v-icon>
        </button>
      </header>
      <div class="resource-card-content"><slot :item="item" /></div>
      <footer class="resource-card-footer">
        <slot name="footer" :item="item" />
      </footer>
    </article>
  </div>
</template>
<style scoped>
.resource-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr));
  gap: 16px;
}
.resource-card {
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: rgb(var(--v-theme-surface));
  border: 1px solid var(--border);
  border-radius: 8px;
  transition:
    border-color 0.15s,
    opacity 0.15s;
  position: relative;
}
.resource-card:hover {
  border-color: rgb(var(--v-theme-primary), 0.45);
}
.resource-card.dragging {
  opacity: 0.4;
}
.resource-card.drop-before::before,
.resource-card.drop-after::after {
  content: "";
  position: absolute;
  top: 0;
  bottom: 0;
  width: 3px;
  border-radius: 3px;
  background: rgb(var(--v-theme-primary));
}
.resource-card.drop-before::before {
  left: -9px;
}
.resource-card.drop-after::after {
  right: -9px;
}
.resource-card-heading {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 16px 10px;
}
.resource-card-name {
  min-width: 0;
  flex: 1;
  font-size: 14px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: left;
}
.resource-card-name:hover {
  color: rgb(var(--v-theme-primary));
}
.resource-drag-handle {
  display: grid;
  place-items: center;
  width: 24px;
  height: 28px;
  flex-shrink: 0;
  color: var(--muted);
  cursor: grab;
  border-radius: 4px;
}
.resource-drag-handle:hover {
  color: rgb(var(--v-theme-primary));
  background: var(--hover);
}
.resource-drag-handle:active {
  cursor: grabbing;
}
.resource-drag-handle:disabled {
  cursor: default;
  opacity: 0.4;
}
button:focus-visible {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: 3px;
}
.resource-card-content {
  flex: 1;
  padding: 0 16px 16px;
  min-height: 66px;
  font-size: 12px;
}
.resource-card-footer {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  padding: 7px 12px;
  border-top: 1px solid var(--border);
  min-height: 48px;
}
</style>
