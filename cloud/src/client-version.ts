import { createMiddleware } from "hono/factory";
import type { AppEnv } from "./types";

function parseVersion(value: string): [number, number, number] | null {
  const match = value.trim().replace(/^v/i, "").match(/^(\d+)\.(\d+)\.(\d+)/);
  if (!match) return null;
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

function compareVersion(left: [number, number, number], right: [number, number, number]): number {
  for (let index = 0; index < 3; index += 1) {
    if (left[index] !== right[index]) return (left[index] ?? 0) - (right[index] ?? 0);
  }
  return 0;
}

function requestVersion(headers: Headers): string {
  const explicit = headers.get("x-amber-client-version") || "";
  if (explicit) return explicit;
  return (headers.get("user-agent") || "").match(/Amber\/(\d+\.\d+\.\d+)/i)?.[1] || "";
}

export const requireCurrentClient = createMiddleware<AppEnv>(async (c, next) => {
  const path = c.req.path;
  if (path.startsWith("/v1/auth/") || path.startsWith("/v1/webhooks/") ||
      path === "/v1/responses" || path === "/v1/chat/completions" ||
      path === "/v1/relay/connect") return next();
  const settings = await c.env.DB.prepare(`SELECT key,value FROM platform_settings WHERE key IN
    ('enforce_client_version','minimum_client_version','latest_client_version','client_release_url')`).all<{ key: string; value: string }>();
  const values = Object.fromEntries(settings.results.map((entry) => [entry.key, entry.value]));
  if (values.enforce_client_version !== "true") return next();
  const minimumText = values.minimum_client_version || "0.4.2";
  const minimum = parseVersion(minimumText) || [0, 4, 2];
  const supplied = parseVersion(requestVersion(c.req.raw.headers));
  if (supplied && compareVersion(supplied, minimum) >= 0) return next();
  const latest = values.latest_client_version || minimumText;
  const updateURL = values.client_release_url || "https://github.com/oyg123less/sub2api-desktop/releases/latest";
  c.header("X-Amber-Minimum-Version", minimumText);
  c.header("X-Amber-Latest-Version", latest);
  c.header("X-Amber-Update-URL", updateURL);
  return c.json({ error: {
    code: "client_upgrade_required",
    message: `请更新到 Amber ${minimumText} 或更高版本后再使用云账户。下载并覆盖安装（本地数据会保留）：${updateURL}`,
    minimum_version: minimumText, latest_version: latest, update_url: updateURL,
  }, request_id: c.get("requestId") }, 426);
});
