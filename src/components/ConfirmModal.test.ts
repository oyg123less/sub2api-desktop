import { mount } from "@vue/test-utils";
import { createI18n } from "vue-i18n";
import { nextTick } from "vue";
import { afterEach, describe, expect, it } from "vitest";
import ConfirmModal from "./ConfirmModal.vue";

const mounted: ReturnType<typeof mount>[] = [];

function render(open = false) {
  const i18n = createI18n({ legacy: false, locale: "en", messages: { en: { common: { cancel: "Cancel", confirm: "Confirm" } } } });
  const wrapper = mount(ConfirmModal, {
    attachTo: document.body,
    props: { open, title: "Delete logs" },
    global: { plugins: [i18n] },
  });
  mounted.push(wrapper);
  return wrapper;
}

afterEach(() => {
  for (const wrapper of mounted.splice(0)) wrapper.unmount();
  document.body.innerHTML = "";
});

describe("ConfirmModal keyboard behavior", () => {
  it("cancels on Escape and restores focus after closing", async () => {
    const trigger = document.createElement("button");
    document.body.appendChild(trigger);
    trigger.focus();
    const wrapper = render(false);
    await wrapper.setProps({ open: true });
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(wrapper.emitted("cancel")).toHaveLength(1);
    await wrapper.setProps({ open: false });
    await nextTick();
    expect(document.activeElement).toBe(trigger);
  });

  it("wraps focus within the dialog", async () => {
    const wrapper = render(true);
    await nextTick();
    const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>("[role='dialog'] button"));
    expect(buttons).toHaveLength(2);
    buttons[1].focus();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true }));
    expect(document.activeElement).toBe(buttons[0]);
  });
});
