<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";

const props = defineProps<{ siteKey: string }>();
const emit = defineEmits<{
  token: [value: string];
  expired: [];
  error: [];
}>();

interface TurnstileAPI {
  render(element: HTMLElement, options: Record<string, unknown>): string;
  remove(widgetID: string): void;
  reset(widgetID: string): void;
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI;
  }
}

const container = ref<HTMLElement | null>(null);
let widgetID = "";

function loadTurnstile(): Promise<TurnstileAPI> {
  if (window.turnstile) return Promise.resolve(window.turnstile);
  return new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>('script[data-amber-turnstile="1"]');
    const script = existing || document.createElement("script");
    const finish = () => window.turnstile ? resolve(window.turnstile) : reject(new Error("Turnstile unavailable"));
    script.addEventListener("load", finish, { once: true });
    script.addEventListener("error", () => reject(new Error("Turnstile failed to load")), { once: true });
    if (!existing) {
      script.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
      script.async = true;
      script.defer = true;
      script.dataset.amberTurnstile = "1";
      document.head.appendChild(script);
    }
  });
}

async function renderWidget() {
  if (!container.value || !props.siteKey) return;
  try {
    const turnstile = await loadTurnstile();
    if (widgetID) turnstile.remove(widgetID);
    widgetID = turnstile.render(container.value, {
      sitekey: props.siteKey,
      theme: "auto",
      callback: (token: string) => emit("token", token),
      "expired-callback": () => emit("expired"),
      "error-callback": () => emit("error"),
    });
  } catch {
    emit("error");
  }
}

defineExpose({
  reset: () => {
    if (widgetID && window.turnstile) window.turnstile.reset(widgetID);
  },
});

onMounted(renderWidget);
watch(() => props.siteKey, renderWidget);
onBeforeUnmount(() => {
  if (widgetID && window.turnstile) window.turnstile.remove(widgetID);
});
</script>

<template>
  <div ref="container" class="turnstile-widget"></div>
</template>

<style scoped>
.turnstile-widget {
  min-height: 65px;
}
</style>
