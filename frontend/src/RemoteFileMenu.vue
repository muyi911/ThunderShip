<script setup lang="ts">
import type { RemoteEntry } from "./types";
import IconButton from "./IconButton.vue";
defineProps<{ entry: RemoteEntry | null; multiple: boolean; favorite: boolean; canPaste: boolean; disabled: boolean }>();
const emit = defineEmits<{ action: [name: string] }>();
</script>
<template>
  <v-list density="compact" min-width="220" class="hex-file-menu">
    <div class="menu-actions"><span>操作</span><span class="spacer" />
      <IconButton icon="mdi-crosshairs-gps" label="终端切到当前目录" :disabled="disabled" size="x-small" @click="emit('action', 'terminal')" />
      <IconButton icon="mdi-content-paste" label="粘贴 (Ctrl+V)" :disabled="disabled || !canPaste" size="x-small" @click="emit('action', 'paste')" />
      <IconButton icon="mdi-content-cut" label="剪切 (Ctrl+X)" :disabled="disabled || !entry" size="x-small" @click="emit('action', 'cut')" />
      <IconButton icon="mdi-content-copy" label="复制 (Ctrl+C)" :disabled="disabled || !entry" size="x-small" @click="emit('action', 'copy')" />
    </div>
    <v-divider />
    <v-list-item title="打开" prepend-icon="mdi-file-document-outline" :disabled="disabled || !entry || multiple" @click="emit('action', 'open')"><template #append><kbd>Enter</kbd></template></v-list-item>
    <v-list-item title="编辑" prepend-icon="mdi-file-edit-outline" :disabled="disabled || !entry || entry.directory || entry.link || multiple" @click="emit('action', 'edit')"><template #append><kbd>Ctrl+Alt+E</kbd></template></v-list-item>
    <v-list-item title="刷新" prepend-icon="mdi-refresh" :disabled="disabled" @click="emit('action', 'refresh')"><template #append><kbd>F5</kbd></template></v-list-item>
    <v-divider />
    <v-list-item :title="favorite ? '取消收藏路径' : '收藏路径'" prepend-icon="mdi-folder-star-outline" :disabled="disabled" @click="emit('action', 'favorite')" />
    <v-list-item title="下载到…" prepend-icon="mdi-monitor-arrow-down-variant" :disabled="disabled || !entry || entry.link || multiple" @click="emit('action', 'download')" />
    <v-list-item title="上传文件…" prepend-icon="mdi-upload" :disabled="disabled" @click="emit('action', 'upload')" />
    <v-list-item title="上传文件夹…" prepend-icon="mdi-folder-upload-outline" :disabled="disabled" @click="emit('action', 'upload-folder')" />
    <v-menu open-on-hover submenu location="end" :close-on-content-click="false"><template #activator="{ props }"><v-list-item v-bind="props" title="SCP" prepend-icon="mdi-folder-arrow-up-down-outline" append-icon="mdi-menu-right" :disabled="disabled" /></template>
      <v-list class="hex-file-menu" density="compact" min-width="180">
        <v-list-item title="SCP下载" prepend-icon="mdi-download" :disabled="!entry || entry.link || multiple" @click="emit('action', 'scp-download')" />
        <v-list-item title="SCP文件上传" prepend-icon="mdi-upload" @click="emit('action', 'scp-upload')" />
      </v-list>
    </v-menu>
    <v-list-item title="压缩" prepend-icon="mdi-folder-zip-outline" :disabled="disabled || !entry || entry.link || multiple" @click="emit('action', 'compress')" />
    <v-list-item title="解压" prepend-icon="mdi-folder-zip" :disabled="disabled || !entry || entry.directory || entry.link || multiple || !/\.(zip|tar|tar\.gz|tgz)$/i.test(entry.name)" @click="emit('action', 'extract')" />
    <v-divider />
    <v-list-item title="重命名" prepend-icon="mdi-file-document-edit-outline" :disabled="disabled || !entry || multiple" @click="emit('action', 'rename')"><template #append><kbd>F2</kbd></template></v-list-item>
    <v-list-item title="删除" prepend-icon="mdi-file-remove-outline" :disabled="disabled || !entry" @click="emit('action', 'delete')"><template #append><kbd>Backspace</kbd></template></v-list-item>
    <v-menu open-on-hover submenu location="end" :close-on-content-click="false"><template #activator="{ props }"><v-list-item v-bind="props" title="更多" prepend-icon="mdi-chevron-down" append-icon="mdi-menu-right" /></template>
      <v-list class="hex-file-menu" density="compact" min-width="220">
        <v-list-item title="复制路径" prepend-icon="mdi-content-copy" @click="emit('action', 'path')"><template #append><kbd>Ctrl+Alt+C</kbd></template></v-list-item>
        <v-list-item title="新建文件夹" prepend-icon="mdi-folder-plus-outline" :disabled="disabled" @click="emit('action', 'mkdir')" />
        <v-list-item title="新建文件" prepend-icon="mdi-file-document-plus-outline" :disabled="disabled" @click="emit('action', 'create')"><template #append><kbd>Ctrl+Alt+N</kbd></template></v-list-item>
        <v-list-item title="权限" prepend-icon="mdi-archive-lock-open-outline" :disabled="disabled || !entry || entry.link || multiple" @click="emit('action', 'chmod')"><template #append><kbd>Ctrl+Alt+M</kbd></template></v-list-item>
        <v-list-item title="属性" prepend-icon="mdi-information-outline" :disabled="!entry || multiple" @click="emit('action', 'properties')" />
      </v-list>
    </v-menu>
  </v-list>
</template>
<style>
.hex-file-menu.v-list { padding: 5px; border: 1px solid var(--border); border-radius: 5px; box-shadow: 0 3px 10px #0005; }
.hex-file-menu .v-list-item { min-height: 28px; padding: 3px 7px; border-radius: 3px; }
.hex-file-menu .v-list-item-title { font-size: 12px; line-height: 20px; }
.hex-file-menu .v-list-item__prepend .v-icon, .hex-file-menu .v-list-item__append .v-icon { font-size: 16px; }
.hex-file-menu .v-list-item__prepend > .v-list-item__spacer { width: 10px; }
.hex-file-menu .v-divider { margin: 5px 4px; }
.hex-file-menu kbd { font: 11px var(--font-mono, monospace); color: var(--muted); margin-left: 10px; }
.hex-file-menu .menu-actions { display: flex; align-items: center; gap: 4px; padding: 3px 5px 4px; font-size: 11px; }
.hex-file-menu .menu-actions .v-btn { width: 25px; height: 22px; border-radius: 3px; background: rgba(var(--v-theme-on-surface), .12); }
.hex-file-menu .menu-actions .v-icon { font-size: 15px; }
</style>
