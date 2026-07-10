import { expect, test } from "@playwright/test";

test("loads the operational desktop shell without horizontal overflow", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("#app")).toBeVisible();
  await expect(page.getByText(/Amber/).first()).toBeVisible();
  const dimensions = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }));
  expect(dimensions.scroll).toBeLessThanOrEqual(dimensions.client);
});

test("exposes diagnostics and statistics navigation", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator('a[href="#/diagnostics"]')).toBeVisible();
  await expect(page.locator('a[href="#/statistics"]')).toBeVisible();
});
