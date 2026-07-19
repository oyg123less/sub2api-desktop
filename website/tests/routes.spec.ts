import { describe, expect, it } from "vitest";
import { routes } from "../src/router";

const publicPaths = ["/", "/download", "/docs", "/changelog", "/faq", "/security", "/status", "/404"];

describe("public routes", () => {
  it("defines every required page with unique SEO metadata", () => {
    const staticRoutes = routes.filter((route) => publicPaths.includes(route.path));
    expect(staticRoutes.map((route) => route.path)).toEqual(publicPaths);

    const titles = staticRoutes.map((route) => route.meta?.title);
    expect(new Set(titles).size).toBe(publicPaths.length);

    for (const route of staticRoutes) {
      expect(route.meta?.title).toBeTruthy();
      expect(route.meta?.description).toBeTruthy();
    }
  });
});
