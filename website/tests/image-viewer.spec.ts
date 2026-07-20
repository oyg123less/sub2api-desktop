import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ImageViewer from "../src/components/ImageViewer.vue";

describe("ImageViewer", () => {
  it("opens with focus, closes on Escape, and restores the trigger", async () => {
    const wrapper = mount(ImageViewer, {
      attachTo: document.body,
      props: {
        src: "/screenshots/dashboard.png",
        alt: "Amber 仪表盘",
        caption: "演示界面",
      },
    });

    const trigger = wrapper.get<HTMLButtonElement>(".image-button");
    trigger.element.focus();
    await trigger.trigger("click");

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const close = document.querySelector<HTMLButtonElement>(".lightbox-close");
    expect(dialog).not.toBeNull();
    expect(close).toBe(document.activeElement);
    expect(document.body.classList.contains("modal-open")).toBe(true);

    dialog?.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await wrapper.vm.$nextTick();

    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(trigger.element).toBe(document.activeElement);
    expect(document.body.classList.contains("modal-open")).toBe(false);
    wrapper.unmount();
  });
});
