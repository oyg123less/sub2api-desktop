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

  const linkedElements = document.querySelectorAll(
    'a[href], img[src], source[srcset], link[rel="icon"][href], meta[property="og:image"][content], meta[name="twitter:image"][content]',
  );

  for (const element of linkedElements) {
    const attribute = element.tagName === "IMG"
      ? "src"
      : element.tagName === "SOURCE"
        ? "srcset"
        : element.tagName === "META"
          ? "content"
          : "href";
    const rawValue = element.getAttribute(attribute);
    if (!rawValue || rawValue.startsWith("mailto:") || rawValue.startsWith("tel:")) continue;

    const values = attribute === "srcset"
      ? rawValue.split(",").map((candidate) => candidate.trim().split(/\s+/)[0]).filter(Boolean)
      : [rawValue];

    for (const raw of values) {
      const url = new URL(raw, `https://amberapp.asia${currentRoute}`);
      if (url.hostname !== "amberapp.asia") continue;

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
}

if (errors.length) {
  console.error([...new Set(errors)].join("\n"));
  process.exit(1);
}

console.log(`Checked internal links, responsive images, and metadata assets across ${htmlFiles.length} HTML files.`);
