import type { Context } from "hono";
import { AppError, errorResponse } from "./errors";
import { decryptShareCredential } from "./share-crypto";
import { bytesToBase64URL, randomToken, sha256 } from "./security";
import type { AppEnv } from "./types";

const maxGatewayBody = 4 * 1024 * 1024;
const gatewayEncoder = new TextEncoder();

interface AccessGrant {
  key_id: number;
  key_public_id: string;
  recipient_id: number;
  recipient_public_id: string;
  recipient_status: string;
  rpm_limit: number;
  concurrency_limit: number;
  quota_requests: number;
  used_requests: number;
  reserved_requests: number;
  expires_at: string | null;
  group_id: number;
  group_public_id: string;
  group_status: string;
  owner_id: number;
}

interface GroupAccount {
  id: number;
  public_id: string;
  account_uid: string;
  account_type: "oauth" | "api_key";
  relay_mode: "owner_device" | "worker_direct";
  token_cipher: string | null;
  priority: number;
  weight: number;
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
    const parsed = JSON.parse(new TextDecoder().decode(body)) as { model?: unknown };
    return typeof parsed.model === "string" ? parsed.model.slice(0, 128) : "";
  } catch { return ""; }
}

function normalizeGatewayBody(body: ArrayBuffer): ArrayBuffer {
  try {
    const parsed = JSON.parse(new TextDecoder().decode(body)) as Record<string, unknown>;
    if (parsed.model !== "gpt-5.6") return body;
    parsed.model = "gpt-5.6-sol";
    return new Uint8Array(gatewayEncoder.encode(JSON.stringify(parsed))).buffer;
  } catch {
    return body;
  }
}

function upstreamURL(configured: string, path: string): string {
  const target = new URL(configured);
  if (path.endsWith("/chat/completions")) target.pathname = "/v1/chat/completions";
  else if (target.hostname.toLowerCase() === "api.openai.com") target.pathname = "/v1/responses";
  else target.pathname = "/backend-api/codex/responses";
  target.search = "";
  target.hash = "";
  return target.toString();
}

async function loadGrant(c: Context<AppEnv>, guestKey: string): Promise<AccessGrant> {
  const grant = await c.env.DB.prepare(`SELECT k.id AS key_id,k.public_id AS key_public_id,r.id AS recipient_id,
    r.public_id AS recipient_public_id,r.status AS recipient_status,r.rpm_limit,r.concurrency_limit,r.quota_requests,
    r.used_requests,r.reserved_requests,r.expires_at,g.id AS group_id,g.public_id AS group_public_id,g.status AS group_status,g.owner_id
    FROM share_access_keys k JOIN share_group_recipients r ON r.id=k.recipient_grant_id
    JOIN share_groups g ON g.id=r.group_id WHERE k.guest_key_hash=? AND k.status='active'`)
    .bind(await sha256(guestKey)).first<AccessGrant>();
  if (!grant) throw new AppError(401, "share_access_revoked", "The access key is invalid or has been revoked.");
  if (grant.group_status === "paused") throw new AppError(403, "share_group_paused", "The share owner paused this share group.");
  if (grant.group_status !== "active") throw new AppError(401, "share_access_revoked", "The access key is invalid or has been revoked.");
  if (grant.recipient_status === "paused") throw new AppError(403, "share_access_paused", "The share owner paused your access.");
  if (grant.recipient_status !== "active") throw new AppError(401, "share_access_revoked", "The access key is invalid or has been revoked.");
  if (grant.expires_at && Date.parse(grant.expires_at) <= Date.now()) throw new AppError(403, "share_access_expired", "The shared access has expired.");
  if (grant.quota_requests > 0 && grant.used_requests + grant.reserved_requests >= grant.quota_requests) {
    throw new AppError(429, "share_quota_exhausted", "The shared request quota has been exhausted.");
  }
  return grant;
}

