import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { faqs, homeFaqs } from "../../src/data/faq";
import { capabilityItems, showcaseItems } from "../../src/data/home";

const routes = [
  { path: "/", title: /Amber - Windows/ },
  { path: "/download", title: /下载 Amber/ },
  { path: "/docs", title: /使用文档/ },
  { path: "/changelog", title: /更新日志/ },
  { path: "/faq", title: /常见问题/ },
  { path: "/security", title: /安全与隐私/ },
  { path: "/status", title: /服务状态/ },
  { path: "/404", title: /页面未找到/ },
] as const;

const heroViewports = [
  { width: 1440, height: 900 },
  { width: 1280, height: 720 },
  { width: 900, height: 650 },
  { width: 412, height: 915 },
  { width: 320, height: 568 },
] as const;

async function expectNonblankImage(image: Locator, expectedSrc?: string) {
  await expect(image).toHaveCount(1);
  if (expectedSrc) await expect(image).toHaveAttribute("src", expectedSrc);
  await image.scrollIntoViewIfNeeded();

  const metrics = await image.evaluate(async (node) => {
    const element = node as HTMLImageElement;
    element.loading = "eager";
    await element.decode();

    const canvas = document.createElement("canvas");
    canvas.width = 20;
    canvas.height = 20;
    const context = canvas.getContext("2d");
    context?.drawImage(element, 0, 0, canvas.width, canvas.height);
    const data = context?.getImageData(0, 0, canvas.width, canvas.height).data ?? [];
    const colors = new Set<string>();
    for (let index = 0; index < data.length; index += 16) {
      colors.add(`${data[index]}-${data[index + 1]}-${data[index + 2]}-${data[index + 3]}`);
    }

    return {
      complete: element.complete,
      width: element.naturalWidth,
      height: element.naturalHeight,
      colors: colors.size,
      currentSrc: new URL(element.currentSrc).pathname,
    };
  });

  expect(metrics.complete).toBe(true);
  expect(metrics.width).toBeGreaterThan(100);
  expect(metrics.height).toBeGreaterThan(100);
  expect(metrics.colors).toBeGreaterThan(1);
  if (expectedSrc) expect(metrics.currentSrc).toBe(expectedSrc);
}

async function prepareVisualEvidence(page: Page) {
  await page.evaluate(async () => {
    for (const image of document.querySelectorAll<HTMLImageElement>("img")) image.loading = "eager";

    const settle = () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      });
    const step = Math.max(160, Math.floor(window.innerHeight * 0.45));
    let position = 0;

    while (position < document.documentElement.scrollHeight - window.innerHeight) {
      window.scrollTo(0, position);
      await settle();
      position += step;
    }
    window.scrollTo(0, document.documentElement.scrollHeight);
    await settle();
  });

  const revealCount = await page.locator(".reveal").count();
  if (revealCount > 0) {
    await expect.poll(async () => page.locator(".reveal-ready").count(), { timeout: 10_000 }).toBe(revealCount);
    for (let index = 0; index < revealCount; index += 1) {
      const reveal = page.locator(".reveal").nth(index);
      if (!(await reveal.evaluate((node) => node.classList.contains("is-visible")))) {
        await reveal.scrollIntoViewIfNeeded();
        await expect(reveal).toHaveClass(/is-visible/);
      }
    }
    await expect.poll(async () => page.locator(".reveal-ready:not(.is-visible)").count(), { timeout: 10_000 }).toBe(0);
  }

  const imageFailures = await page.locator("img").evaluateAll(async (nodes) => {
    const results = await Promise.all(
      nodes.map(async (node) => {
        const image = node as HTMLImageElement;
        image.loading = "eager";
        try {
          await image.decode();
          return null;
        } catch (error) {
          return {
            src: image.getAttribute("src"),
            currentSrc: image.currentSrc,
            width: image.naturalWidth,
            height: image.naturalHeight,
            error: error instanceof Error ? error.message : String(error),
          };
        }
      }),
    );
    return results.filter((result) => result !== null);
  });
  expect(imageFailures, "visual evidence images must decode").toEqual([]);
  await page.evaluate(async () => {
    document.documentElement.style.scrollBehavior = "auto";
    window.scrollTo(0, 0);
    await document.fonts.ready;
    await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
  });
  await expect.poll(async () => page.evaluate(() => window.scrollY)).toBe(0);
  if (revealCount > 0) {
    await expect
      .poll(
        async () =>
          page.locator(".reveal").evaluateAll((nodes) =>
            nodes.every((node) => Number.parseFloat(getComputedStyle(node).opacity) >= 0.999),
          ),
        { timeout: 5_000 },
      )
      .toBe(true);
  }
}

