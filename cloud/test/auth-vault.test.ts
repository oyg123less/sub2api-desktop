import { env, SELF } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import { bytesToBase64URL, sha256 } from "../src/security";

const email = "amber-test@example.test";
const authHash = bytesToBase64URL(new Uint8Array(32).fill(7));
const saltKDF = bytesToBase64URL(new Uint8Array(16).fill(8));
const saltAuth = bytesToBase64URL(new Uint8Array(16).fill(9));
const wrappedVaultKey = `v1.${bytesToBase64URL(new Uint8Array(60).fill(10))}`;

function jsonRequest(path: string, body: unknown, headers: Record<string, string> = {}) {
  return SELF.fetch(`https://amber.test${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...headers },
    body: JSON.stringify(body),
  });
}

async function registerAndVerify(targetEmail = email) {
  const registration = await jsonRequest("/v1/auth/register", {
    email: targetEmail,
    turnstile_token: "test-pass",
    auth_hash: authHash,
    salt_kdf: saltKDF,
    salt_auth: saltAuth,
    wrapped_vault_key: wrappedVaultKey,
  });
  expect(registration.status).toBe(202);
  const code = await env.SESSIONS.get(`test-mail:${await sha256(targetEmail)}`);
  expect(code).toMatch(/^\d{6}$/);
  const verification = await jsonRequest("/v1/auth/verify-email", { email: targetEmail, code });
  expect(verification.status).toBe(200);
}

async function login(targetEmail = email, hash = authHash) {
  return jsonRequest("/v1/auth/login", { email: targetEmail, auth_hash: hash });
}

describe("authentication", () => {
  it("requires Turnstile and stores only a server-side authentication verifier", async () => {
    const missing = await jsonRequest("/v1/auth/register", {
      email,
      auth_hash: authHash,
      salt_kdf: saltKDF,
      salt_auth: saltAuth,
      wrapped_vault_key: wrappedVaultKey,
    });
    expect(missing.status).toBe(400);
    expect((await missing.json<{ error: { code: string } }>()).error.code).toBe("turnstile_required");

    await registerAndVerify();
    const row = await env.DB.prepare("SELECT auth_hash,wrapped_vault_key FROM users WHERE email=?").bind(email).first<{
      auth_hash: string;
      wrapped_vault_key: string;
    }>();
    expect(row?.auth_hash).not.toBe(authHash);
    expect(row?.wrapped_vault_key).toBe(wrappedVaultKey);
  });

  it("verifies email and makes refresh rotation idempotent during the response-loss grace period", async () => {
    await registerAndVerify();
    const parameters = await jsonRequest("/v1/auth/parameters", { email });
    expect(parameters.status).toBe(200);
    const knownParameters = await parameters.json<Record<string, string>>();
    expect(knownParameters).toEqual({ salt_kdf: saltKDF, salt_auth: saltAuth });
    expect(knownParameters).not.toHaveProperty("wrapped_vault_key");
    const unknown = await jsonRequest("/v1/auth/parameters", { email: "missing@example.test" });
    expect(unknown.status).toBe(200);
    const fakeParameters = await unknown.json<Record<string, string>>();
    expect(fakeParameters).not.toHaveProperty("wrapped_vault_key");
    expect(Object.keys(fakeParameters).sort()).toEqual(Object.keys(knownParameters).sort());

    const response = await login();
    expect(response.status).toBe(200);
    const session = await response.json<{ access_token: string; refresh_token: string; user: { email: string } }>();
    expect(session.user.email).toBe(email);
    expect(session.access_token.split(".")).toHaveLength(3);

    const refresh = await jsonRequest("/v1/auth/refresh", { refresh_token: session.refresh_token });
    expect(refresh.status).toBe(200);
    const refreshed = await refresh.json<{ refresh_token: string }>();
    const replay = await jsonRequest("/v1/auth/refresh", { refresh_token: session.refresh_token });
    expect(replay.status).toBe(200);
    expect((await replay.json<{ refresh_token: string }>()).refresh_token).toBe(refreshed.refresh_token);

    await env.SESSIONS.delete(`refresh:${await sha256(session.refresh_token)}`);
    const expiredReplay = await jsonRequest("/v1/auth/refresh", { refresh_token: session.refresh_token });
    expect(expiredReplay.status).toBe(401);
    const nextRefresh = await jsonRequest("/v1/auth/refresh", { refresh_token: refreshed.refresh_token });
    expect(nextRefresh.status).toBe(200);
  });

  it("locks repeated failed logins without revealing whether the email exists", async () => {
    await registerAndVerify();
    const wrong = bytesToBase64URL(new Uint8Array(32).fill(99));
    for (let attempt = 0; attempt < 10; attempt += 1) {
      const response = await login(email, wrong);
      expect(response.status).toBe(401);
    }
    const locked = await login(email, wrong);
    expect(locked.status).toBe(429);
    const unknown = await jsonRequest("/v1/auth/parameters", { email: "missing@example.test" });
    expect(unknown.status).toBe(200);
    const fake = await unknown.json<Record<string, string>>();
    expect(fake.salt_kdf).not.toBe(saltKDF);
    expect(Object.keys(fake).sort()).toEqual(["salt_auth", "salt_kdf"]);
  });

  it("resends verification without enumeration and limits each email to three attempts per hour", async () => {
    const pendingEmail = "pending-resend@example.test";
    const registration = await jsonRequest("/v1/auth/register", {
      email: pendingEmail,
      turnstile_token: "test-pass",
      auth_hash: authHash,
      salt_kdf: saltKDF,
      salt_auth: saltAuth,
      wrapped_vault_key: wrappedVaultKey,
    });
    expect(registration.status).toBe(202);
    const verificationKey = `verify:${await sha256(pendingEmail)}`;
    const original = await env.SESSIONS.get<{ attempts: number }>(verificationKey, "json");
    expect(original?.attempts).toBe(0);
    const wrong = await jsonRequest("/v1/auth/verify-email", { email: pendingEmail, code: "000000" });
    expect(wrong.status).toBe(400);
    expect((await env.SESSIONS.get<{ attempts: number }>(verificationKey, "json"))?.attempts).toBe(1);

    for (let attempt = 0; attempt < 3; attempt += 1) {
      const resent = await jsonRequest("/v1/auth/resend-verification", { email: pendingEmail });
      expect(resent.status).toBe(202);
      expect(await resent.json()).toEqual({ ok: true });
    }
    expect((await env.SESSIONS.get<{ attempts: number }>(verificationKey, "json"))?.attempts).toBe(0);
    const limited = await jsonRequest("/v1/auth/resend-verification", { email: pendingEmail });
    expect(limited.status).toBe(429);

    const latestCode = await env.SESSIONS.get(`test-mail:${await sha256(pendingEmail)}`);
    expect((await jsonRequest("/v1/auth/verify-email", { email: pendingEmail, code: latestCode })).status).toBe(200);
    await registerAndVerify("verified-resend@example.test");
    const verified = await jsonRequest("/v1/auth/resend-verification", { email: "verified-resend@example.test" });
    const missing = await jsonRequest("/v1/auth/resend-verification", { email: "missing-resend@example.test" });
    expect(verified.status).toBe(202);
    expect(missing.status).toBe(202);
    expect(await verified.json()).toEqual(await missing.json());
  });

  it("reveals a ban only after the supplied authentication hash is valid", async () => {
    await registerAndVerify();
    await env.DB.prepare("UPDATE users SET banned=1 WHERE email=?").bind(email).run();
    const wrongHash = bytesToBase64URL(new Uint8Array(32).fill(88));
    const wrong = await login(email, wrongHash);
    expect(wrong.status).toBe(401);
    expect(await wrong.json()).toMatchObject({ error: { code: "invalid_credentials" } });
    const valid = await login(email, authHash);
    expect(valid.status).toBe(403);
    expect(await valid.json()).toMatchObject({ error: { code: "account_disabled" } });
  });
});

describe("encrypted vault", () => {
  it("replays an idempotent batch without incrementing the vault version twice", async () => {
    await registerAndVerify();
    const session = await (await login()).json<{ access_token: string }>();
    const clientUID = "018f1f46-7a19-7cc2-88cb-f577e51d3555";
    const ciphertext = `v1.${bytesToBase64URL(new Uint8Array(80).fill(31))}`;
    const headers = {
      Authorization: `Bearer ${session.access_token}`,
      "Content-Type": "application/json",
      "Idempotency-Key": "amber-sync-replay-test-0001",
    };
    const body = JSON.stringify({ items: [{ kind: "account", client_uid: clientUID, ciphertext, version: 0, deleted: false }] });
    const first = await SELF.fetch("https://amber.test/v1/vault/batch", { method: "PUT", headers, body });
    expect(first.status).toBe(200);
    const firstBody = await first.text();
    const replay = await SELF.fetch("https://amber.test/v1/vault/batch", { method: "PUT", headers, body });
    expect(replay.status).toBe(200);
    expect(replay.headers.get("Idempotency-Replayed")).toBe("true");
    expect(await replay.text()).toBe(firstBody);

    const row = await env.DB.prepare("SELECT version FROM vault_items WHERE user_id=1 AND client_uid=?")
      .bind(clientUID).first<{ version: number }>();
    expect(row?.version).toBe(1);
    const reused = await SELF.fetch("https://amber.test/v1/vault/batch", {
      method: "PUT",
      headers,
      body: JSON.stringify({ items: [{ kind: "account", client_uid: clientUID, ciphertext, version: 1, deleted: true }] }),
    });
    expect(reused.status).toBe(409);
    await expect(reused.json()).resolves.toMatchObject({ error: { code: "idempotency_key_reused" } });
  });

  it("allows only one concurrent writer to claim the same vault base version", async () => {
    await registerAndVerify();
    const session = await (await login()).json<{ access_token: string }>();
    const clientUID = "018f1f46-7a19-7cc2-88cb-f577e51d3666";
    const makeRequest = (key: string, fill: number) => SELF.fetch("https://amber.test/v1/vault/batch", {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${session.access_token}`,
        "Content-Type": "application/json",
        "Idempotency-Key": key,
      },
      body: JSON.stringify({ items: [{
        kind: "account",
        client_uid: clientUID,
        ciphertext: `v1.${bytesToBase64URL(new Uint8Array(80).fill(fill))}`,
        version: 0,
        deleted: false,
      }] }),
    });
    const responses = await Promise.all([
      makeRequest("amber-sync-concurrent-a-0001", 61),
      makeRequest("amber-sync-concurrent-b-0001", 62),
    ]);
    expect(responses.map((response) => response.status).sort()).toEqual([200, 409]);
    const row = await env.DB.prepare("SELECT version FROM vault_items WHERE user_id=1 AND client_uid=?")
      .bind(clientUID).first<{ version: number }>();
    expect(row?.version).toBe(1);
  });

  it("upserts opaque ciphertext, returns tombstones, and detects optimistic-lock conflicts", async () => {
    await registerAndVerify();
    const session = await (await login()).json<{ access_token: string }>();
    const headers = { Authorization: `Bearer ${session.access_token}`, "Content-Type": "application/json" };
    const clientUID = "018f1f46-7a19-7cc2-88cb-f577e51d3521";
    const ciphertext = `v1.${bytesToBase64URL(new Uint8Array(80).fill(42))}`;
    const put = await SELF.fetch("https://amber.test/v1/vault/batch", {
      method: "PUT",
      headers,
      body: JSON.stringify({ items: [{ kind: "account", client_uid: clientUID, ciphertext, version: 0, deleted: false }] }),
    });
    expect(put.status).toBe(200);
    expect(JSON.stringify(await put.json())).not.toContain("upstream-secret");

    const pull = await SELF.fetch("https://amber.test/v1/vault", { headers });
    expect(pull.status).toBe(200);
    expect(await pull.json()).toMatchObject({ items: [{ client_uid: clientUID, ciphertext, version: 1, deleted: false }] });

    const conflict = await SELF.fetch("https://amber.test/v1/vault/batch", {
      method: "PUT",
      headers,
      body: JSON.stringify({ items: [{ kind: "account", client_uid: clientUID, ciphertext, version: 0, deleted: true }] }),
    });
    expect(conflict.status).toBe(409);
    expect(await conflict.json()).toMatchObject({ error: { code: "vault_conflict" }, conflicts: [{ version: 1 }] });
  });

  it("paginates equal-timestamp vault items with a composite cursor", async () => {
    await registerAndVerify();
    const session = await (await login()).json<{ access_token: string }>();
    const headers = { Authorization: `Bearer ${session.access_token}`, "Content-Type": "application/json" };
    const user = await env.DB.prepare("SELECT id FROM users WHERE email=?").bind(email).first<{ id: number }>();
    const timestamp = "2026-07-17T00:00:00.000Z";
    const ciphertext = `v1.${bytesToBase64URL(new Uint8Array(80).fill(51))}`;
    await env.DB.batch([
      env.DB.prepare(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<600)
        INSERT INTO vault_items(user_id,kind,client_uid,ciphertext,version,deleted,updated_at)
        SELECT ?,'account',printf('018f1f46-7a19-7cc2-88cb-%012x',n),?,1,0,? FROM seq`).bind(user?.id, ciphertext, timestamp),
      env.DB.prepare(`WITH RECURSIVE seq(n) AS (SELECT 601 UNION ALL SELECT n+1 FROM seq WHERE n<1002)
        INSERT INTO vault_items(user_id,kind,client_uid,ciphertext,version,deleted,updated_at)
        SELECT ?,'account',printf('018f1f46-7a19-7cc2-88cb-%012x',n),?,1,0,? FROM seq`).bind(user?.id, ciphertext, timestamp),
    ]);

    const first = await SELF.fetch("https://amber.test/v1/vault", { headers });
    const firstPage = await first.json<{ items: Array<{ client_uid: string }>; cursor: string }>();
    expect(firstPage.items).toHaveLength(1000);
    expect(firstPage.cursor).toMatch(/^2026-07-17T00:00:00\.000Z\|\d+$/);
    const second = await SELF.fetch(`https://amber.test/v1/vault?since=${encodeURIComponent(firstPage.cursor)}`, { headers });
    const secondPage = await second.json<{ items: Array<{ client_uid: string }>; cursor: string }>();
    expect(secondPage.items).toHaveLength(2);
    expect(new Set([...firstPage.items, ...secondPage.items].map((item) => item.client_uid)).size).toBe(1002);

    const legacy = await SELF.fetch(`https://amber.test/v1/vault?since=${encodeURIComponent(timestamp)}`, { headers });
    expect(await legacy.json()).toMatchObject({ items: [], cursor: timestamp });
  });

  it("invalidates every old session after changing the master password", async () => {
    await registerAndVerify();
    const session = await (await login()).json<{ access_token: string }>();
    const newHash = bytesToBase64URL(new Uint8Array(32).fill(11));
    const changed = await SELF.fetch("https://amber.test/v1/auth/master-password", {
      method: "PUT",
      headers: { Authorization: `Bearer ${session.access_token}`, "Content-Type": "application/json" },
      body: JSON.stringify({
        current_auth_hash: authHash,
        auth_hash: newHash,
        salt_kdf: bytesToBase64URL(new Uint8Array(16).fill(12)),
        salt_auth: bytesToBase64URL(new Uint8Array(16).fill(13)),
        wrapped_vault_key: `v1.${bytesToBase64URL(new Uint8Array(60).fill(14))}`,
      }),
    });
    expect(changed.status).toBe(200);
    const oldSession = await SELF.fetch("https://amber.test/v1/vault", { headers: { Authorization: `Bearer ${session.access_token}` } });
    expect(oldSession.status).toBe(401);
    expect((await login(email, authHash)).status).toBe(401);
    expect((await login(email, newHash)).status).toBe(200);
  });
});
