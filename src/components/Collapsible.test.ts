import { mount } from "@vue/test-utils";
import { defineComponent, ref } from "vue";
import { describe, expect, it } from "vitest";
import Collapsible from "./Collapsible.vue";

describe("Collapsible", () => {
  it("is closed by default and emits update:open", async () => {
    const wrapper = mount(Collapsible, {
      slots: { trigger: "Advanced", default: "Hidden settings" },
    });
    expect(wrapper.get("button").attributes("aria-expanded")).toBe("false");
    expect(wrapper.get(".collapsible-content").attributes("aria-hidden")).toBe("true");
    await wrapper.get("button").trigger("click");
    expect(wrapper.emitted("update:open")).toEqual([[true]]);
  });

  it("supports v-model:open", async () => {
    const Host = defineComponent({
      components: { Collapsible },
      setup() {
        return { open: ref(false) };
      },
      template: `
        <Collapsible v-model:open="open">
          <template #trigger>Files</template>
          <span data-test="content">Config</span>
        </Collapsible>
      `,
    });
    const wrapper = mount(Host);
    await wrapper.get("button").trigger("click");
    expect(wrapper.get("button").attributes("aria-expanded")).toBe("true");
    expect(wrapper.get(".collapsible-content").attributes("aria-hidden")).toBe("false");
    await wrapper.get("button").trigger("click");
    expect(wrapper.get("button").attributes("aria-expanded")).toBe("false");
  });
});