async function captureFullPage(page: Page, filePath: string) {
  const pageMetrics = await page.evaluate(() => ({
    bodyHeight: document.body.scrollHeight,
    documentHeight: document.documentElement.scrollHeight,
    textLength: document.body.innerText.length,
  }));
  const screenshot = await page.screenshot({ fullPage: true, scale: "css" });
  const width = screenshot.readUInt32BE(16);
  const height = screenshot.readUInt32BE(20);
  expect(width, `${path.basename(filePath)} PNG width`).toBeGreaterThan(0);
  expect(height, `${path.basename(filePath)} must exceed the viewport: ${JSON.stringify(pageMetrics)}`).toBeGreaterThan(
    page.viewportSize()?.height ?? 0,
  );
  expect(screenshot.byteLength, `${path.basename(filePath)} PNG bytes`).toBeGreaterThan(20_000);
  fs.writeFileSync(filePath, screenshot);
}

async function captureLocator(locator: Locator, filePath: string) {
  const screenshot = await locator.screenshot();
  const width = screenshot.readUInt32BE(16);
  const height = screenshot.readUInt32BE(20);
  expect(screenshot.byteLength, `${path.basename(filePath)} PNG bytes`).toBeGreaterThan(20_000);
  expect(width, `${path.basename(filePath)} PNG width`).toBeGreaterThan(100);
  expect(height, `${path.basename(filePath)} PNG height`).toBeGreaterThan(100);
  fs.writeFileSync(filePath, screenshot);
}

for (const route of routes) {
  test(`${route.path} renders with metadata, no overflow, and no serious accessibility violations`, async ({ page }) => {
    await page.goto(route.path);
    await expect(page).toHaveTitle(route.title);
    await expect(page.locator("h1")).toHaveCount(1);

    const expectedUrl = `https://amberapp.asia${route.path === "/" ? "/" : route.path}`;
    const description = page.locator('meta[name="description"]');
    const canonical = page.locator('link[rel="canonical"]');
    const openGraphUrl = page.locator('meta[property="og:url"]');
    await expect(description).toHaveAttribute("content", /.+/);
    await expect(canonical).toHaveAttribute("href", expectedUrl);
    await expect(openGraphUrl).toHaveAttribute("content", expectedUrl);

    const canonicalHref = await canonical.getAttribute("href");
    const openGraphHref = await openGraphUrl.getAttribute("content");
    for (const url of [canonicalHref, openGraphHref]) {
      expect(url).not.toBeNull();
      expect(url).not.toContain("www.");
      expect(new URL(url as string).hostname).toBe("amberapp.asia");
    }

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
  const navigation = page.getByRole("navigation", { name: "主导航" });

  if ((page.viewportSize()?.width ?? 0) <= 900) {
    await expect(menu).toBeVisible();
    await menu.focus();
    await page.keyboard.press("Enter");
    await expect(menu).toHaveAttribute("aria-expanded", "true");

    for (const linkName of ["首页", "产品", "文档", "更新", "FAQ", "安全", "状态", "GitHub"]) {
      await expect(navigation.getByRole("link", { name: linkName, exact: true })).toBeVisible();
    }
    await expect(navigation.getByRole("link", { name: /下载 v/ })).toBeVisible();
  }

  const docsLink = navigation.getByRole("link", { name: "文档", exact: true });
  await docsLink.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/docs$/);
  await expect(page.locator("#main-content")).toBeFocused();
});

