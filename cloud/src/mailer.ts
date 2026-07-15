import { AppError } from "./errors";
import { sha256 } from "./security";
import type { Bindings } from "./types";

export interface Mailer {
  sendVerification(email: string, code: string): Promise<void>;
}

class ResendMailer implements Mailer {
  constructor(private readonly env: Bindings) {}

  async sendVerification(email: string, code: string): Promise<void> {
    if (!this.env.RESEND_API_KEY) {
      throw new AppError(503, "mailer_not_configured", "Email delivery is not configured.");
    }
    const response = await fetch("https://api.resend.com/emails", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${this.env.RESEND_API_KEY}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        from: this.env.RESEND_FROM || "Amber <noreply@example.invalid>",
        to: [email],
        subject: "Verify your Amber Cloud account",
        html: `<p>Your Amber verification code is:</p><p style="font-size:28px;font-weight:700;letter-spacing:6px">${code}</p><p>This code expires in 10 minutes.</p>`,
        text: `Your Amber verification code is ${code}. It expires in 10 minutes.`,
      }),
    });
    if (!response.ok) {
      throw new AppError(503, "email_delivery_failed", "The verification email could not be sent.");
    }
  }
}

class ConsoleMailer implements Mailer {
  async sendVerification(email: string, code: string): Promise<void> {
    const [name = "", domain = ""] = email.split("@", 2);
    const masked = `${name.slice(0, 2)}***@${domain}`;
    console.info(JSON.stringify({ event: "development_verification_code", email: masked, code }));
  }
}

class TestMailer implements Mailer {
  constructor(private readonly env: Bindings) {}

  async sendVerification(email: string, code: string): Promise<void> {
    await this.env.SESSIONS.put(`test-mail:${await sha256(email)}`, code, { expirationTtl: 10 * 60 });
  }
}

export function mailerFor(env: Bindings): Mailer {
  if (env.ENVIRONMENT === "test") return new TestMailer(env);
  if (env.ENVIRONMENT !== "production" && env.MAILER_MODE === "console") return new ConsoleMailer();
  return new ResendMailer(env);
}
