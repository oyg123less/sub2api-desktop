import { safeEqual } from "./security";

export const verificationLifetimeMs = 15 * 60 * 1000;
export const previousCodeGraceMs = 3 * 60 * 1000;

export interface VerificationCodeRecord {
  code_hash: string;
  expires_at: number;
  sent_at: number;
  message_id?: string;
}

export interface VerificationRecord {
  user_id: number;
  codes: VerificationCodeRecord[];
  attempts: number;
  expires_at: number;
}

interface LegacyVerificationRecord {
  user_id?: unknown;
  code_hash?: unknown;
  attempts?: unknown;
  expires_at?: unknown;
  codes?: unknown;
}

function validCode(value: unknown): value is VerificationCodeRecord {
  if (!value || typeof value !== "object") return false;
  const code = value as Record<string, unknown>;
  return typeof code.code_hash === "string" && /^[A-Za-z0-9_-]{43}$/.test(code.code_hash) &&
    typeof code.expires_at === "number" && Number.isFinite(code.expires_at) &&
    typeof code.sent_at === "number" && Number.isFinite(code.sent_at) &&
    (code.message_id === undefined || (typeof code.message_id === "string" && code.message_id.length <= 128));
}

export function normalizeVerificationRecord(value: unknown): VerificationRecord | null {
  if (!value || typeof value !== "object") return null;
  const source = value as LegacyVerificationRecord;
  if (typeof source.user_id !== "number" || !Number.isInteger(source.user_id) || source.user_id <= 0 ||
      typeof source.expires_at !== "number" || !Number.isFinite(source.expires_at)) return null;
  const attempts = typeof source.attempts === "number" && Number.isInteger(source.attempts) && source.attempts >= 0
    ? source.attempts : 0;
  let codes: VerificationCodeRecord[] = [];
  if (Array.isArray(source.codes)) codes = source.codes.filter(validCode).slice(-2);
  else if (typeof source.code_hash === "string" && /^[A-Za-z0-9_-]{43}$/.test(source.code_hash)) {
    codes = [{
      code_hash: source.code_hash,
      expires_at: source.expires_at,
      sent_at: source.expires_at - 10 * 60 * 1000,
    }];
  }
  if (!codes.length) return null;
  return { user_id: source.user_id, attempts, expires_at: source.expires_at, codes };
}

export function newVerificationRecord(
  userID: number,
  codeHash: string,
  messageID: string,
  now = Date.now(),
): VerificationRecord {
  const expiresAt = now + verificationLifetimeMs;
  return {
    user_id: userID,
    codes: [{ code_hash: codeHash, message_id: messageID, sent_at: now, expires_at: expiresAt }],
    attempts: 0,
    expires_at: expiresAt,
  };
}

export function rotateVerificationRecord(
  previous: VerificationRecord | null,
  userID: number,
  codeHash: string,
  messageID: string,
  now = Date.now(),
): VerificationRecord {
  const previousCode = previous?.codes
    .filter((entry) => entry.expires_at > now)
    .sort((left, right) => right.sent_at - left.sent_at)[0];
  const current = newVerificationRecord(userID, codeHash, messageID, now);
  if (!previousCode) return current;
  return {
    ...current,
    codes: [
      { ...previousCode, expires_at: Math.min(previousCode.expires_at, now + previousCodeGraceMs) },
      current.codes[0]!,
    ],
  };
}

export function verificationMatches(record: VerificationRecord, suppliedHash: string, now = Date.now()): boolean {
  let matches = false;
  for (const entry of record.codes) {
    const active = entry.expires_at > now;
    const equal = safeEqual(entry.code_hash, suppliedHash);
    matches = matches || (active && equal);
  }
  return matches;
}

export function verificationKVOptions(record: VerificationRecord): { expiration: number } {
  return { expiration: Math.ceil(record.expires_at / 1000) };
}
