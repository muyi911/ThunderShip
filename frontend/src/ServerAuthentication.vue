<script setup lang="ts">
import { ref, watch } from "vue";
import type { Server } from "./types";
import PrivateKeyPicker from "./PrivateKeyPicker.vue";
import IconButton from "./IconButton.vue";

const props = defineProps<{
  server: Server;
  servers: Server[];
  secret: string;
  hasSavedSecret: boolean;
  disabled?: boolean;
}>();
defineEmits<{ "update:secret": [value: string]; error: [error: unknown] }>();
const visible = ref(false);
watch(
  () => props.server.auth,
  () => {
    visible.value = false;
  },
);
</script>
<template>
  <div class="authentication-grid">
    <div class="authentication-methods">
      <label id="authentication-label">认证方式</label>
      <v-btn-toggle
        v-model="server.auth"
        mandatory
        divided
        rounded="pill"
        density="compact"
        variant="tonal"
        color="primary"
        :disabled="disabled"
        aria-labelledby="authentication-label"
      >
        <v-btn value="password" prepend-icon="mdi-lock-outline">密码</v-btn>
        <v-btn value="key" prepend-icon="mdi-key-outline">私钥</v-btn>
      </v-btn-toggle>
    </div>
    <div class="authentication-fields">
      <div v-if="server.auth === 'key'" class="connection-field">
        <label>私钥文件</label>
        <PrivateKeyPicker
          v-model="server.keyPath"
          :servers="servers"
          :disabled="disabled"
          @error="$emit('error', $event)"
        />
      </div>
      <div class="connection-field">
        <label for="connection-secret">{{
          server.auth === "key" ? "私钥口令" : "密码"
        }}</label>
        <v-text-field
          id="connection-secret"
          :model-value="secret"
          @update:model-value="$emit('update:secret', $event)"
          :type="visible ? 'text' : 'password'"
          :disabled="disabled"
          autocomplete="new-password"
          variant="solo-filled"
          flat
          :placeholder="
            hasSavedSecret
              ? '已保存，留空保持原值'
              : server.auth === 'key'
                ? '未加密私钥可留空'
                : '输入登录密码'
          "
        >
          <template #append-inner
            ><IconButton
              :icon="visible ? 'mdi-eye-off-outline' : 'mdi-eye-outline'"
              :label="visible ? '隐藏密码或口令' : '显示密码或口令'"
              :disabled="disabled"
              @click="visible = !visible"
          /></template>
        </v-text-field>
      </div>
    </div>
  </div>
</template>
<style scoped>
.authentication-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
  align-items: center;
  min-height: 140px;
}
.authentication-methods {
  display: grid;
  justify-items: start;
  gap: 12px;
}
.authentication-fields {
  display: grid;
  gap: 16px;
}
.connection-field {
  display: grid;
  gap: 7px;
}
label {
  font-size: 13px;
  color: rgb(var(--v-theme-on-surface));
  opacity: 0.8;
}
@media (max-width: 600px) {
  .authentication-grid {
    grid-template-columns: 1fr;
  }
}
</style>
