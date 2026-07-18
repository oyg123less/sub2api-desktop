import type { Bindings } from "./types";

interface RelayConnection {
  deviceID: string;
  socket: WebSocket;
  primary: boolean;
  capabilities: string[];
  sessionID: string;
  lastHeartbeat: number;
}

interface PendingRelay {
  requestID: string;
  connection: RelayConnection;
  started: boolean;
  resolved: boolean;
  resolve: (response: Response) => void;
  controller?: ReadableStreamDefaultController<Uint8Array>;
  timer: ReturnType<typeof setTimeout>;
  hardTimer: ReturnType<typeof setTimeout>;
}

interface RelayMessage {
  protocol?: number;
  type?: string;
  request_id?: string;
  status?: number;
  headers?: Record<string, string>;
  data?: string;
  error_code?: string;
  message?: string;
  upstream_started?: boolean;
}

const encoder = new TextEncoder();

function base64URLToBytes(value: string): Uint8Array {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - normalized.length % 4) % 4);
  const raw = atob(padded);
  return Uint8Array.from(raw, (char) => char.charCodeAt(0));
}

function safeRelayHeaders(input: Record<string, string> | undefined): Headers {
  const headers = new Headers();
  for (const name of ["content-type", "openai-processing-ms", "x-request-id", "x-ratelimit-limit-requests", "x-ratelimit-remaining-requests", "x-ratelimit-reset-requests"]) {
    const value = input?.[name];
    if (value) headers.set(name, value.slice(0, 2048));
  }
  headers.set("Cache-Control", "no-store");
  headers.set("X-Content-Type-Options", "nosniff");
  return headers;
}

function relayError(status: number, code: string, message: string): Response {
  return Response.json({ error: { code, message } }, { status, headers: { "Cache-Control": "no-store" } });
}

export class OwnerRelay {
  private readonly state: DurableObjectState;
  private readonly env: Bindings;
  private readonly connections = new Map<string, RelayConnection>();
  private readonly pending = new Map<string, PendingRelay>();

  constructor(state: DurableObjectState, env: Bindings) {
    this.state = state;
    this.env = env;
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/connect") return this.connectWebSocket(request);
    if (url.pathname === "/status") return this.status();
    if (url.pathname === "/request" && request.method === "POST") return this.relay(request);
    if (url.pathname === "/disconnect" && request.method === "POST") return this.disconnect(request);
    return new Response("not found", { status: 404 });
  }

  private connectWebSocket(request: Request): Response {
    if ((request.headers.get("Upgrade") || "").toLowerCase() !== "websocket") return new Response("upgrade required", { status: 426 });
    const deviceID = request.headers.get("X-Amber-Device-ID") || "";
    const sessionID = request.headers.get("X-Amber-Relay-Session") || "";
    if (!deviceID || !sessionID) return new Response("invalid device", { status: 400 });
    const existing = this.connections.get(deviceID);
    if (existing) existing.socket.close(4001, "replaced");
    const pair = new WebSocketPair();
    const client = pair[0];
    const server = pair[1];
    server.accept();
    const connection: RelayConnection = {
      deviceID,
      socket: server,
      sessionID,
      primary: request.headers.get("X-Amber-Device-Primary") === "1",
      capabilities: (request.headers.get("X-Amber-Device-Capabilities") || "").split(",").filter(Boolean),
      lastHeartbeat: Date.now(),
    };
    this.connections.set(deviceID, connection);
    server.addEventListener("message", (event) => this.onMessage(connection, event));
    server.addEventListener("close", () => this.onClose(connection, "socket_closed"));
    server.addEventListener("error", () => this.onClose(connection, "socket_error"));
    server.send(JSON.stringify({ protocol: 1, type: "hello_ack", session_id: sessionID, heartbeat_seconds: 20 }));
    return new Response(null, { status: 101, webSocket: client });
  }

  private status(): Response {
    const now = Date.now();
    const devices = [...this.connections.values()].filter((connection) => now - connection.lastHeartbeat <= 60_000).map((connection) => ({
      public_id: connection.deviceID,
      primary: connection.primary,
      capabilities: connection.capabilities,
      last_heartbeat_at: new Date(connection.lastHeartbeat).toISOString(),
      active_requests: [...this.pending.values()].filter((pending) => pending.connection.deviceID === connection.deviceID).length,
    }));
    return Response.json({ devices });
  }

  private async relay(request: Request): Promise<Response> {
    if (this.pending.size >= 100) return relayError(503, "owner_relay_busy", "The owner's relay is busy.");
    const payload = await request.json<Record<string, unknown>>();
    const requestID = typeof payload.request_id === "string" ? payload.request_id : "";
    if (!requestID || this.pending.has(requestID)) return relayError(400, "invalid_relay_request", "The relay request is invalid.");
    const now = Date.now();
    const candidates = [...this.connections.values()].filter((connection) => now - connection.lastHeartbeat <= 60_000);
    candidates.sort((left, right) => Number(right.primary) - Number(left.primary));
    const connection = candidates[0];
    if (!connection) return relayError(503, "owner_device_offline", "The share owner's device is offline.");
    return new Promise<Response>((resolve) => {
      const timeout = () => {
        const pending = this.pending.get(requestID);
        if (!pending) return;
        if (pending.resolved) pending.controller?.error(new Error("relay timeout"));
        else resolve(relayError(504, "relay_timeout", "The owner-device relay timed out."));
        try { connection.socket.send(JSON.stringify({ protocol: 1, type: "cancel_request", request_id: requestID })); } catch { /* closed */ }
        this.finishPending(pending, false);
      };
      const timer = setTimeout(timeout, 35_000);
      const hardTimer = setTimeout(timeout, 30 * 60_000);
      this.pending.set(requestID, { requestID, connection, started: false, resolved: false, resolve, timer, hardTimer });
      try {
        connection.socket.send(JSON.stringify({ protocol: 1, type: "relay_request", ...payload }));
      } catch {
        clearTimeout(timer);
        this.pending.delete(requestID);
        resolve(relayError(503, "owner_device_offline", "The share owner's device is offline."));
      }
    });
  }

