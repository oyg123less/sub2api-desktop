import { mount } from "@vue/test-utils";
import { createI18n } from "vue-i18n";
import { defineComponent, h, markRaw, nextTick } from "vue";
import { describe, expect, it } from "vitest";
import RouteErrorBoundary from "./RouteErrorBoundary.vue";

const messages = {
  en: {
    common: {
      pageLoadFailed: "The page could not be displayed",
      pageLoadFailedDesc: "Retry to reload it.",
      retry: "Retry",
    },
  },
};

describe("RouteErrorBoundary", () => {
  it("replaces a broken route with a safe retry state", async () => {
    let renders = 0;
    const flakyRoute = defineComponent({
      setup() {
        return () => {
          renders += 1;
          if (renders === 1) throw new Error("sensitive-runtime-detail");
          return h("div", { "data-test": "route-ready" }, "Recovered route");
        };
      },
    });
    const i18n = createI18n({ legacy: false, locale: "en", messages });
    const wrapper = mount(RouteErrorBoundary, {
      props: { component: markRaw(flakyRoute), resetKey: "/codex" },
      global: { plugins: [i18n] },
    });

    await nextTick();
    expect(wrapper.get('[data-test="route-error"]').text()).toContain("The page could not be displayed");
    expect(wrapper.html()).not.toContain("sensitive-runtime-detail");

    await wrapper.get('[data-test="route-error-retry"]').trigger("click");
    await nextTick();
    expect(wrapper.get('[data-test="route-ready"]').text()).toBe("Recovered route");
  });
});
