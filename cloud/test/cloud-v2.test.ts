import { env, SELF } from "cloudflare:test";
import { describe, expect, it, vi } from "vitest";
import { bytesToBase64URL, randomToken, sha256 } from "../src/security";

const accountUID = "018f1f46-7a19-7cc2-88cb-f577e51d4999";
const secondAccountUID = "018f1f46-7a19-7cc2-88cb-f577e51d5000";

function b64(fill: number, length: number) {
  return bytesToBase64URL(new Uint8Array(length).fill(fill));
}

async function createUser(email: string, fill: number) {
  const authHash = b64(fill, 32);
  const auth = {
    email,
    turnstile_token: "test-pass",
    auth_hash: authHash,
    salt_kdf: b64(fill + 1, 16),
    salt_auth: b64(fill + 2, 16),
    wrapped_vault_key: `v1.${b64(fill + 3, 60)}`,
  };
  expect((await SELF.fetch("https://amber.test/v1/auth/register", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(auth),
  })).status).toBe(202);
  const code = await env.SESSIONS.get(`test-mail:${await sha256(email)}`);
  expect((await SELF.fetch("https://amber.test/v1/auth/verify-email", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, code }),
  })).status).toBe(200);
  const login = await SELF.fetch("https://amber.test/v1/auth/login", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, auth_hash: authHash }),
  });
  const session = await login.json<{ access_token: string }>();
  const headers = { Authorization: `Bearer ${session.access_token}`, "Content-Type": "application/json" };
  const profile = await SELF.fetch("https://amber.test/v1/profile", {
    method: "PUT", headers, body: JSON.stringify({
      display_name: email.split("@")[0],
      encryption_public_key: b64(fill + 4, 32),
      encryption_private_cipher: `v1.${b64(fill + 5, 80)}`,
    }),
  });
  expect(profile.status).toBe(200);
  return { headers, profile: (await profile.json<{ profile: { friend_code: string } }>()).profile };
}

function envelope(fill: number) {
  return JSON.stringify({
    version: 1,
    algorithm: "X25519-HKDF-SHA256-AES-256-GCM",
    ephemeral_public_key: b64(fill, 32),
    salt: b64(fill + 1, 16),
    nonce: b64(fill + 2, 12),
    ciphertext: b64(fill + 3, 48),
  });
}

async function makeFriends(owner: Awaited<ReturnType<typeof createUser>>, recipient: Awaited<ReturnType<typeof createUser>>) {
  const request = await SELF.fetch("https://amber.test/v1/friend-requests", {
    method: "POST", headers: owner.headers, body: JSON.stringify({ friend_code: recipient.profile.friend_code }),
  });
  expect(request.status).toBe(201);
  const requestID = (await request.json<{ request: { public_id: string } }>()).request.public_id;
  expect((await SELF.fetch(`https://amber.test/v1/friend-requests/${requestID}/accept`, {
    method: "POST", headers: recipient.headers,
  })).status).toBe(200);
  const friends = await (await SELF.fetch("https://amber.test/v1/friends", { headers: owner.headers }))
    .json<{ friends: Array<{ public_id: string; friend_code: string }> }>();
  const friendship = friends.friends.find((friend) => friend.friend_code === recipient.profile.friend_code);
  expect(friendship).toBeTruthy();
  return friendship!.public_id;
}

