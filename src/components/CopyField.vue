<script setup lang="ts">
import { ref } from "vue";
import Icon from "./Icon.vue";
import { useAppStore } from "../store";
import { useI18n } from "vue-i18n";

const props = defineProps<{ value: string; mask?: boolean }>();
const app = useAppStore();
const { t } = useI18n();
const revealed = ref(false);

function display() {
  if (props.mask && !revealed.value) {
    const v = props.value || "";
    if (v.length <= 12) return "••••••••";
    return v.slice(0, 8) + "••••••••" + v.slice(-4);
  }
  return props.value;
}

async function copy() {
  try {
    await navigator.clipboard.writeText(props.value);
  } catch {
    const ta = document.createElement("textarea");
    ta.value = props.value;
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    document.body.removeChild(ta);
  }
  app.toast(t("common.copied"), "success");
}
</script>

<template>
  <div class="code-box">
    <span :style="mask ? 'cursor:pointer' : ''" @click="mask && (revealed = !revealed)">
      {{ display() }}
    </span>
    <button class="copy-btn" @click="copy" :title="t('common.copy')">
      <Icon name="copy" :size="15" />
    </button>
  </div>
</template>
