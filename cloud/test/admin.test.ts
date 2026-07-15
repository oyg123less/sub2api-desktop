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
  return login.json<{ access_token: string }>();
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
});
