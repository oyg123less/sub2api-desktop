import { env, SELF } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import { bytesToBase64URL, randomToken, sha256 } from "../src/security";

const accountUID = "018f1f46-7a19-7cc2-88cb-f577e51d5100";

function b64(fill: number, length: number) {
  return bytesToBase64URL(new Uint8Array(length).fill(fill));
}

async function createUser(email: string, fill: number) {
  const authHash = b64(fill, 32);
  const auth = {
    email, turnstile_token: "test-pass", auth_hash: authHash,
    salt_kdf: b64(fill + 1, 16), salt_auth: b64(fill + 2, 16), wrapped_vault_key: `v1.${b64(fill + 3, 60)}`,
  };
  expect((await SELF.fetch("https://amber.test/v1/auth/register", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(auth),
  })).status).toBe(202);
  const code = await env.SESSIONS.get(`test-mail:${await sha256(email)}`);
  await SELF.fetch("https://amber.test/v1/auth/verify-email", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, code }),
  });
  const login = await SELF.fetch("https://amber.test/v1/auth/login", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, auth_hash: authHash }),
  });
  const session = await login.json<{ access_token: string }>();
  const headers = { Authorization: `Bearer ${session.access_token}`, "Content-Type": "application/json" };
  expect((await SELF.fetch("https://amber.test/v1/profile", {
    method: "PUT", headers, body: JSON.stringify({
      display_name: email.split("@")[0], encryption_public_key: b64(fill + 4, 32),
      encryption_private_cipher: `v1.${b64(fill + 5, 80)}`,
    }),
  })).status).toBe(200);
  return { headers };
}

async function keyMaterial(fill: number, guestKey: string) {
  return {
    key_prefix: guestKey.slice(0, 18),
    guest_key_hash: await sha256(guestKey),
    key_envelope: JSON.stringify({
      version: 1, algorithm: "X25519-HKDF-SHA256-AES-256-GCM",
      ephemeral_public_key: b64(fill, 32), salt: b64(fill + 1, 16), nonce: b64(fill + 2, 12), ciphertext: b64(fill + 3, 48),
    }),
    envelope_context: `ctx_${randomToken(24)}`,
    recipient_key_version: 1,
  };
}

async function configureHost(owner: Awaited<ReturnType<typeof createUser>>) {
  await SELF.fetch("https://amber.test/v1/vault/batch", {
    method: "PUT", headers: { ...owner.headers, "Idempotency-Key": "connect-account-upload-001" },
    body: JSON.stringify({ items: [{ kind: "account", client_uid: accountUID, ciphertext: `v1.${b64(91, 80)}`, version: 0, deleted: false }] }),
  });
  const configured = await SELF.fetch("https://amber.test/v1/connect/host/accounts", {
    method: "PUT", headers: owner.headers, body: JSON.stringify({ accounts: [{
      account_uid: accountUID, account_type: "oauth", relay_mode: "owner_device",
    }] }),
  });
  expect(configured.status).toBe(200);
  const start = await SELF.fetch("https://amber.test/v1/connect/host/start", {
    method: "POST", headers: { ...owner.headers, "Idempotency-Key": "connect-window-start-001" },
    body: JSON.stringify({ password: "AB3D5F", max_claims: 2, duration_minutes: 30 }),
  });
  expect(start.status).toBe(200);
  const host = await start.json<{ host: { endpoint: { connection_code: string } } }>();
  return host.host.endpoint.connection_code;
}

