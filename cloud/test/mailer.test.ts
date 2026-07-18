import { describe, expect, it } from "vitest";
import { shouldUseQQSmtp, verificationEmailPayload } from "../src/mailer";
import { buildQQSmtpMessage } from "../src/qq-smtp";

describe("verification email", () => {
  it("uses a reply-friendly bilingual 15-minute template", () => {
    const message = verificationEmailPayload(
      "Amber Verification <verify@mail.amberapp.asia>",
      "recipient@example.test",
      "123456",
    );
    expect(message.from).toBe("Amber Verification <verify@mail.amberapp.asia>");
    expect(message.from.toLowerCase()).not.toContain("noreply");
    expect(message.from.toLowerCase()).not.toContain("no-reply");
    expect(message.subject).toContain("Amber 云账户验证码");
    expect(message.subject).toContain("Amber verification code");
    expect(message.html).toContain("123456");
    expect(message.html).toContain("15 分钟");
    expect(message.text).toContain("15 minutes");
    expect(message.tags).toEqual([{ name: "category", value: "verification" }]);
  });
});

describe("verification email routing", () => {
  it.each([
    "user@qq.com", "user@foxmail.com", "user@163.com", "user@126.com", "user@yeah.net",
    "user@139.com", "user@189.cn", "user@wo.cn", "user@sina.com", "user@sohu.com",
    "user@aliyun.com", "user@21cn.com",
  ])("routes domestic mailbox %s through QQ SMTP", (email) => {
    expect(shouldUseQQSmtp(email)).toBe(true);
  });

  it.each(["user@gmail.com", "user@outlook.com", "user@proton.me", "user@example.cn"])(
    "keeps non-listed mailbox %s on Resend",
    (email) => expect(shouldUseQQSmtp(email)).toBe(false),
  );

  it("builds an RFC-compatible bilingual MIME message without exposing the subject as raw UTF-8", () => {
    const payload = verificationEmailPayload("sender@qq.com", "recipient@163.com", "123456");
    const message = buildQQSmtpMessage(
      "sender@qq.com",
      "recipient@163.com",
      payload,
      "00000000-0000-4000-8000-000000000001",
      new Date("2026-07-19T00:00:00.000Z"),
    );
    expect(message).toContain("From: Amber Verification <sender@qq.com>");
    expect(message).toContain("To: <recipient@163.com>");
    expect(message).toContain("Subject: =?UTF-8?B?");
    expect(message).toContain("Content-Type: multipart/alternative");
    expect(message).not.toContain("123456");
  });
});