test("documentation screenshots are nonblank and the lightbox restores focus", async ({ page }) => {
  await page.goto("/docs");
  const images = page.locator(".image-viewer img");
  const imageCount = await images.count();
  expect(imageCount).toBeGreaterThan(0);

  for (let index = 0; index < imageCount; index += 1) {
    await expectNonblankImage(images.nth(index));
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
  await expect(page.locator(".faq-list details")).toHaveCount(faqs.length);
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

test("product showcase supports keyboard navigation without stage movement and every image is nonblank", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop", "Showcase interaction contract runs once in the desktop project.");

  await page.goto("/");
  const showcase = page.locator("#product-showcase");
  const tabs = showcase.getByRole("tab");
  const stage = showcase.getByTestId("showcase-stage");
  await showcase.scrollIntoViewIfNeeded();
  await expect(tabs).toHaveCount(showcaseItems.length);
  await expect(stage).toBeVisible();

  const heights: number[] = [];
  await tabs.first().focus();
  for (let index = 0; index < showcaseItems.length; index += 1) {
    if (index > 0) await page.keyboard.press("ArrowRight");

    const item = showcaseItems[index];
    const tab = tabs.nth(index);
    const panel = page.locator(`#showcase-panel-${item.id}`);
    await expect(tab).toBeFocused();
    await expect(tab).toHaveAttribute("aria-selected", "true");
    await expect(tab).toHaveAttribute("aria-controls", `showcase-panel-${item.id}`);
    await expect(panel).toBeVisible();
    await expectNonblankImage(panel.locator("img"), item.image);

    const bounds = await stage.boundingBox();
    expect(bounds).not.toBeNull();
    if (bounds) heights.push(bounds.height);
  }

  expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(2);
  await page.keyboard.press("ArrowRight");
  await expect(tabs.first()).toBeFocused();
  await expect(tabs.first()).toHaveAttribute("aria-selected", "true");
});

test("interactive cards hover on desktop and remain still with reduced motion", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop", "Pointer and reduced-motion contract runs once in the desktop project.");

  await page.goto("/");
  let card = page.locator(".capability-card").first();
  await card.scrollIntoViewIfNeeded();
  await expect(page.locator(".capability-grid .reveal").first()).toHaveClass(/is-visible/);
  await card.hover();
  await expect.poll(async () => card.evaluate((node) => getComputedStyle(node).transform)).not.toBe("none");

  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.reload();
  card = page.locator(".capability-card").first();
  await card.scrollIntoViewIfNeeded();
  await card.hover();
  await expect.poll(async () => card.evaluate((node) => getComputedStyle(node).transform)).toBe("none");
});

test("touch viewports expose card actions without hover", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "mobile", "Touch-only action visibility runs in the mobile project.");

  await page.goto("/");
  const cardWrappers = page.locator(".capability-grid .reveal");
  const actions = page.locator(".capability-card .card-link");
  await expect(cardWrappers).toHaveCount(capabilityItems.length);
  await expect(actions).toHaveCount(capabilityItems.length);

  for (let index = 0; index < capabilityItems.length; index += 1) {
    await cardWrappers.nth(index).scrollIntoViewIfNeeded();
    await expect(cardWrappers.nth(index)).toHaveClass(/is-visible/);
    await expect(actions.nth(index)).toBeVisible();
    const styles = await actions.nth(index).evaluate((node) => {
      const computed = getComputedStyle(node);
      return {
        opacity: computed.opacity,
        visibility: computed.visibility,
        pointerEvents: computed.pointerEvents,
      };
    });
    expect(styles).toEqual({ opacity: "1", visibility: "visible", pointerEvents: "auto" });
  }
});

