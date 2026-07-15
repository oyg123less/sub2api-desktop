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

  it("verifies email, logs in, rotates refresh tokens, and rejects refresh replay", async () => {
    await registerAndVerify();
    const parameters = await jsonRequest("/v1/auth/parameters", { email });
    expect(parameters.status).toBe(200);
    expect(await parameters.json()).toMatchObject({ salt_kdf: saltKDF, salt_auth: saltAuth, wrapped_vault_key: wrappedVaultKey });

    const response = await login();
    expect(response.status).toBe(200);
    const session = await response.json<{ access_token: string; refresh_token: string; user: { email: string } }>();
    expect(session.user.email).toBe(email);
    expect(session.access_token.split(".")).toHaveLength(3);

    const refresh = await jsonRequest("/v1/auth/refresh", { refresh_token: session.refresh_token });
    expect(refresh.status).toBe(200);
    const replay = await jsonRequest("/v1/auth/refresh", { refresh_token: session.refresh_token });
    expect(replay.status).toBe(401);
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
    expect(fake.wrapped_vault_key).toMatch(/^v1\./);
    expect(fake.wrapped_vault_key?.length).toBe(wrappedVaultKey.length);
  });
});

describe("encrypted vault", () => {
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