describe("connection-code sharing", () => {
  it("allows one-to-many claims without friendship and isolates every Guest Key", async () => {
    const owner = await createUser("connect-owner@example.test", 11);
    const first = await createUser("connect-first@example.test", 31);
    const second = await createUser("connect-second@example.test", 51);
    const third = await createUser("connect-third@example.test", 71);
    const connectionCode = await configureHost(owner);

    const claim = async (user: typeof first, fill: number, key: string, idempotency: string) => SELF.fetch("https://amber.test/v1/connect/claim", {
      method: "POST", headers: { ...user.headers, "Idempotency-Key": idempotency },
      body: JSON.stringify({ connection_code: connectionCode, password: "AB3D5F", key_material: await keyMaterial(fill, key) }),
    });
    const firstKey = `sk-amber-${randomToken(32)}`;
    const secondKey = `sk-amber-${randomToken(32)}`;
    const firstResponse = await claim(first, 101, firstKey, "connect-claim-first-001");
    expect(firstResponse.status).toBe(201);
    expect((await firstResponse.json<{ share: { status: string } }>()).share.status).toBe("active");
    const offline = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST",
      headers: { Authorization: `Bearer ${firstKey}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.6", input: "health check" }),
    });
    expect(offline.status).toBe(503);
    await expect(offline.json()).resolves.toMatchObject({ error: { code: "owner_device_offline" } });
    const usage = await env.DB.prepare("SELECT error_code FROM share_usage_log_v2 ORDER BY id DESC LIMIT 1").first<{ error_code: string }>();
    expect(usage?.error_code).toBe("owner_device_offline");
    const secondResponse = await claim(second, 121, secondKey, "connect-claim-second-001");
    expect(secondResponse.status).toBe(201);

    const ownerEventsResponse = await SELF.fetch("https://amber.test/v1/events?cursor=0", { headers: owner.headers });
    expect(ownerEventsResponse.status).toBe(200);
    const ownerEvents = await ownerEventsResponse.json<{ events: Array<{ event_type: string }>; cursor: number; has_more: boolean }>();
    expect(ownerEvents.events.filter((event) => event.event_type === "connect.recipient_joined")).toHaveLength(2);
    expect(ownerEvents.cursor).toBeGreaterThan(0);
    expect(ownerEvents.has_more).toBe(false);

    const firstEvents = await (await SELF.fetch("https://amber.test/v1/events?cursor=0", { headers: first.headers }))
      .json<{ events: Array<{ event_type: string }>; cursor: number }>();
    expect(firstEvents.events.map((event) => event.event_type)).toContain("connect.access_claimed");
    expect(firstEvents.events.map((event) => event.event_type)).not.toContain("connect.recipient_joined");

    const noChanges = await (await SELF.fetch(`https://amber.test/v1/events?cursor=${ownerEvents.cursor}`, { headers: owner.headers }))
      .json<{ events: unknown[]; cursor: number }>();
    expect(noChanges).toEqual({ events: [], cursor: ownerEvents.cursor, has_more: false });

    const keys = await env.DB.prepare("SELECT guest_key_hash,status FROM share_access_keys ORDER BY id").all<{ guest_key_hash: string; status: string }>();
    expect(keys.results).toHaveLength(2);
    expect(new Set(keys.results.map((row) => row.guest_key_hash)).size).toBe(2);
    expect(keys.results.every((row) => row.status === "active")).toBe(true);
    const window = await env.DB.prepare("SELECT status,claimed_count,max_claims FROM share_connect_windows").first<{ status: string; claimed_count: number; max_claims: number }>();
    expect(window).toMatchObject({ status: "exhausted", claimed_count: 2, max_claims: 2 });

    const exhausted = await claim(third, 141, `sk-amber-${randomToken(32)}`, "connect-claim-third-001");
    expect(exhausted.status).toBe(409);
    await expect(exhausted.json()).resolves.toMatchObject({ error: { code: "connect_window_unavailable" } });

    expect((await SELF.fetch("https://amber.test/v1/connect/host/pause", { method: "POST", headers: owner.headers })).status).toBe(200);
    const pausedEvents = await (await SELF.fetch(`https://amber.test/v1/events?cursor=${firstEvents.cursor}`, { headers: first.headers }))
      .json<{ events: Array<{ event_type: string }> }>();
    expect(pausedEvents.events.map((event) => event.event_type)).toContain("connect.host_paused");
  });

  it("rejects bad passwords and replays a successful claim idempotently", async () => {
    const owner = await createUser("connect-owner2@example.test", 15);
    const recipient = await createUser("connect-recipient2@example.test", 35);
    const connectionCode = await configureHost(owner);
    const guestKey = `sk-amber-${randomToken(32)}`;
    const body = { connection_code: connectionCode, password: "AB3D5F", key_material: await keyMaterial(161, guestKey) };
    const bad = await SELF.fetch("https://amber.test/v1/connect/claim", {
      method: "POST", headers: { ...recipient.headers, "Idempotency-Key": "connect-bad-password-01" },
      body: JSON.stringify({ ...body, password: "ZZ9ZZ9" }),
    });
    expect(bad.status).toBe(404);
    const headers = { ...recipient.headers, "Idempotency-Key": "connect-idempotent-001" };
    expect((await SELF.fetch("https://amber.test/v1/connect/claim", { method: "POST", headers, body: JSON.stringify(body) })).status).toBe(201);
    const replay = await SELF.fetch("https://amber.test/v1/connect/claim", { method: "POST", headers, body: JSON.stringify(body) });
    expect(replay.status).toBe(201);
    expect(replay.headers.get("Idempotency-Replayed")).toBe("true");
    expect((await env.DB.prepare("SELECT COUNT(*) AS value FROM share_connect_claims").first<{ value: number }>())?.value).toBe(1);
  });

  it("returns an actionable 426 response when minimum client enforcement is enabled", async () => {
    const user = await createUser("old-client@example.test", 19);
    await env.DB.prepare("UPDATE platform_settings SET value='true' WHERE key='enforce_client_version'").run();
    const blocked = await SELF.fetch("https://amber.test/v1/profile", { headers: user.headers });
    expect(blocked.status).toBe(426);
    expect(blocked.headers.get("X-Amber-Minimum-Version")).toBe("0.4.2");
    await expect(blocked.json()).resolves.toMatchObject({
      error: { code: "client_upgrade_required", minimum_version: "0.4.2", update_url: expect.stringContaining("releases") },
    });
    const current = await SELF.fetch("https://amber.test/v1/profile", {
      headers: { ...user.headers, "X-Amber-Client-Version": "0.4.2" },
    });
    expect(current.status).toBe(200);
  });

  it("lets an unversioned owner relay reach relay authentication", async () => {
    await env.DB.prepare("UPDATE platform_settings SET value='true' WHERE key='enforce_client_version'").run();
    const response = await SELF.fetch("https://amber.test/v1/relay/connect?device_id=dev_missing&protocol=1", {
      headers: { Upgrade: "websocket" },
    });
    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toMatchObject({ error: { code: "authentication_required" } });
  });
});
