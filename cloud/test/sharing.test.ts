import { env, SELF } from "cloudflare:test";
import { describe, expect, it, vi } from "vitest";
import { bytesToBase64URL, sha256 } from "../src/security";

const authHash = bytesToBase64URL(new Uint8Array(32).fill(41));
const accountUID = "018f1f46-7a19-7cc2-88cb-f577e51d3999";
const upstreamToken = "upstream-owner-token-must-never-leak";
const streamHoldMs = Number((import.meta as unknown as { env: Record<string, string | undefined> }).env.VITE_AMBER_STREAM_HOLD_MS || 25);

async function ownerSession() {
  const email = "share-owner@example.test";
  const auth = {
    email,
    turnstile_token: "test-pass",
    auth_hash: authHash,
    salt_kdf: bytesToBase64URL(new Uint8Array(16).fill(42)),
    salt_auth: bytesToBase64URL(new Uint8Array(16).fill(43)),
    wrapped_vault_key: `v1.${bytesToBase64URL(new Uint8Array(60).fill(44))}`,
  };
  await SELF.fetch("https://amber.test/v1/auth/register", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(auth),
  });
  const code = await env.SESSIONS.get(`test-mail:${await sha256(email)}`);
  await SELF.fetch("https://amber.test/v1/auth/verify-email", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, code }),
  });
  const login = await SELF.fetch("https://amber.test/v1/auth/login", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, auth_hash: authHash }),
  });
  const session = await login.json<{ access_token: string }>();
  const headers = { Authorization: `Bearer ${session.access_token}`, "Content-Type": "application/json" };
  const ciphertext = `v1.${bytesToBase64URL(new Uint8Array(80).fill(45))}`;
  const uploaded = await SELF.fetch("https://amber.test/v1/vault/batch", {
    method: "PUT", headers,
    body: JSON.stringify({ items: [{ kind: "account", client_uid: accountUID, ciphertext, version: 0, deleted: false }] }),
  });
  expect(uploaded.status).toBe(200);
  return headers;
}

async function createShare(headers: Record<string, string>, quota = 0) {
  return SELF.fetch("https://amber.test/v1/shares", {
    method: "POST",
    headers,
    body: JSON.stringify({
      account_uid: accountUID,
      credential: {
        token: upstreamToken,
        account_type: "api_key",
        upstream_url: "https://api.openai.com/v1",
      },
      quota_requests: quota,
      consent: true,
    }),
  });
}

