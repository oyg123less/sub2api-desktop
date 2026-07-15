<script setup lang="ts">
import { useAppStore } from "../store";
import Icon from "./Icon.vue";
const app = useAppStore();

function toastIcon(type: "info" | "success" | "error" | "warn") {
  if (type === "success") return "check";
  if (type === "info") return "info";
  return "warn";
}
</script>

<template>
  <Teleport to="body">
    <div class="toast-wrap">
      <div v-for="t in app.toasts" :key="t.id" class="toast" :class="t.type" role="status">
        <Icon :name="toastIcon(t.type)" :size="16" />
        <span>{{ t.message }}</span>
        <i class="toast-progress" aria-hidden="true"></i>
      </div>
    </div>
  </Teleport>
</template>
