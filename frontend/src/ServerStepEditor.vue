<script setup lang="ts">
import type { Step, Server } from "./types";
defineProps<{ step: Step; servers: Server[] }>();
defineEmits<{ manage: [] }>();
</script>
<template>
  <div v-if="step.targets" class="pa-4 d-flex flex-column ga-4">
    <v-text-field label="节点名称" v-model="step.name" />
    <v-select
      label="目标服务器"
      v-model="step.targets.servers"
      :items="servers"
      item-title="name"
      item-value="id"
      multiple
      chips
      closable-chips
    />
    <v-btn
      v-if="!servers.length"
      size="small"
      prepend-icon="mdi-server-plus"
      @click="$emit('manage')"
      >添加服务器</v-btn
    >
    <v-text-field
      label="服务器最大并发（1 为串行）"
      type="number"
      min="1"
      max="64"
      v-model.number="step.targets.concurrency"
    />
    <small class="text-medium-emphasis"
      >后续步骤在这些服务器上执行；连接新的服务器节点可切换目标。路径或命令不同的服务器请分支配置。</small
    >
    <details class="advanced">
      <summary>资源锁</summary>
      <v-combobox
        label="共享资源锁"
        v-model="step.targets.resources"
        multiple
        chips
        closable-chips
        :items="[]"
        hint="同一主机的同名资源互斥，例如 systemd:service-a"
        persistent-hint
      />
    </details>
  </div>
</template>