test("the Hero reveals the next section at every required viewport", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop", "Responsive viewport matrix runs once in the desktop project.");
  test.setTimeout(60_000);

  for (const viewport of heroViewports) {
    await page.setViewportSize(viewport);
    await page.goto("/");
    await page.evaluate(() => document.fonts.ready);
    await expect(page.locator(".hero-copy")).toHaveClass(/is-visible/);
    await expectNonblankImage(page.locator(".hero-product img"));
    await expect(page.getByRole("link", { name: "下载 Windows", exact: true })).toBeVisible();

    const layout = await page.evaluate(() => {
      const product = document.querySelector(".hero-product")?.getBoundingClientRect();
      const nextSection = document.querySelector(".home-trust")?.getBoundingClientRect();
      return {
        overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        productWidth: product?.width ?? 0,
        productHeight: product?.height ?? 0,
        nextSectionTop: nextSection?.top ?? Number.POSITIVE_INFINITY,
        nextSectionBottom: nextSection?.bottom ?? Number.NEGATIVE_INFINITY,
        viewportHeight: window.innerHeight,
      };
    });

    expect(layout.overflow, `${viewport.width}x${viewport.height} horizontal overflow`).toBeLessThanOrEqual(1);
    expect(layout.productWidth).toBeGreaterThan(0);
    expect(layout.productHeight).toBeGreaterThan(0);
    expect(layout.nextSectionTop, `${viewport.width}x${viewport.height} next section top`).toBeLessThan(layout.viewportHeight);
    expect(layout.nextSectionBottom).toBeGreaterThan(0);
  }
});

test("all public routes avoid horizontal overflow at 320px and 640px", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop", "Small-width route matrix runs once; 900px and 1440px run in their projects.");
  test.setTimeout(90_000);

  for (const width of [320, 640]) {
    await page.setViewportSize({ width, height: width === 320 ? 568 : 900 });
    for (const route of routes) {
      await page.goto(route.path);
      await page.evaluate(() => document.fonts.ready);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow, `${route.path} at ${width}px`).toBeLessThanOrEqual(1);
    }
  }
});

test("homepage FAQ renders the same shared questions as the full FAQ page", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop", "Shared FAQ data contract runs once in the desktop project.");

  await page.goto("/");
  const homeItems = page.locator(".home-faq-item");
  await expect(homeItems).toHaveCount(homeFaqs.length);
  for (const faq of homeFaqs) {
    await expect(page.locator(`[data-faq-id="${faq.id}"] summary span`).first()).toHaveText(faq.question);
  }

  await page.goto("/faq");
  for (const faq of homeFaqs) {
    await expect(page.locator(`#${faq.id} .faq-question`)).toHaveText(faq.question);
  }
});

test("captures all required visual evidence", async ({ page }, testInfo) => {
  test.setTimeout(120_000);
  const directory = path.resolve("test-results", "screenshots");
  fs.mkdirSync(directory, { recursive: true });

  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1, name: "Amber", exact: true })).toBeVisible();
  await prepareVisualEvidence(page);
  await captureFullPage(page, path.join(directory, `home-${testInfo.project.name}.png`));

  if (testInfo.project.name === "desktop") {
    const showcase = page.locator("#product-showcase");
    await showcase.scrollIntoViewIfNeeded();
    await captureLocator(showcase, path.join(directory, "showcase-desktop.png"));

    await page.goto("/docs");
    await expect(page.locator("h1")).toBeVisible();
    await prepareVisualEvidence(page);
    await captureFullPage(page, path.join(directory, "docs-desktop.png"));
  }

  if (testInfo.project.name === "mobile") {
    await page.goto("/download");
    await expect(page.getByRole("heading", { level: 1, name: /下载 Amber/ })).toBeVisible();
    await prepareVisualEvidence(page);
    await captureFullPage(page, path.join(directory, "download-mobile.png"));
  }
});