  private async disconnect(request: Request): Promise<Response> {
    const body = await request.json<{ device_id?: string }>();
    const connection = body.device_id ? this.connections.get(body.device_id) : undefined;
    connection?.socket.close(4002, "revoked");
    return Response.json({ ok: true });
  }

  private onMessage(connection: RelayConnection, event: MessageEvent): void {
    if (typeof event.data !== "string" || event.data.length > 128 * 1024) {
      connection.socket.close(4003, "invalid_message");
      return;
    }
    let message: RelayMessage;
    try { message = JSON.parse(event.data) as RelayMessage; } catch { connection.socket.close(4003, "invalid_message"); return; }
    if (message.protocol !== 1 || !message.type) return;
    if (message.type === "heartbeat") {
      connection.lastHeartbeat = Date.now();
      connection.socket.send(JSON.stringify({ protocol: 1, type: "heartbeat_ack", at: new Date().toISOString() }));
      this.state.waitUntil(this.env.DB.prepare("UPDATE share_device_sessions SET last_heartbeat_at=? WHERE id=? AND disconnected_at IS NULL")
        .bind(new Date().toISOString(), connection.sessionID).run());
      return;
    }
    const requestID = message.request_id || "";
    const pending = this.pending.get(requestID);
    if (!pending || pending.connection.deviceID !== connection.deviceID) return;
    if (message.type === "upstream_started") { pending.started = true; this.armIdleTimeout(pending, 30_000); return; }
    if (message.type === "response_start") {
      if (pending.resolved) return;
      pending.resolved = true;
      this.armIdleTimeout(pending, 90_000);
      const stream = new ReadableStream<Uint8Array>({ start: (controller) => { pending.controller = controller; } });
      pending.resolve(new Response(stream, { status: Number(message.status) || 502, headers: safeRelayHeaders(message.headers) }));
      return;
    }
    if (message.type === "response_chunk") {
      if (!pending.resolved || !pending.controller || typeof message.data !== "string") return;
      try {
        pending.controller.enqueue(base64URLToBytes(message.data));
        this.armIdleTimeout(pending, 90_000);
        connection.socket.send(JSON.stringify({ protocol: 1, type: "chunk_ack", request_id: requestID }));
      } catch { this.finishPending(pending, false); }
      return;
    }
    if (message.type === "response_end") { this.finishPending(pending, true); return; }
    if (message.type === "relay_error") {
      const status = Number(message.status) || 502;
      const code = message.upstream_started || pending.started ? "relay_result_unknown" : (message.error_code || "owner_relay_failed");
      const publicMessage = code === "relay_result_unknown"
        ? "The relay disconnected after the upstream request started. Amber did not replay it."
        : (message.message || "The owner-device relay failed.");
      if (pending.resolved) pending.controller?.error(new Error(code));
      else pending.resolve(relayError(status, code, publicMessage));
      this.finishPending(pending, false, false);
    }
  }

  private onClose(connection: RelayConnection, reason: string): void {
    if (this.connections.get(connection.deviceID) === connection) this.connections.delete(connection.deviceID);
    const now = new Date().toISOString();
    this.state.waitUntil(this.env.DB.prepare(`UPDATE share_device_sessions SET disconnected_at=?,close_reason=?
      WHERE id=? AND disconnected_at IS NULL`).bind(now, reason, connection.sessionID).run());
    for (const pending of this.pending.values()) {
      if (pending.connection !== connection) continue;
      const code = pending.started ? "relay_result_unknown" : "owner_device_offline";
      if (pending.resolved) pending.controller?.error(new Error(code));
      else pending.resolve(relayError(pending.started ? 502 : 503, code, pending.started
        ? "The relay disconnected after the upstream request started. Amber did not replay it."
        : "The share owner's device is offline."));
      this.finishPending(pending, false, false);
    }
  }

  private finishPending(pending: PendingRelay, closeStream: boolean, remove = true): void {
    clearTimeout(pending.timer);
    clearTimeout(pending.hardTimer);
    if (closeStream) pending.controller?.close();
    if (remove) this.pending.delete(pending.requestID);
    else this.pending.delete(pending.requestID);
  }

  private armIdleTimeout(pending: PendingRelay, delay: number): void {
    clearTimeout(pending.timer);
    pending.timer = setTimeout(() => {
      if (!this.pending.has(pending.requestID)) return;
      if (pending.resolved) pending.controller?.error(new Error("relay timeout"));
      else pending.resolve(relayError(504, "relay_timeout", "The owner-device relay timed out."));
      try { pending.connection.socket.send(JSON.stringify({ protocol: 1, type: "cancel_request", request_id: pending.requestID })); } catch { /* closed */ }
      this.finishPending(pending, false);
    }, delay);
  }
}
