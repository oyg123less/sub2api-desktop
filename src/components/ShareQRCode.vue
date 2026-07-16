<script setup lang="ts">
import QRCode from "qrcode";
import { nextTick, onMounted, ref, watch } from "vue";

const props = defineProps<{ baseUrl: string; guestKey: string; shareCode: string }>();
const canvas = ref<HTMLCanvasElement | null>(null);

async function render() {
  await nextTick();
  if (!canvas.value) return;
  await QRCode.toCanvas(canvas.value, JSON.stringify({
    base_url: props.baseUrl,
    api_key: props.guestKey,
    share_code: props.shareCode,
  }), {
    width: 164,
    margin: 1,
    errorCorrectionLevel: "M",
    color: { dark: "#1b1b1aff", light: "#ffffffff" },
  });
}

onMounted(render);
watch(() => [props.baseUrl, props.guestKey, props.shareCode], render);
</script>

<template><canvas ref="canvas" class="share-qr" width="164" height="164" data-test="share-qr"></canvas></template>

<style scoped>
.share-qr { width: 164px; height: 164px; padding: 5px; border: 1px solid var(--border-soft); border-radius: 8px; background: #fff; image-rendering: pixelated; }
</style>