describe("cloud sharing", () => {
  it("rejects OAuth relay before consuming quota or contacting the upstream", async () => {
    const headers = await ownerSession();
    const created = await SELF.fetch("https://amber.test/v1/shares", {
      method: "POST",
      headers,
      body: JSON.stringify({
        account_uid: accountUID,
        credential: {
          token: upstreamToken,
          account_type: "oauth",
          upstream_url: "https://chatgpt.com/backend-api/codex/responses",
          chatgpt_account_id: "acct-owner",
        },
        quota_requests: 10,
        consent: true,
      }),
    });
    const creation = await created.json<{ guest_key: string; share: { id: number } }>();
    const upstream = vi.fn(async () => new Response("unexpected", { status: 200 }));
    vi.stubGlobal("fetch", upstream);

    const response = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST",
      headers: { Authorization: `Bearer ${creation.guest_key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.4", input: "hello" }),
    });
    expect(response.status).toBe(409);
    await expect(response.json()).resolves.toMatchObject({ error: { code: "oauth_device_relay_required" } });
    expect(upstream).not.toHaveBeenCalled();
    const grant = await env.DB.prepare("SELECT used_requests FROM share_grants WHERE id=?")
      .bind(creation.share.id).first<{ used_requests: number }>();
    expect(grant?.used_requests).toBe(0);
    vi.unstubAllGlobals();
  });

  it("converts upstream HTML errors into safe JSON", async () => {
    const headers = await ownerSession();
    const creation = await (await createShare(headers)).json<{ guest_key: string }>();
    vi.stubGlobal("fetch", vi.fn(async () => new Response("<html><body>blocked</body></html>", {
      status: 403,
      headers: { "Content-Type": "text/html; charset=UTF-8" },
    })));

    const response = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST",
      headers: { Authorization: `Bearer ${creation.guest_key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.4", input: "hello" }),
    });
    expect(response.status).toBe(403);
    expect(response.headers.get("content-type")).toContain("application/json");
    const text = await response.text();
    expect(text).toContain("share_upstream_rejected");
    expect(text).not.toContain("<html>");
    vi.unstubAllGlobals();
  });

  it("encrypts the upstream token, streams responses, enforces quota, and records metadata-only usage", async () => {
    const headers = await ownerSession();
    const created = await createShare(headers, 1);
    expect(created.status).toBe(201);
    const creationText = await created.text();
    expect(creationText).not.toContain(upstreamToken);
    const creation = JSON.parse(creationText) as { guest_key: string; share: { id: number } };
    expect(creation.guest_key).toMatch(/^sk-share-/);
    const stored = await env.DB.prepare("SELECT token_cipher,guest_key_hash FROM share_grants WHERE id=?")
      .bind(creation.share.id).first<{ token_cipher: string; guest_key_hash: string }>();
    expect(stored?.token_cipher).toMatch(/^v1\./);
    expect(stored?.token_cipher).not.toContain(upstreamToken);
    expect(stored?.guest_key_hash).not.toBe(creation.guest_key);

    const upstream = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe("https://api.openai.com/v1/responses");
      expect(new Headers(init?.headers).get("authorization")).toBe(`Bearer ${upstreamToken}`);
      return new Response("data: {\"type\":\"response.completed\"}\n\n", {
        status: 200, headers: { "content-type": "text/event-stream" },
      });
    });
    vi.stubGlobal("fetch", upstream);
    const streamed = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST",
      headers: { Authorization: `Bearer ${creation.guest_key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.6", input: "hello", stream: true }),
    });
    expect(streamed.status).toBe(200);
    const streamedText = await streamed.text();
    expect(streamedText).toContain("response.completed");
    expect(streamedText).not.toContain(upstreamToken);
    expect(upstream).toHaveBeenCalledTimes(1);

    const exhausted = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST",
      headers: { Authorization: `Bearer ${creation.guest_key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.6", input: "again" }),
    });
    expect(exhausted.status).toBe(429);
    const usage = await env.DB.prepare("SELECT model,status,latency_ms FROM share_usage_log WHERE grant_id=?")
      .bind(creation.share.id).all<{ model: string; status: number; latency_ms: number }>();
    expect(usage.results).toHaveLength(1);
    expect(usage.results[0]).toMatchObject({ model: "gpt-5.6", status: 200 });
    expect(JSON.stringify(usage.results)).not.toContain("hello");
    expect(JSON.stringify(usage.results)).not.toContain(upstreamToken);
    vi.unstubAllGlobals();
  });

  it("resolves API-key endpoints and passes a delayed stream through without buffering", async () => {
    const headers = await ownerSession();
    const creation = await (await createShare(headers)).json<{ guest_key: string }>();
    const encoder = new TextEncoder();
    const upstreamURLs: string[] = [];
    const upstream = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      upstreamURLs.push(url);
      if (url.endsWith("/chat/completions")) {
        return new Response(JSON.stringify({ choices: [] }), {
          status: 200, headers: { "content-type": "application/json" },
        });
      }
      return new Response(new ReadableStream({
        start(controller) {
          controller.enqueue(encoder.encode("data: first\n\n"));
          setTimeout(() => {
            controller.enqueue(encoder.encode("data: second\n\n"));
            controller.close();
          }, streamHoldMs);
        },
      }), {
        status: 200,
        headers: {
          "content-type": "text/event-stream",
          authorization: `Bearer ${upstreamToken}`,
        },
      });
    });
    vi.stubGlobal("fetch", upstream);

    const started = Date.now();
    const streamed = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST",
      headers: { Authorization: `Bearer ${creation.guest_key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.6", input: "hold", stream: true }),
    });
    expect(streamed.status).toBe(200);
    expect(streamed.headers.get("authorization")).toBeNull();
    const reader = streamed.body!.getReader();
    const first = await reader.read();
    expect(new TextDecoder().decode(first.value)).toContain("first");
    expect(Date.now() - started).toBeLessThan(5_000);
    const second = await reader.read();
    expect(new TextDecoder().decode(second.value)).toContain("second");
    expect(Date.now() - started).toBeGreaterThanOrEqual(Math.max(0, streamHoldMs - 250));

    const chat = await SELF.fetch("https://amber.test/v1/chat/completions", {
      method: "POST",
      headers: { Authorization: `Bearer ${creation.guest_key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.6", messages: [] }),
    });
    expect(chat.status).toBe(200);
    expect(upstreamURLs).toEqual([
      "https://api.openai.com/v1/responses",
      "https://api.openai.com/v1/chat/completions",
    ]);
    let usageCount = 0;
    for (let attempt = 0; attempt < 50 && usageCount < 2; attempt += 1) {
      const usage = await env.DB.prepare("SELECT COUNT(*) AS count FROM share_usage_log").first<{ count: number }>();
      usageCount = usage?.count ?? 0;
      if (usageCount < 2) await new Promise((resolve) => setTimeout(resolve, 10));
    }
    expect(usageCount).toBe(2);
    vi.unstubAllGlobals();
  }, streamHoldMs + 15_000);

  it("rejects revoked and expired keys and cascades account deletion into revocation", async () => {
    const headers = await ownerSession();
    const first = await (await createShare(headers)).json<{ guest_key: string; share: { id: number } }>();
    const revoked = await SELF.fetch(`https://amber.test/v1/shares/${first.share.id}`, {
      method: "PATCH", headers, body: JSON.stringify({ revoked: true }),
    });
    expect(revoked.status).toBe(200);
    const revokedRequest = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST", headers: { Authorization: `Bearer ${first.guest_key}` }, body: "{}",
    });
    expect(revokedRequest.status).toBe(401);

    const second = await (await createShare(headers)).json<{ guest_key: string; share: { id: number } }>();
    await env.DB.prepare("UPDATE share_grants SET expires_at=? WHERE id=?")
      .bind(new Date(Date.now() - 1000).toISOString(), second.share.id).run();
    const expiredRequest = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST", headers: { Authorization: `Bearer ${second.guest_key}` }, body: "{}",
    });
    expect(expiredRequest.status).toBe(403);

    const third = await (await createShare(headers)).json<{ guest_key: string; share: { id: number } }>();
    const ciphertext = `v1.${bytesToBase64URL(new Uint8Array(80).fill(46))}`;
    const deleted = await SELF.fetch("https://amber.test/v1/vault/batch", {
      method: "PUT", headers,
      body: JSON.stringify({ items: [{ kind: "account", client_uid: accountUID, ciphertext, version: 1, deleted: true }] }),
    });
    expect(deleted.status).toBe(200);
    const row = await env.DB.prepare("SELECT revoked FROM share_grants WHERE id=?").bind(third.share.id).first<{ revoked: number }>();
    expect(row?.revoked).toBe(1);
    const restoreDeleted = await SELF.fetch(`https://amber.test/v1/shares/${third.share.id}`, {
      method: "PATCH", headers, body: JSON.stringify({ revoked: false }),
    });
    expect(restoreDeleted.status).toBe(409);
    const stillRevoked = await env.DB.prepare("SELECT revoked FROM share_grants WHERE id=?").bind(third.share.id).first<{ revoked: number }>();
    expect(stillRevoked?.revoked).toBe(1);
  });
});
