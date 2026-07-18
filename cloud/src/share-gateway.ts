import { Hono, type Context } from "hono";
import { AppError, errorResponse } from "./errors";
import { decryptShareCredential } from "./share-crypto";
import { randomToken, sha256 } from "./security";
import { forwardGroupShare } from "./share-group-gateway";
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
  reserved_requests: number;
  expires_at: string | null;
  revoked: number;
}

function bearerToken(value: string): string {
  return value.startsWith("Bearer ") ? value.slice(7).trim() : "";
}

async function loadGrant(c: Context<AppEnv>): Promise<GatewayGrant> {
  const guestKey = bearerToken(c.req.header("authorization") || "");
  if (!guestKey.startsWith("sk-share-") || guestKey.length > 512) {
    throw new AppError(401, "invalid_share_key", "The share key is invalid or has been revoked.");
  }
  const hash = await sha256(guestKey);
  const grant = await c.env.DB.prepare(`SELECT id,owner_id,account_uid,token_cipher,quota_requests,used_requests,reserved_requests,expires_at,revoked
    FROM share_grants WHERE guest_key_hash=?`).bind(hash).first<GatewayGrant>();
  if (!grant || grant.revoked) throw new AppError(401, "invalid_share_key", "The share key is invalid or has been revoked.");
  if (grant.expires_at && Date.parse(grant.expires_at) <= Date.now()) {
    throw new AppError(403, "share_expired", "The share has expired.");
  }
  if (grant.quota_requests > 0 && grant.used_requests + grant.reserved_requests >= grant.quota_requests) {
    throw new AppError(429, "share_quota_exhausted", "The share request quota has been exhausted.");
  }
  return grant;
}

async function reserveGrantUsage(c: Context<AppEnv>, grant: GatewayGrant): Promise<string> {
  const now = new Date().toISOString();
  await c.env.DB.prepare("DELETE FROM share_request_reservations WHERE expires_at<=?").bind(now).run();
  const reservationID = randomToken(18);
  try {
    await c.env.DB.prepare(`INSERT INTO share_request_reservations(id,grant_id,state,created_at,updated_at,expires_at)
      VALUES(?,?,'pending',?,?,?)`).bind(
        reservationID, grant.id, now, now, new Date(Date.now() + 10 * 60 * 1000).toISOString(),
      ).run();
    return reservationID;
  } catch {
    const current = await c.env.DB.prepare(`SELECT revoked,quota_requests,used_requests,reserved_requests,expires_at
      FROM share_grants WHERE id=?`).bind(grant.id)
      .first<Pick<GatewayGrant, "revoked" | "quota_requests" | "used_requests" | "reserved_requests" | "expires_at">>();
    if (!current || current.revoked) throw new AppError(401, "invalid_share_key", "The share key is invalid or has been revoked.");
    if (current.expires_at && Date.parse(current.expires_at) <= Date.now()) throw new AppError(403, "share_expired", "The share has expired.");
    throw new AppError(429, "share_quota_exhausted", "The share request quota has been exhausted.");
  }
}

async function finalizeGrantUsage(c: Context<AppEnv>, reservationID: string, state: "settled" | "released"): Promise<void> {
  await c.env.DB.prepare(`UPDATE share_request_reservations SET state=?,updated_at=? WHERE id=? AND state='pending'`)
    .bind(state, new Date().toISOString(), reservationID).run();
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
  const bearer = bearerToken(c.req.header("authorization") || "");
  if (bearer.startsWith("sk-amber-")) return forwardGroupShare(c, bearer);
  const contentLength = Number(c.req.header("content-length") || 0);
  if (!Number.isFinite(contentLength) || contentLength > maxGatewayBody) {
    throw new AppError(413, "request_too_large", "The gateway request is too large.");
  }
  const body = await c.req.arrayBuffer();
  if (body.byteLength > maxGatewayBody) throw new AppError(413, "request_too_large", "The gateway request is too large.");
  const grant = await loadGrant(c);
  const credential = await decryptShareCredential(c.env, grant.owner_id, grant.account_uid, grant.token_cipher);
  if (credential.account_type === "oauth") {
    throw new AppError(409, "oauth_device_relay_required", "OAuth sharing requires the owner device to be online and is not available in this version.");
  }
  const reservationID = await reserveGrantUsage(c, grant);
  const started = Date.now();
  let status = 502;
  try {
    const headers = new Headers({
      Authorization: `Bearer ${credential.token}`,
      "Content-Type": "application/json",
      Accept: c.req.header("accept") || "text/event-stream, application/json",
      "User-Agent": "codex_cli_rs/0.4.0 (Amber Cloud Share)",
      originator: "codex_cli_rs",
      "OpenAI-Beta": "responses=experimental",
      session_id: crypto.randomUUID(),
    });
    const upstream = await fetch(upstreamURL(credential.upstream_url, c.req.path, credential.account_type), {
      method: "POST",
      headers,
      body,
    });
    status = upstream.status;
    await finalizeGrantUsage(c, reservationID, status < 400 ? "settled" : "released");
    c.executionCtx.waitUntil(c.env.DB.prepare(`INSERT INTO share_usage_log(grant_id,ts,model,status,latency_ms)
      VALUES(?,?,?,?,?)`).bind(grant.id, new Date().toISOString(), requestModel(body), status, Date.now() - started).run());
    if (upstream.status >= 400 && (upstream.headers.get("content-type") || "").toLowerCase().includes("text/html")) {
      await upstream.body?.cancel();
      return errorResponse(c, upstream.status, "share_upstream_rejected", "The shared upstream rejected the request.");
    }
    return new Response(upstream.body, { status, headers: safeResponseHeaders(upstream) });
  } catch (error) {
    await finalizeGrantUsage(c, reservationID, "released");
    c.executionCtx.waitUntil(c.env.DB.prepare(`INSERT INTO share_usage_log(grant_id,ts,model,status,latency_ms)
      VALUES(?,?,?,?,?)`).bind(grant.id, new Date().toISOString(), requestModel(body), status, Date.now() - started).run());
    if (error instanceof AppError) throw error;
    return errorResponse(c, 502, "share_upstream_unreachable", "The shared upstream service is unavailable.");
  }
}

gateway.post("/responses", forward);
gateway.post("/chat/completions", forward);

export default gateway;
