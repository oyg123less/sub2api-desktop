import { AppError } from "./errors";
import { sha256 } from "./security";
import type { Bindings } from "./types";

export interface Mailer {
  sendVerification(email: string, code: string): Promise<{ id: string }>;
}

export function verificationEmailPayload(from: string, email: string, code: string) {
  return {
    from,
    to: [email],
    subject: "Amber 云账户验证码 / Amber verification code",
    html: `<!doctype html><html lang="zh-CN"><body style="margin:0;background:#f5f6f8;color:#17191c;font-family:Arial,'Microsoft YaHei',sans-serif"><div style="max-width:520px;margin:32px auto;padding:32px;background:#fff;border:1px solid #e1e4e8;border-radius:8px"><h1 style="margin:0 0 20px;font-size:22px">Amber 云账户验证</h1><p style="margin:0 0 8px;line-height:1.6">你的验证码是：</p><p style="margin:12px 0 20px;font-size:30px;font-weight:700;letter-spacing:6px">${code}</p><p style="margin:0;line-height:1.6;color:#555">验证码将在 15 分钟后失效。若你没有请求此邮件，请忽略。</p><hr style="margin:28px 0;border:0;border-top:1px solid #e8eaed"><p style="margin:0 0 8px;line-height:1.6">Your Amber verification code is shown above.</p><p style="margin:0;line-height:1.6;color:#555">It expires in 15 minutes. Ignore this email if you did not request it.</p></div></body></html>`,
    text: `Amber 云账户验证码：${code}。验证码将在 15 分钟后失效。若你没有请求此邮件，请忽略。\n\nYour Amber verification code is ${code}. It expires in 15 minutes. Ignore this email if you did not request it.`,
    tags: [{ name: "category", value: "verification" }],
  };
}

class ResendMailer implements Mailer {
  constructor(private readonly env: Bindings) {}

  async sendVerification(email: string, code: string): Promise<{ id: string }> {
    const from = this.env.RESEND_FROM?.trim();
    if (!this.env.RESEND_API_KEY || !from) {
      throw new AppError(503, "mailer_not_configured", "Email delivery is not configured.");
    }
    const response = await fetch("https://api.resend.com/emails", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${this.env.RESEND_API_KEY}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(verificationEmailPayload(from, email, code)),
    });
    if (!response.ok) {
      throw new AppError(503, "email_delivery_failed", "The verification email could not be sent.");
    }
    let result: { id?: unknown };
    try {
      result = await response.json<{ id?: unknown }>();
    } catch {
      throw new AppError(503, "email_delivery_failed", "The verification email could not be sent.");
    }
    if (typeof result.id !== "string" || !result.id.trim() || result.id.length > 128) {
      throw new AppError(503, "email_delivery_failed", "The verification email could not be sent.");
    }
    return { id: result.id };
  }
}

class ConsoleMailer implements Mailer {
  async sendVerification(email: string, code: string): Promise<{ id: string }> {
    const [name = "", domain = ""] = email.split("@", 2);
    const masked = `${name.slice(0, 2)}***@${domain}`;
    console.info(JSON.stringify({ event: "development_verification_code", email: masked, code }));
    return { id: `console_${await sha256(email)}` };
  }
}

class TestMailer implements Mailer {
  constructor(private readonly env: Bindings) {}

  async sendVerification(email: string, code: string): Promise<{ id: string }> {
    const emailHash = await sha256(email);
    await this.env.SESSIONS.put(`test-mail:${emailHash}`, code, { expirationTtl: 15 * 60 });
    return { id: `test_${emailHash}` };
  }
}

export function mailerFor(env: Bindings): Mailer {
  if (env.ENVIRONMENT === "test") return new TestMailer(env);
  if (env.ENVIRONMENT !== "production" && env.MAILER_MODE === "console") return new ConsoleMailer();
  return new ResendMailer(env);
}
