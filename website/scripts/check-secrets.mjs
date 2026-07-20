import fs from "node:fs";
import path from "node:path";

const roots = ["."];
const textExtensions = new Set([".css", ".html", ".js", ".json", ".md", ".mjs", ".py", ".svg", ".toml", ".ts", ".txt", ".vue", ".xml"]);
const patterns = [
  [/-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/g, "private key"],
  [/\bghp_[A-Za-z0-9]{30,}\b/g, "GitHub classic token"],
  [/\bgithub_pat_[A-Za-z0-9_]{40,}\b/g, "GitHub fine-grained token"],
  [/\bsk-(?:proj|live|local)-[A-Za-z0-9_-]{16,}\b/g, "API key"],
  [/(?:CLOUDFLARE_API_TOKEN|JWT_SECRET|TURNSTILE_SECRET|RESEND_API_KEY|RESEND_WEBHOOK_SECRET|QQ_SMTP_USER|QQ_SMTP_AUTH_CODE|ADMIN_API_KEY|SHARE_KMS_KEY|SHARE_CONNECT_PEPPER)\s*[:=]\s*["']?[A-Za-z0-9_+@\/=.-]{8,}/g, "assigned secret"],
];
const findings = [];

function walk(target) {
  if (!fs.existsSync(target)) return;
  for (const entry of fs.readdirSync(target, { withFileTypes: true })) {
    if (["node_modules", ".git", "playwright-report", "test-results"].includes(entry.name)) continue;
    const fullPath = path.join(target, entry.name);
    if (entry.isDirectory()) {
      walk(fullPath);
      continue;
    }
    if (!textExtensions.has(path.extname(entry.name).toLowerCase())) continue;
    const text = fs.readFileSync(fullPath, "utf8");
    for (const [pattern, label] of patterns) {
      pattern.lastIndex = 0;
      if (pattern.test(text)) findings.push(`${fullPath}: ${label}`);
    }
  }
}

for (const root of roots) walk(path.resolve(root));

if (findings.length) {
  console.error(findings.join("\n"));
  process.exit(1);
}

console.log("Secret-pattern scan passed for the website source, documentation, tests, and dist.");
