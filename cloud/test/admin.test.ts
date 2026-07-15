import { SELF, env } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import { bytesToBase64URL, sha256 } from "../src/security";

const authHash = bytesToBase64URL(new Uint8Array(32).fill(21));

async function createUser(email: string) {
  const base = {
    email,
    turnstile_token: "test-pass",
    auth_hash: authHash,
    salt_kdf: bytesToBase64URL(new Uint8Array(16).fill(22)),
    salt_auth: bytesToBase64URL(new Uint8Array(16).fill(23)),
    wrapped_vault_key: `v1.${bytesToBase64URL(new Uint8Array(60).fill(24))}`,
  };
  await SELF.fetch("https://amber.test/v1/auth/register", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(base) });
  const code = await env.SESSIONS.get(`test-mail:${await sha256(email)}`);
  await SELF.fetch("https://amber.test/v1/auth/verify-email", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, code }) });
  const login = await SELF.fetch("https://amber.test/v1/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, auth_hash: authHash }) });
  return login.json<{ access_token: string; refresh_token: string }>();
}

async function loginUser(email: string) {
  const response = await SELF.fetch("https://amber.test/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, auth_hash: authHash }),
  });
  return response.json<{ access_token: string; refresh_token: string }>();
}

describe("administrator boundaries", () => {
  it("requires both the admin role and independent second factor", async () => {
    const email = "admin@example.test";
    const session = await createUser(email);
    const userRow = await env.DB.prepare("SELECT id FROM users WHERE email=?").bind(email).first<{ id: number }>();
    await env.DB.prepare("UPDATE users SET role='admin',session_version=session_version+1 WHERE id=?").bind(userRow?.id).run();
    const stale = await SELF.fetch("https://amber.test/v1/admin/users", { headers: { Authorization: `Bearer ${session.access_token}` } });
    expect(stale.status).toBe(401);

    const refreshed = await SELF.fetch("https://amber.test/v1/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, auth_hash: authHash }) });
    const adminSession = await refreshed.json<{ access_token: string }>();
    const missingFactor = await SELF.fetch("https://amber.test/v1/admin/users", { headers: { Authorization: `Bearer ${adminSession.access_token}` } });
    expect(missingFactor.status).toBe(403);
    const allowed = await SELF.fetch("https://amber.test/v1/admin/users", {
      headers: { Authorization: `Bearer ${adminSession.access_token}`, "X-Admin-Key": "test-admin-second-factor-at-least-32-bytes" },
    });
    expect(allowed.status).toBe(200);
    const body = JSON.stringify(await allowed.json());
    expect(body).not.toContain("auth_hash");
    expect(body).not.toContain("wrapped_vault_key");
  });

  it("revokes sessions, controls registration, and preserves audit history", async () => {
    const adminEmail = "governance-admin@example.test";
    const targetEmail = "managed-user@example.test";
    await createUser(adminEmail);
    const targetSession = await createUser(targetEmail);
    const adminRow = await env.DB.prepare("SELECT id FROM users WHERE email=?").bind(adminEmail).first<{ id: number }>();
    const targetRow = await env.DB.prepare("SELECT id FROM users WHERE email=?").bind(targetEmail).first<{ id: number }>();
    await env.DB.prepare("UPDATE users SET role='admin',session_version=session_version+1 WHERE id=?").bind(adminRow?.id).run();
    const adminSession = await loginUser(adminEmail);
    const headers = {
      Authorization: `Bearer ${adminSession.access_token}`,
      "Content-Type": "application/json",
      "X-Admin-Key": "test-admin-second-factor-at-least-32-bytes",
    };

    expect(await env.SESSIONS.get(`refresh:${await sha256(targetSession.refresh_token)}`)).not.toBeNull();
    const ban = await SELF.fetch(`https://amber.test/v1/admin/users/${targetRow?.id}`, {
      method: "PATCH", headers, body: JSON.stringify({ banned: true }),
    });
    expect(ban.status).toBe(200);
    expect(await env.SESSIONS.get(`refresh:${await sha256(targetSession.refresh_token)}`)).toBeNull();
    const staleAccess = await SELF.fetch("https://amber.test/v1/vault", {
      headers: { Authorization: `Bearer ${targetSession.access_token}` },
    });
    expect(staleAccess.status).toBe(401);
    const staleRefresh = await SELF.fetch("https://amber.test/v1/auth/refresh", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: targetSession.refresh_token }),
    });
    expect(staleRefresh.status).toBe(401);

    const disableRegistration = await SELF.fetch("https://amber.test/v1/admin/settings", {
      method: "PATCH", headers, body: JSON.stringify({ registration_enabled: false }),
    });
    expect(disableRegistration.status).toBe(200);
    const blockedRegistration = await SELF.fetch("https://amber.test/v1/auth/register", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({}),
    });
    expect(blockedRegistration.status).toBe(503);

    const audit = await SELF.fetch("https://amber.test/v1/admin/audit", { headers });
    expect(audit.status).toBe(200);
    const auditBody = JSON.stringify(await audit.json());
    expect(auditBody).toContain("user.ban");
    expect(auditBody).toContain("platform.settings.update");
    expect(auditBody).not.toContain("auth_hash");
    expect(auditBody).not.toContain("wrapped_vault_key");
  });
});
