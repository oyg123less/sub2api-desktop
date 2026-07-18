import { expect, test } from "@playwright/test";

test("loads the operational desktop shell without horizontal overflow", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("#app")).toBeVisible();
  await expect(page.getByText(/Amber/).first()).toBeVisible();
  const dimensions = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
  expect(dimensions.scroll).toBeLessThanOrEqual(dimensions.client);
});

test("exposes model and statistics navigation without a standalone diagnostics route", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator('a[href="#/diagnostics"]')).toHaveCount(0);
  await expect(page.locator('a[href="#/statistics"]')).toBeVisible();
  await expect(page.locator('a[href="#/models"]')).toBeVisible();
});

test("persists manual dark mode, copies the version, and respects reduced motion", async ({ page, context }, testInfo) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: "http://127.0.0.1:4173" });
  await page.addInitScript(() => {
    localStorage.setItem("s2a_theme", "light");
    localStorage.setItem("s2a_lang", "en");
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Use dark theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  expect(await page.evaluate(() => localStorage.getItem("s2a_theme"))).toBe("dark");
  const colors = await page.evaluate(() => {
    const style = getComputedStyle(document.documentElement);
    return { background: style.getPropertyValue("--bg"), card: style.getPropertyValue("--bg-card") };
  });
  expect(colors.background).not.toBe(colors.card);
  const radius = await page.locator(".card").first().evaluate((element) => parseFloat(getComputedStyle(element).borderRadius));
  expect(radius).toBeLessThanOrEqual(8);

  const versionCopyButton = page.locator(".version-copy");
  const displayedVersion = (await versionCopyButton.innerText()).trim();
  await versionCopyButton.click();
  await expect(page.getByText("Version copied")).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(displayedVersion);

  await page.emulateMedia({ reducedMotion: "reduce" });
  const duration = await page.locator(".nav-item").first().evaluate((element) => getComputedStyle(element).transitionDuration);
  expect(duration.split(",").every((value) => Number.parseFloat(value) <= 0.00001)).toBe(true);
  await page.screenshot({ path: `test-results/amber-dark-${testInfo.project.name}.png`, fullPage: true });
});
