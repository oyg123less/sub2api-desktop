import fs from "node:fs";
import path from "node:path";
import { JSDOM } from "jsdom";

const dist = path.resolve("dist");
const errors = [];
const htmlFiles = [];

function walk(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const fullPath = path.join(directory, entry.name);
    if (entry.isDirectory()) walk(fullPath);
    else if (entry.name.endsWith(".html")) htmlFiles.push(fullPath);
  }
}

function routeCandidates(pathname) {
  if (pathname === "/") return [path.join(dist, "index.html")];
  const clean = pathname.replace(/^\//, "").replace(/\/$/, "");
  return [path.join(dist, clean, "index.html"), path.join(dist, `${clean}.html`)];
}

function builtRoute(pathname) {
  return routeCandidates(pathname).find((candidate) => fs.existsSync(candidate));
}

walk(dist);

for (const htmlFile of htmlFiles) {
  const source = fs.readFileSync(htmlFile, "utf8");
  const dom = new JSDOM(source);
  const { document } = dom.window;
  const relativeHtml = path.relative(dist, htmlFile).replaceAll("\\", "/");
  const currentRoute = relativeHtml === "index.html"
    ? "/"
    : `/${relativeHtml.replace(/\/index\.html$/, "/").replace(/\.html$/, "")}`;

  for (const element of document.querySelectorAll("a[href], img[src]")) {
    const attribute = element.tagName === "IMG" ? "src" : "href";
    const raw = element.getAttribute(attribute);
    if (!raw || raw.startsWith("http:") || raw.startsWith("https:") || raw.startsWith("mailto:") || raw.startsWith("tel:")) continue;

    const url = new URL(raw, `https://amberapp.asia${currentRoute}`);
    const extension = path.extname(url.pathname);

    if (extension && extension !== ".html") {
      const asset = path.join(dist, url.pathname.replace(/^\//, ""));
      if (!fs.existsSync(asset)) errors.push(`${path.relative(dist, htmlFile)}: missing asset ${url.pathname}`);
      continue;
    }

    const targetFile = builtRoute(url.pathname);
    if (!targetFile) {
      errors.push(`${path.relative(dist, htmlFile)}: missing route ${url.pathname}`);
      continue;
    }

    if (url.hash) {
      const target = new JSDOM(fs.readFileSync(targetFile, "utf8")).window.document;
      const id = decodeURIComponent(url.hash.slice(1));
      if (!target.getElementById(id)) errors.push(`${path.relative(dist, htmlFile)}: missing anchor ${url.pathname}${url.hash}`);
    }
  }
}

if (errors.length) {
  console.error([...new Set(errors)].join("\n"));
  process.exit(1);
}

console.log(`Checked internal links and image sources across ${htmlFiles.length} HTML files.`);