async function reserve(c: Context<AppEnv>, grant: AccessGrant, accountID: number | null): Promise<string> {
  const now = new Date().toISOString();
  c.executionCtx.waitUntil(c.env.DB.prepare("DELETE FROM share_request_reservations_v2 WHERE expires_at<=?").bind(now).run());
  const reservationID = randomToken(18);
  try {
    await c.env.DB.prepare(`INSERT INTO share_request_reservations_v2
      (id,recipient_grant_id,access_key_id,group_account_id,state,created_at,updated_at,expires_at)
      VALUES(?,?,?,?,'pending',?,?,?)`).bind(reservationID, grant.recipient_id, grant.key_id, accountID, now, now,
        new Date(Date.now() + 10 * 60_000).toISOString()).run();
    return reservationID;
  } catch {
    throw new AppError(429, "share_quota_exhausted", "The shared request quota has been exhausted.");
  }
}

async function finishReservation(c: Context<AppEnv>, id: string, state: "settled" | "released"): Promise<void> {
  await c.env.DB.prepare(`UPDATE share_request_reservations_v2 SET state=?,updated_at=? WHERE id=? AND state='pending'`)
    .bind(state, new Date().toISOString(), id).run();
}

async function acquireAccess(c: Context<AppEnv>, grant: AccessGrant, ticket: string): Promise<DurableObjectStub> {
  const stub = c.env.SHARE_ACCESS.get(c.env.SHARE_ACCESS.idFromName(`access:${grant.key_public_id}`));
  const response = await stub.fetch("https://access.internal/acquire", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ticket, rpm: grant.rpm_limit, concurrency: grant.concurrency_limit }),
  });
  if (!response.ok) {
    const body = await response.json<{ error?: string; retry_after?: number }>();
    throw new AppError(429, body.error === "share_rate_limited" ? "share_rate_limited" : "share_concurrency_limited",
      body.error === "share_rate_limited" ? `Request rate exceeded. Retry in ${body.retry_after || 1} seconds.` : "The shared concurrency limit is full.");
  }
  return stub;
}

async function releaseAccess(stub: DurableObjectStub, ticket: string): Promise<void> {
  await stub.fetch("https://access.internal/release", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ticket }),
  });
}

async function directResponse(c: Context<AppEnv>, grant: AccessGrant, account: GroupAccount, body: ArrayBuffer): Promise<Response> {
  if (!account.token_cipher) throw new AppError(503, "shared_account_unavailable", "A shared account is unavailable.");
  const credential = await decryptShareCredential(c.env, grant.owner_id, account.account_uid, account.token_cipher);
  const headers = new Headers({
    Authorization: `Bearer ${credential.token}`,
    "Content-Type": "application/json",
    Accept: c.req.header("accept") || "text/event-stream, application/json",
    "User-Agent": "codex_cli_rs/0.4.0 (Amber Cloud Share)",
    originator: "codex_cli_rs",
    "OpenAI-Beta": "responses=experimental",
    session_id: crypto.randomUUID(),
  });
  try {
    const upstream = await fetch(upstreamURL(credential.upstream_url, c.req.path), { method: "POST", headers, body });
    if (upstream.status >= 400 && (upstream.headers.get("content-type") || "").toLowerCase().includes("text/html")) {
      await upstream.body?.cancel();
      return errorResponse(c, upstream.status, "share_upstream_rejected", "The shared upstream rejected the request.");
    }
    return new Response(upstream.body, { status: upstream.status, headers: safeResponseHeaders(upstream) });
  } catch {
    return errorResponse(c, 502, "share_upstream_unreachable", "The shared upstream service is unavailable.");
  }
}

async function relayResponse(c: Context<AppEnv>, grant: AccessGrant, account: GroupAccount, body: ArrayBuffer, requestID: string): Promise<Response> {
  const stub = c.env.OWNER_RELAY.get(c.env.OWNER_RELAY.idFromName(`owner:${grant.owner_id}`));
  return stub.fetch("https://relay.internal/request", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      request_id: requestID,
      group_id: grant.group_public_id,
      account_uid: account.account_uid,
      endpoint: c.req.path.endsWith("/chat/completions") ? "chat/completions" : "responses",
      model: requestModel(body),
      accept: c.req.header("accept") || "text/event-stream, application/json",
      body: bytesToBase64URL(new Uint8Array(body)),
    }),
  });
}

