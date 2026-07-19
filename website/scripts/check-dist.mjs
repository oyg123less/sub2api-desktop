import fs from "node:fs";
import path from "node:path";
import { JSDOM } from "jsdom";

const dist = path.resolve("dist");
const routes = ["/", "/download", "/docs", "/changelog", "/faq", "/security", "/status", "/404"];
const errors = [];

function routeFile(route) {
  if (route === "/") return path.join(dist, "index.html");
  const clean = route.slice(1);
  const candidates = [path.join(dist, clean, "index.html"), path.join(dist, `${clean}.html`)];
  return candidates.find((candidate) => fs.existsSync(candidate)) ?? candidates[0];
}

for (const route of routes) {
  const file = routeFile(route);
  if (!fs.existsSync(file)) {
    errors.push(`Missing generated HTML for ${route}: ${file}`);
    continue;
  }

  const dom = new JSDOM(fs.readFileSync(file, "utf8"));
  const { document } = dom.window;
  const expectedCanonical = `https://amberapp.asia${route === "/" ? "/" : route}`;
  const title = document.querySelector("title")?.textContent?.trim();
  const description = document.querySelector('meta[name="description"]')?.getAttribute("content")?.trim();
  const canonical = document.querySelector('link[rel="canonical"]')?.getAttribute("href");
  const ogTitle = document.querySelector('meta[property="og:title"]')?.getAttribute("content")?.trim();

  if (!title) errors.push(`${route}: missing title`);
  if (!description) errors.push(`${route}: missing description`);
  if (canonical !== expectedCanonical) errors.push(`${route}: canonical is ${canonical ?? "missing"}`);
  if (!ogTitle) errors.push(`${route}: missing og:title`);
  if (document.querySelectorAll("h1").length !== 1) errors.push(`${route}: expected exactly one h1`);
}

for (const publicFile of ["robots.txt", "sitemap.xml", "_headers", "app-icon.png"]) {
  if (!fs.existsSync(path.join(dist, publicFile))) errors.push(`Missing public build artifact: ${publicFile}`);
}

const redirectsFile = path.join(dist, "_redirects");
if (fs.existsSync(redirectsFile) && /\/\*\s+\/index\.html\s+200/.test(fs.readFileSync(redirectsFile, "utf8"))) {
  errors.push("SPA fallback would turn unknown routes into HTTP 200 soft 404s");
}

const homeHtml = fs.existsSync(routeFile("/")) ? fs.readFileSync(routeFile("/"), "utf8") : "";
if (!homeHtml.includes('"@type": "SoftwareApplication"')) errors.push("Home page is missing SoftwareApplication JSON-LD");
if (!homeHtml.includes("v0.4.4")) errors.push("Home page does not identify v0.4.4 as stable");
if (homeHtml.includes("__AMBER_")) errors.push("Release metadata placeholders were not replaced");

const jsonLdText = new JSDOM(homeHtml).window.document.querySelector('script[type="application/ld+json"]')?.textContent;
if (jsonLdText) {
  const jsonLd = JSON.parse(jsonLdText);
  if (jsonLd.softwareVersion !== "0.4.4") errors.push("SoftwareApplication version does not match the stable release");
  if (!jsonLd.downloadUrl?.endsWith("/v0.4.4/Amber_0.4.4_x64-setup.exe")) {
    errors.push("SoftwareApplication download URL does not match the stable installer");
  }
}

if (errors.length) {
  console.error(errors.join("\n"));
  process.exit(1);
}

console.log(`Verified ${routes.length} generated routes and public metadata artifacts.`);
