<script setup lang="ts">
import { useAppStore } from "../store";
const app = useAppStore();
</script>

<template>
  <Teleport to="body">
    <div class="toast-wrap">
      <div v-for="t in app.toasts" :key="t.id" class="toast" :class="t.type">
        {{ t.message }}
      </div>
    </div>
  </Teleport>
</template>
