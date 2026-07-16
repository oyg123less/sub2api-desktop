import { Hono, type Context } from "hono";
import { AppError, errorResponse } from "./errors";
import { decryptShareCredential } from "./share-crypto";
import { randomToken, sha256 } from "./security";
import type { AppEnv } from "./types";

const gateway = new Hono<AppEnv>();
const maxGatewayBody = 4 * 1024 * 1024;

interface GatewayGrant {
  id: number;
  owner_id: number;
  account_uid: string;
  token_cipher: string;
  quota_requests: number;
  used_requests: number;
  expires_at: string | null;
  revoked: number;
}

function bearerToken(value: string): string {
  return value.startsWith("Bearer ") ? value.slice(7).trim() : "";
}

async function claimGrant(c: Context<AppEnv>): Promise<GatewayGrant> {
  const guestKey = bearerToken(c.req.header("authorization") || "");
  if (!guestKey.startsWith("sk-share-") || guestKey.length > 512) {
    throw new AppError(401, "invalid_share_key", "The share key is invalid or has been revoked.");
  }
  const hash = await sha256(guestKey);
  const grant = await c.env.DB.prepare(`SELECT id,owner_id,account_uid,token_cipher,quota_requests,used_requests,expires_at,revoked
    FROM share_grants WHERE guest_key_hash=?`).bind(hash).first<GatewayGrant>();
  if (!grant || grant.revoked) throw new AppError(401, "invalid_share_key", "The share key is invalid or has been revoked.");
  if (grant.expires_at && Date.parse(grant.expires_at) <= Date.now()) {
    throw new AppError(403, "share_expired", "The share has expired.");
  }
  if (grant.quota_requests > 0 && grant.used_requests >= grant.quota_requests) {
    throw new AppError(429, "share_quota_exhausted", "The share request quota has been exhausted.");
  }
  const claimed = await c.env.DB.prepare(`UPDATE share_grants SET used_requests=used_requests+1,updated_at=?
    WHERE id=? AND revoked=0 AND (expires_at IS NULL OR expires_at>?)
      AND (quota_requests=0 OR used_requests<quota_requests)`).bind(
        new Date().toISOString(), grant.id, new Date().toISOString(),
      ).run();
  if (!claimed.meta.changes) {
    const current = await c.env.DB.prepare("SELECT revoked,quota_requests,used_requests,expires_at FROM share_grants WHERE id=?")
      .bind(grant.id).first<Pick<GatewayGrant, "revoked" | "quota_requests" | "used_requests" | "expires_at">>();
    if (!current || current.revoked) throw new AppError(401, "invalid_share_key", "The share key is invalid or has been revoked.");
    if (current.expires_at && Date.parse(current.expires_at) <= Date.now()) throw new AppError(403, "share_expired", "The share has expired.");
    throw new AppError(429, "share_quota_exhausted", "The share request quota has been exhausted.");
  }
  return grant;
}

function safeResponseHeaders(upstream: Response): Headers {
  const headers = new Headers();
  for (const name of ["content-type", "openai-processing-ms", "x-request-id", "x-ratelimit-limit-requests", "x-ratelimit-remaining-requests", "x-ratelimit-reset-requests"]) {
    const value = upstream.headers.get(name);
    if (value) headers.set(name, value);
  }
  headers.set("Cache-Control", "no-store");
  headers.set("X-Content-Type-Options", "nosniff");
  return headers;
}

function requestModel(body: ArrayBuffer): string {
  try {
    const value = JSON.parse(new TextDecoder().decode(body)) as { model?: unknown };
    return typeof value.model === "string" ? value.model.slice(0, 128) : "";
  } catch {
    return "";
  }
}

function upstreamURL(configured: string, path: string, accountType: "oauth" | "api_key"): string {
  const target = new URL(configured);
  if (path.endsWith("/chat/completions")) {
    if (accountType !== "api_key" || target.hostname.toLowerCase() !== "api.openai.com") {
      throw new AppError(400, "share_endpoint_unsupported", "This shared account supports the Responses API endpoint.");
    }
    target.pathname = "/v1/chat/completions";
  } else if (target.hostname.toLowerCase() === "api.openai.com") {
    target.pathname = "/v1/responses";
  } else {
    target.pathname = "/backend-api/codex/responses";
  }
  target.search = "";
  target.hash = "";
  return target.toString();
}

async function forward(c: Context<AppEnv>) {
  const contentLength = Number(c.req.header("content-length") || 0);
  if (!Number.isFinite(contentLength) || contentLength > maxGatewayBody) {
    throw new AppError(413, "request_too_large", "The gateway request is too large.");
  }
  const body = await c.req.arrayBuffer();
  if (body.byteLength > maxGatewayBody) throw new AppError(413, "request_too_large", "The gateway request is too large.");
  const grant = await claimGrant(c);
  const credential = await decryptShareCredential(c.env, grant.owner_id, grant.account_uid, grant.token_cipher);
  const started = Date.now();
  let status = 502;
  try {
    const headers = new Headers({
      Authorization: `Bearer ${credential.token}`,
      "Content-Type": "application/json",
      Accept: c.req.header("accept") || "text/event-stream, application/json",
      "User-Agent": "codex_cli_rs/0.3.1 (Amber Cloud Share)",
      originator: "codex_cli_rs",
      "OpenAI-Beta": "responses=experimental",
      session_id: crypto.randomUUID(),
    });
    if (credential.account_type === "oauth" && credential.chatgpt_account_id) {
      headers.set("chatgpt-account-id", credential.chatgpt_account_id);
    }
    const upstream = await fetch(upstreamURL(credential.upstream_url, c.req.path, credential.account_type), {
      method: "POST",
      headers,
      body,
    });
    status = upstream.status;
    c.executionCtx.waitUntil(c.env.DB.prepare(`INSERT INTO share_usage_log(grant_id,ts,model,status,latency_ms)
      VALUES(?,?,?,?,?)`).bind(grant.id, new Date().toISOString(), requestModel(body), status, Date.now() - started).run());
    return new Response(upstream.body, { status, headers: safeResponseHeaders(upstream) });
  } catch (error) {
    c.executionCtx.waitUntil(c.env.DB.prepare(`INSERT INTO share_usage_log(grant_id,ts,model,status,latency_ms)
      VALUES(?,?,?,?,?)`).bind(grant.id, new Date().toISOString(), requestModel(body), status, Date.now() - started).run());
    if (error instanceof AppError) throw error;
    return errorResponse(c, 502, "share_upstream_unreachable", "The shared upstream service is unavailable.");
  }
}

gateway.post("/responses", forward);
gateway.post("/chat/completions", forward);

export default gateway;
