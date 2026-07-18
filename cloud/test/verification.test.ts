import { describe, expect, it } from "vitest";
import {
  newVerificationRecord,
  normalizeVerificationRecord,
  previousCodeGraceMs,
  rotateVerificationRecord,
  verificationLifetimeMs,
  verificationMatches,
} from "../src/verification";

const firstHash = "a".repeat(43);
const secondHash = "b".repeat(43);
const thirdHash = "c".repeat(43);

describe("verification records", () => {
  it("keeps deployed single-code records compatible", () => {
    const expiresAt = 1_000_000;
    const record = normalizeVerificationRecord({
      user_id: 7,
      code_hash: firstHash,
      attempts: 2,
      expires_at: expiresAt,
    });
    expect(record).toMatchObject({ user_id: 7, attempts: 2, expires_at: expiresAt });
    expect(record?.codes).toEqual([{
      code_hash: firstHash,
      sent_at: expiresAt - 10 * 60 * 1000,
      expires_at: expiresAt,
    }]);
  });

  it("issues 15-minute codes and retains the immediately previous code for three minutes", () => {
    const now = 2_000_000;
    const original = newVerificationRecord(9, firstHash, "message_original", now);
    expect(original.expires_at).toBe(now + verificationLifetimeMs);
    const resentAt = now + 60_000;
    const rotated = rotateVerificationRecord(original, 9, secondHash, "message_resent", resentAt);
    expect(rotated.codes).toHaveLength(2);
    expect(rotated.codes[0]).toMatchObject({
      code_hash: firstHash,
      expires_at: resentAt + previousCodeGraceMs,
    });
    expect(rotated.codes[1]).toMatchObject({
      code_hash: secondHash,
      expires_at: resentAt + verificationLifetimeMs,
      message_id: "message_resent",
    });
    expect(verificationMatches(rotated, firstHash, resentAt + previousCodeGraceMs - 1)).toBe(true);
    expect(verificationMatches(rotated, firstHash, resentAt + previousCodeGraceMs)).toBe(false);
    expect(verificationMatches(rotated, secondHash, resentAt + verificationLifetimeMs - 1)).toBe(true);
  });

  it("drops older grace codes on a second resend", () => {
    const now = 3_000_000;
    const original = newVerificationRecord(11, firstHash, "message_one", now);
    const firstResend = rotateVerificationRecord(original, 11, secondHash, "message_two", now + 60_000);
    const secondResend = rotateVerificationRecord(firstResend, 11, thirdHash, "message_three", now + 120_000);
    expect(secondResend.codes.map((entry) => entry.code_hash)).toEqual([secondHash, thirdHash]);
    expect(verificationMatches(secondResend, firstHash, now + 120_001)).toBe(false);
  });
});
