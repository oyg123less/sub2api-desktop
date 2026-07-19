import { Hono, type Context } from "hono";
import { requireAuth } from "./auth-middleware";
import { AppError } from "./errors";
import type { AppEnv } from "./types";

interface UserEventRow {
  id: number;
  event_type: string;
  entity_type: string;
  entity_public_id: string;
  payload_json: string;
  created_at: string;
}

export function userEventStatement(
  c: Context<AppEnv>,
  userID: number,
  eventType: string,
  entityType: string,
  entityPublicID = "",
  payload: Record<string, unknown> = {},
): D1PreparedStatement {
  return c.env.DB.prepare(`INSERT INTO cloud_user_events
    (user_id,event_type,entity_type,entity_public_id,payload_json,created_at) VALUES(?,?,?,?,?,?)`)
    .bind(userID, eventType, entityType, entityPublicID, JSON.stringify(payload), new Date().toISOString());
}

export function shareRecipientEventStatement(
  c: Context<AppEnv>,
  groupID: number,
  eventType: string,
  entityType: string,
  entityPublicID = "",
): D1PreparedStatement {
  return c.env.DB.prepare(`INSERT INTO cloud_user_events
    (user_id,event_type,entity_type,entity_public_id,payload_json,created_at)
    SELECT recipient_id,?,?,?,'{}',? FROM share_group_recipients
    WHERE group_id=? AND status IN ('active','paused')`)
    .bind(eventType, entityType, entityPublicID, new Date().toISOString(), groupID);
}

const routes = new Hono<AppEnv>();
routes.use("/events", requireAuth);

routes.get("/events", async (c) => {
  const rawCursor = c.req.query("cursor") || "0";
  if (!/^\d+$/.test(rawCursor)) throw new AppError(400, "invalid_event_cursor", "The event cursor is invalid.");
  const cursor = Number(rawCursor);
  if (!Number.isSafeInteger(cursor) || cursor < 0) throw new AppError(400, "invalid_event_cursor", "The event cursor is invalid.");
  const requestedLimit = Number(c.req.query("limit") || "100");
  const limit = Number.isInteger(requestedLimit) ? Math.min(100, Math.max(1, requestedLimit)) : 100;
  const result = await c.env.DB.prepare(`SELECT id,event_type,entity_type,entity_public_id,payload_json,created_at
    FROM cloud_user_events WHERE user_id=? AND id>? ORDER BY id ASC LIMIT ?`)
    .bind(c.get("auth").id, cursor, limit + 1).all<UserEventRow>();
  const rows = result.results;
  const hasMore = rows.length > limit;
  const page = hasMore ? rows.slice(0, limit) : rows;
  const events = page.map((row) => {
    let payload: Record<string, unknown> = {};
    try { payload = JSON.parse(row.payload_json) as Record<string, unknown>; } catch { /* Invalid optional payloads are ignored. */ }
    return {
      id: row.id,
      event_type: row.event_type,
      entity_type: row.entity_type,
      entity_public_id: row.entity_public_id,
      payload,
      created_at: row.created_at,
    };
  });
  return c.json({ events, cursor: events.at(-1)?.id ?? cursor, has_more: hasMore });
});

export default routes;