describe("Amber Cloud 2.0", () => {
  it("creates an idempotent multi-account share group and delivers an isolated key to an accepted friend", async () => {
    const owner = await createUser("owner-v4@example.test", 11);
    const recipient = await createUser("friend-v4@example.test", 31);
    const secondRecipient = await createUser("friend2-v4@example.test", 41);
    const friendshipID = await makeFriends(owner, recipient);
    const secondFriendshipID = await makeFriends(owner, secondRecipient);
    const ciphertext = `v1.${b64(71, 80)}`;
    expect((await SELF.fetch("https://amber.test/v1/vault/batch", {
      method: "PUT", headers: { ...owner.headers, "Idempotency-Key": "v4-account-upload-0001" },
      body: JSON.stringify({ items: [
        { kind: "account", client_uid: accountUID, ciphertext, version: 0, deleted: false },
        { kind: "account", client_uid: secondAccountUID, ciphertext: `v1.${b64(72, 80)}`, version: 0, deleted: false },
      ] }),
    })).status).toBe(200);

    const guestKey = `sk-amber-${randomToken(32)}`;
    const secondGuestKey = `sk-amber-${randomToken(32)}`;
    const createBody = {
      name: "Team Pool",
      description: "v0.4 integration",
      route_policy: "balanced",
      default_rpm: 30,
      default_concurrency: 2,
      default_quota_requests: 2,
      accounts: [{
        account_uid: accountUID,
        account_type: "api_key",
        relay_mode: "worker_direct",
        credential: { token: "v4-upstream-secret", account_type: "api_key", upstream_url: "https://api.openai.com/v1" },
      }, {
        account_uid: secondAccountUID,
        account_type: "oauth",
        relay_mode: "owner_device",
      }],
      recipients: [{
        friendship_id: friendshipID,
        key_material: {
          key_prefix: guestKey.slice(0, 18),
          guest_key_hash: await sha256(guestKey),
          key_envelope: envelope(51),
          envelope_context: `ctx_${randomToken(24)}`,
          recipient_key_version: 1,
        },
      }, {
        friendship_id: secondFriendshipID,
        key_material: {
          key_prefix: secondGuestKey.slice(0, 18),
          guest_key_hash: await sha256(secondGuestKey),
          key_envelope: envelope(61),
          envelope_context: `ctx_${randomToken(24)}`,
          recipient_key_version: 1,
        },
      }],
    };
    const headers = { ...owner.headers, "Idempotency-Key": "share-group-create-0001" };
    const created = await SELF.fetch("https://amber.test/v1/share-groups", {
      method: "POST", headers, body: JSON.stringify(createBody),
    });
    expect(created.status).toBe(201);
    const creation = await created.json<{ group: { public_id: string }; recipients: Array<{ public_id: string }> }>();
    expect(creation.group.public_id).toMatch(/^grp_/);
    expect(creation.recipients).toHaveLength(2);
    const replay = await SELF.fetch("https://amber.test/v1/share-groups", {
      method: "POST", headers, body: JSON.stringify(createBody),
    });
    expect(replay.status).toBe(201);
    expect(replay.headers.get("Idempotency-Replayed")).toBe("true");
    expect((await replay.json<{ group: { public_id: string } }>()).group.public_id).toBe(creation.group.public_id);
    expect((await env.DB.prepare("SELECT COUNT(*) AS value FROM share_groups").first<{ value: number }>())?.value).toBe(1);
    expect((await env.DB.prepare("SELECT COUNT(DISTINCT guest_key_hash) AS value FROM share_access_keys").first<{ value: number }>())?.value).toBe(2);

    const pending = await (await SELF.fetch("https://amber.test/v1/received-shares", { headers: recipient.headers }))
      .json<{ shares: Array<{ public_id: string; status: string; key?: { key_envelope?: string } }> }>();
    expect(pending.shares).toHaveLength(1);
    expect(pending.shares[0]).toMatchObject({ status: "pending" });
    expect(pending.shares[0]?.key).not.toHaveProperty("key_envelope");
    const receivedID = pending.shares[0]!.public_id;
    const accepted = await SELF.fetch(`https://amber.test/v1/received-shares/${receivedID}/accept`, {
      method: "POST", headers: recipient.headers,
    });
    expect(accepted.status).toBe(200);
    const acceptedBody = await accepted.json<{ share: { key: { key_envelope: string; recipient_key_version: number } } }>();
    expect(acceptedBody.share.key).toMatchObject({ key_envelope: envelope(51), recipient_key_version: 1 });
    const acceptedReplay = await SELF.fetch(`https://amber.test/v1/received-shares/${receivedID}/accept`, {
      method: "POST", headers: recipient.headers,
    });
    expect(acceptedReplay.status).toBe(200);
    await expect(acceptedReplay.json()).resolves.toMatchObject({
      share: { key: { key_envelope: envelope(51), recipient_key_version: 1, status: "active" } },
    });

    const upstream = vi.fn(async () => new Response(JSON.stringify({ id: "response-v4" }), {
      status: 200, headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", upstream);
    const gateway = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST", headers: { Authorization: `Bearer ${guestKey}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model: "gpt-5.6", input: "hello" }),
    });
    expect(gateway.status).toBe(200);
    expect(await gateway.json()).toEqual({ id: "response-v4" });
    expect(upstream).toHaveBeenCalledTimes(1);

    const paused = await SELF.fetch(`https://amber.test/v1/share-groups/${creation.group.public_id}/recipients/${creation.recipients[0]!.public_id}`, {
      method: "PATCH", headers: owner.headers, body: JSON.stringify({ status: "paused" }),
    });
    expect(paused.status).toBe(200);
    const blocked = await SELF.fetch("https://amber.test/v1/responses", {
      method: "POST", headers: { Authorization: `Bearer ${guestKey}`, "Content-Type": "application/json" }, body: "{}",
    });
    expect(blocked.status).toBe(403);
    await expect(blocked.json()).resolves.toMatchObject({ error: { code: "share_access_paused" } });
    vi.unstubAllGlobals();
  });

  it("keeps Friend Code lookup exact and prevents duplicate pending requests", async () => {
    const first = await createUser("first-v4@example.test", 81);
    const second = await createUser("second-v4@example.test", 91);
    const incomplete = await SELF.fetch("https://amber.test/v1/friend-requests", {
      method: "POST", headers: first.headers, body: JSON.stringify({ friend_code: second.profile.friend_code.slice(0, -1) }),
    });
    expect(incomplete.status).toBe(404);
    const body = JSON.stringify({ friend_code: second.profile.friend_code });
    expect((await SELF.fetch("https://amber.test/v1/friend-requests", { method: "POST", headers: first.headers, body })).status).toBe(201);
    const duplicate = await SELF.fetch("https://amber.test/v1/friend-requests", { method: "POST", headers: first.headers, body });
    expect(duplicate.status).toBe(409);
  });

  it("consumes an Ed25519 relay challenge exactly once", async () => {
    const owner = await createUser("relay-v4@example.test", 101);
    const keyPair = await crypto.subtle.generateKey("Ed25519", true, ["sign", "verify"]);
    const publicKey = bytesToBase64URL(new Uint8Array(await crypto.subtle.exportKey("raw", keyPair.publicKey)));
    const registered = await SELF.fetch("https://amber.test/v1/devices", {
      method: "POST",
      headers: owner.headers,
      body: JSON.stringify({ name: "Relay test", device_public_key: publicKey, capabilities: ["oauth", "streaming"] }),
    });
    expect(registered.status).toBe(201);
    const deviceID = (await registered.json<{ device: { public_id: string } }>()).device.public_id;
    const challengeResponse = await SELF.fetch(`https://amber.test/v1/devices/${deviceID}/challenge`, {
      method: "POST", headers: owner.headers,
    });
    expect(challengeResponse.status).toBe(200);
    const challenge = await challengeResponse.json<{ challenge: string; expires_at: string }>();
    const canonical = new TextEncoder().encode(`amber-relay-v1|${deviceID}|${challenge.challenge}|${challenge.expires_at}`);
    const digest = await crypto.subtle.digest("SHA-256", canonical);
    const proof = bytesToBase64URL(new Uint8Array(await crypto.subtle.sign("Ed25519", keyPair.privateKey, digest)));
    const connect = () => SELF.fetch(`https://amber.test/v1/relay/connect?device_id=${deviceID}&protocol=1`, {
      headers: {
        ...owner.headers,
        Upgrade: "websocket",
        "X-Amber-Device-Challenge": challenge.challenge,
        "X-Amber-Device-Challenge-Expires": challenge.expires_at,
        "X-Amber-Device-Proof": proof,
      },
    });
    const connected = await connect();
    expect(connected.status).toBe(101);
    connected.webSocket?.accept();
    connected.webSocket?.close(1000, "test complete");
    const replay = await connect();
    expect(replay.status).toBe(401);
    await expect(replay.json()).resolves.toMatchObject({ error: { code: "invalid_device_proof" } });
  });

  it("coordinates per-key concurrency and RPM across requests", async () => {
    const concurrencyStub = env.SHARE_ACCESS.get(env.SHARE_ACCESS.idFromName("access:test-v4-concurrency"));
    const acquireConcurrency = (ticket: string) => concurrencyStub.fetch("https://access.internal/acquire", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ticket, rpm: 600, concurrency: 1 }),
    });
    expect((await acquireConcurrency("ticket-one")).status).toBe(200);
    const concurrent = await acquireConcurrency("ticket-two");
    expect(concurrent.status).toBe(429);
    await expect(concurrent.json()).resolves.toMatchObject({ error: "share_concurrency_limited" });
    expect((await concurrencyStub.fetch("https://access.internal/release", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ticket: "ticket-one" }),
    })).status).toBe(200);

    const rpmStub = env.SHARE_ACCESS.get(env.SHARE_ACCESS.idFromName("access:test-v4-rpm"));
    const acquireRPM = (ticket: string) => rpmStub.fetch("https://access.internal/acquire", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ticket, rpm: 1, concurrency: 2 }),
    });
    expect((await acquireRPM("ticket-three")).status).toBe(200);
    await rpmStub.fetch("https://access.internal/release", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ticket: "ticket-three" }),
    });
    const rpmLimited = await acquireRPM("ticket-four");
    expect(rpmLimited.status).toBe(429);
    await expect(rpmLimited.json()).resolves.toMatchObject({ error: "share_rate_limited" });
  });
});
