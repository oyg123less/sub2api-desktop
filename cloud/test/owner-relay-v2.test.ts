import { env } from "cloudflare:test";
import { describe, expect, it } from "vitest";
import { OwnerRelay } from "../src/owner-relay";
import type { Bindings } from "../src/types";

interface MessageChannel {
  socket: WebSocket;
  messages: Array<Record<string, unknown>>;
  next(type: string): Promise<Record<string, unknown>>;
}

function messageChannel(socket: WebSocket): MessageChannel {
  const messages: Array<Record<string, unknown>> = [];
  const waiters: Array<() => void> = [];
  socket.addEventListener("message", (event) => {
    if (typeof event.data !== "string") return;
    messages.push(JSON.parse(event.data) as Record<string, unknown>);
    waiters.splice(0).forEach((resolve) => resolve());
  });
  return {
    socket,
    messages,
    async next(type: string) {
      const deadline = Date.now() + 2_000;
      for (;;) {
        const index = messages.findIndex((message) => message.type === type);
        if (index >= 0) return messages.splice(index, 1)[0]!;
        const remaining = deadline - Date.now();
        if (remaining <= 0) throw new Error(`timed out waiting for relay message ${type}`);
        await new Promise<void>((resolve, reject) => {
          const timer = setTimeout(() => reject(new Error(`timed out waiting for relay message ${type}`)), remaining);
          waiters.push(() => { clearTimeout(timer); resolve(); });
        });
      }
    },
  };
}

async function connect(relay: OwnerRelay, deviceID: string): Promise<MessageChannel> {
  const response = await relay.fetch(new Request("https://relay.internal/connect", {
    headers: {
      Upgrade: "websocket",
      "X-Amber-Device-ID": deviceID,
      "X-Amber-Relay-Session": `session-${deviceID}`,
      "X-Amber-Relay-Protocol": "2",
    },
  }));
  expect(response.status).toBe(101);
  const socket = response.webSocket!;
  const channel = messageChannel(socket);
  socket.accept();
  await channel.next("hello_ack");
  socket.send(JSON.stringify({
    protocol: 2,
    type: "inventory",
    accounts: [{ account_uid: "acct-fixed", enabled: true, healthy: true, proxy_ready: true, active_requests: 0, max_concurrency: 3 }],
  }));
  await new Promise((resolve) => setTimeout(resolve, 0));
  return channel;
}

function createRelay(): OwnerRelay {
  const state = {
    waitUntil(promise: Promise<unknown>) { void promise.catch(() => undefined); },
  } as unknown as DurableObjectState;
  return new OwnerRelay(state, env as unknown as Bindings);
}

function relayRequest() {
  return new Request("https://relay.internal/request", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      request_id: crypto.randomUUID(),
      account_uid: "acct-fixed",
      host_device_id: "device-primary",
      fallback_device_ids: ["device-backup"],
      endpoint: "responses",
      body: "e30",
    }),
  });
}

describe("OwnerRelay protocol 2 routing", () => {
  it("uses the fixed primary and only fails over before upstream starts", async () => {
    const relay = createRelay();
    const primary = await connect(relay, "device-primary");
    const backup = await connect(relay, "device-backup");

    const responsePromise = relay.fetch(relayRequest());
    const firstAttempt = await primary.next("relay_request");
    expect(firstAttempt.attempt_id).toEqual(expect.any(String));
    expect(backup.messages.some((message) => message.type === "relay_request")).toBe(false);
    primary.socket.send(JSON.stringify({
      protocol: 2,
      type: "relay_error",
      request_id: firstAttempt.request_id,
      attempt_id: firstAttempt.attempt_id,
      status: 503,
      error_code: "proxy_unavailable",
    }));

    const secondAttempt = await backup.next("relay_request");
    expect(secondAttempt.attempt_id).toEqual(expect.any(String));
    expect(secondAttempt.attempt_id).not.toBe(firstAttempt.attempt_id);
    backup.socket.send(JSON.stringify({
      protocol: 2,
      type: "response_start",
      request_id: secondAttempt.request_id,
      attempt_id: secondAttempt.attempt_id,
      status: 200,
      headers: { "content-type": "application/json" },
    }));
    const response = await responsePromise;
    backup.socket.send(JSON.stringify({
      protocol: 2,
      type: "response_end",
      request_id: secondAttempt.request_id,
      attempt_id: secondAttempt.attempt_id,
    }));
    expect(response.status).toBe(200);
    await response.text();
    primary.socket.close(1000, "test complete");
    backup.socket.close(1000, "test complete");
  });

  it("does not replay on a backup after upstream_started", async () => {
    const relay = createRelay();
    const primary = await connect(relay, "device-primary");
    const backup = await connect(relay, "device-backup");

    const responsePromise = relay.fetch(relayRequest());
    const attempt = await primary.next("relay_request");
    primary.socket.send(JSON.stringify({ protocol: 2, type: "upstream_started", request_id: attempt.request_id, attempt_id: attempt.attempt_id }));
    primary.socket.send(JSON.stringify({
      protocol: 2,
      type: "relay_error",
      request_id: attempt.request_id,
      attempt_id: attempt.attempt_id,
      status: 502,
      error_code: "connection_lost",
      upstream_started: true,
    }));

    const response = await responsePromise;
    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toMatchObject({ error: { code: "relay_result_unknown" } });
    await new Promise((resolve) => setTimeout(resolve, 25));
    expect(backup.messages.some((message) => message.type === "relay_request")).toBe(false);
    primary.socket.close(1000, "test complete");
    backup.socket.close(1000, "test complete");
  });
});
