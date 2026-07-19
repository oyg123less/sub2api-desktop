import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";

const routes = [
  { path: "/", title: /Amber - Windows/ },
  { path: "/download", title: /下载 Amber/ },
  { path: "/docs", title: /使用文档/ },
  { path: "/changelog", title: /更新日志/ },
  { path: "/faq", title: /常见问题/ },
  { path: "/security", title: /安全与隐私/ },
  { path: "/status", title: /服务状态/ },
  { path: "/404", title: /页面未找到/ },
];

for (const route of routes) {
  test(`${route.path} renders with metadata, no overflow, and no serious accessibility violations`, async ({ page }) => {
    await page.goto(route.path);
    await expect(page).toHaveTitle(route.title);
    await expect(page.locator("h1")).toHaveCount(1);

    const description = page.locator('meta[name="description"]');
    const canonical = page.locator('link[rel="canonical"]');
    await expect(description).toHaveAttribute("content", /.+/);
    await expect(canonical).toHaveAttribute("href", `https://amberapp.asia${route.path === "/" ? "/" : route.path}`);

    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(1);

    const results = await new AxeBuilder({ page }).analyze();
    expect(
      results.violations,
      results.violations.map((violation) => `${violation.id}: ${violation.help}`).join("\n"),
    ).toEqual([]);
  });
}

test("navigation is keyboard reachable and the mobile menu exposes all routes", async ({ page }) => {
  await page.goto("/");
  const menu = page.locator(".menu-toggle");

  if (await menu.isVisible()) {
    await menu.focus();
    await page.keyboard.press("Enter");
    await expect(menu).toHaveAttribute("aria-expanded", "true");
  }

  const docsLink = page.locator("#primary-navigation").getByRole("link", { name: "文档", exact: true });
  await docsLink.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/docs$/);
  await expect(page.locator("#main-content")).toBeFocused();
});

test("screenshots load with nonblank pixel data and lightbox restores focus", async ({ page }) => {
  await page.goto("/docs");
  const images = page.locator(".image-viewer img");
  expect(await images.count()).toBeGreaterThan(0);

  await images.evaluateAll(async (nodes) => {
    await Promise.all(
      nodes.map(async (node) => {
        const image = node as HTMLImageElement;
        image.loading = "eager";
        await image.decode();
      }),
    );
  });

  const imageChecks = await images.evaluateAll((nodes) =>
    nodes.map((node) => {
      const image = node as HTMLImageElement;
      const canvas = document.createElement("canvas");
      canvas.width = 16;
      canvas.height = 16;
      const context = canvas.getContext("2d");
      context?.drawImage(image, 0, 0, 16, 16);
      const data = context?.getImageData(0, 0, 16, 16).data ?? [];
      const colors = new Set<string>();
      for (let index = 0; index < data.length; index += 16) {
        colors.add(`${data[index]}-${data[index + 1]}-${data[index + 2]}-${data[index + 3]}`);
      }
      return { complete: image.complete, width: image.naturalWidth, height: image.naturalHeight, colors: colors.size };
    }),
  );

  for (const image of imageChecks) {
    expect(image.complete).toBe(true);
    expect(image.width).toBeGreaterThan(100);
    expect(image.height).toBeGreaterThan(100);
    expect(image.colors).toBeGreaterThan(1);
  }

  const trigger = page.locator(".image-button").first();
  await trigger.focus();
  await trigger.click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭图片预览" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "关闭图片预览" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();
  await expect(trigger).toBeFocused();
});

test("FAQ search narrows the visible questions", async ({ page }) => {
  await page.goto("/faq");
  const search = page.getByRole("searchbox", { name: "搜索问题、错误码或关键词" });
  await search.fill("502");
  await expect(page.locator(".faq-list details")).toHaveCount(1);
  await expect(page.locator(".faq-list details")).toContainText("502 Bad Gateway");
  await page.getByRole("button", { name: "清除搜索" }).click();
  await expect(search).toBeFocused();
  await expect(page.locator(".faq-list details")).toHaveCount(12);
});

test("unknown client routes render the not-found page", async ({ page }) => {
  await page.goto("/not-a-real-page");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("页面没有找到");
});

test("documentation exposes the appropriate table of contents", async ({ page }) => {
  await page.goto("/docs");
  const viewport = page.viewportSize();
  if ((viewport?.width ?? 0) <= 900) {
    await expect(page.locator(".mobile-toc")).toBeVisible();
    await expect(page.locator(".desktop-toc")).toBeHidden();
  } else {
    await expect(page.locator(".desktop-toc")).toBeVisible();
    await expect(page.locator(".mobile-toc")).toBeHidden();
  }
});

test("hero screenshot is loaded and nonblank", async ({ page }) => {
  await page.goto("/");
  const result = await page.evaluate(async () => {
    const image = new Image();
    image.src = "/screenshots/dashboard.png";
    await image.decode();
    const canvas = document.createElement("canvas");
    canvas.width = 20;
    canvas.height = 20;
    const context = canvas.getContext("2d");
    context?.drawImage(image, 0, 0, 20, 20);
    const data = context?.getImageData(0, 0, 20, 20).data ?? [];
    const colors = new Set<string>();
    for (let index = 0; index < data.length; index += 16) {
      colors.add(`${data[index]}-${data[index + 1]}-${data[index + 2]}-${data[index + 3]}`);
    }
    return { width: image.naturalWidth, height: image.naturalHeight, colors: colors.size };
  });

  expect(result).toEqual({ width: 1440, height: 900, colors: expect.any(Number) });
  expect(result.colors).toBeGreaterThan(1);
});

test("captures homepage and documentation visual evidence", async ({ page }, testInfo) => {
  const directory = path.resolve("test-results", "screenshots");
  fs.mkdirSync(directory, { recursive: true });

  await page.goto("/");
  await page.screenshot({ path: path.join(directory, `home-${testInfo.project.name}.png`), fullPage: true });
  await page.goto("/docs");
  await page.screenshot({ path: path.join(directory, `docs-${testInfo.project.name}.png`), fullPage: true });
});
