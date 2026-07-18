import { describe, expect, it } from "vitest";
import { verificationEmailPayload } from "../src/mailer";

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