async function copyWithCompletion(response: Response, onComplete: () => Promise<void>): Promise<Response> {
  if (!response.body) {
    await onComplete();
    return response;
  }
  const reader = response.body.getReader();
  const stream = new ReadableStream<Uint8Array>({
    async pull(controller) {
      try {
        const next = await reader.read();
        if (next.done) {
          await onComplete();
          controller.close();
        } else controller.enqueue(next.value);
      } catch (error) {
        await onComplete();
        controller.error(error);
      }
    },
    async cancel(reason) {
      try { await reader.cancel(reason); } finally { await onComplete(); }
    },
  });
  return new Response(stream, { status: response.status, statusText: response.statusText, headers: response.headers });
}

export async function forwardGroupShare(c: Context<AppEnv>, guestKey: string): Promise<Response> {
  if (!guestKey.startsWith("sk-amber-") || guestKey.length > 256) throw new AppError(401, "share_access_revoked", "The access key is invalid or has been revoked.");
  const contentLength = Number(c.req.header("content-length") || 0);
  if (!Number.isFinite(contentLength) || contentLength > maxGatewayBody) throw new AppError(413, "request_too_large", "The gateway request is too large.");
  const requestBody = await c.req.arrayBuffer();
  if (requestBody.byteLength > maxGatewayBody) throw new AppError(413, "request_too_large", "The gateway request is too large.");
  const body = normalizeGatewayBody(requestBody);
  const grant = await loadGrant(c, guestKey);
  const accounts = await c.env.DB.prepare(`SELECT id,public_id,account_uid,account_type,relay_mode,token_cipher,priority,weight
    FROM share_group_accounts WHERE group_id=? AND enabled=1 ORDER BY priority ASC,id ASC`).bind(grant.group_id).all<GroupAccount>();
  if (!accounts.results.length) throw new AppError(503, "share_no_eligible_account", "The share group has no available accounts.");
  const ticket = randomToken(18);
  const accessStub = await acquireAccess(c, grant, ticket);
  const reservationID = await reserve(c, grant, null).catch(async (error) => { await releaseAccess(accessStub, ticket); throw error; });
  const started = Date.now();
  const requestID = newPublicRequestID();
  let response: Response | null = null;
  let selected: GroupAccount | null = null;
  for (const account of accounts.results) {
    selected = account;
    await c.env.DB.prepare("UPDATE share_request_reservations_v2 SET group_account_id=?,updated_at=? WHERE id=? AND state='pending'")
      .bind(account.id, new Date().toISOString(), reservationID).run();
    response = account.relay_mode === "worker_direct"
      ? await directResponse(c, grant, account, body)
      : await relayResponse(c, grant, account, body, requestID);
    if (response.status !== 503 || account === accounts.results[accounts.results.length - 1]) break;
    let retryable = false;
    try {
      const clone = response.clone();
      const error = await clone.json<{ error?: { code?: string } }>();
      retryable = error.error?.code === "owner_device_offline" || error.error?.code === "owner_relay_busy";
    } catch { retryable = false; }
    if (!retryable) break;
  }
  response ||= errorResponse(c, 503, "share_no_eligible_account", "The share group has no available accounts.");
  const settlement = response.status < 400 ? "settled" : "released";
  await finishReservation(c, reservationID, settlement);
  c.executionCtx.waitUntil(c.env.DB.prepare(`INSERT INTO share_usage_log_v2
    (request_id,group_id,recipient_grant_id,group_account_id,route_mode,model,status,error_code,latency_ms,created_at)
    VALUES(?,?,?,?,?,?,?,?,?,?)`).bind(requestID, grant.group_id, grant.recipient_id, selected?.id ?? null,
      selected?.relay_mode ?? "owner_device", requestModel(body), response.status, response.status >= 400 ? "request_failed" : null,
      Date.now() - started, new Date().toISOString()).run());
  return copyWithCompletion(response, () => releaseAccess(accessStub, ticket));
}

function newPublicRequestID(): string {
  return `req_${randomToken(18)}`;
}
