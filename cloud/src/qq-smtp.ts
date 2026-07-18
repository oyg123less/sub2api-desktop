import { connect } from "cloudflare:sockets";
import { AppError } from "./errors";
import type { Bindings } from "./types";

export interface QQSmtpMessage {
  subject: string;
  html: string;
  text: string;
}

const smtpHost = "smtp.qq.com";
const smtpPort = 465;
const smtpTimeoutMs = 15_000;
const encoder = new TextEncoder();

class SmtpProtocolError extends Error {
  constructor(readonly responseCode: number) {
    super("smtp_protocol_error");
    this.name = "SmtpProtocolError";
  }
}

class SmtpResponseReader {
  private buffer = "";
  private readonly decoder = new TextDecoder();

  constructor(private readonly reader: ReadableStreamDefaultReader<Uint8Array>) {}

  async readResponse(): Promise<number> {
    let responseCode: string | undefined;
    for (let lineCount = 0; lineCount < 100; lineCount += 1) {
      const line = await this.readLine();
      const match = /^(\d{3})([ -])/.exec(line);
      if (!match) continue;
      responseCode ??= match[1];
      if (match[1] !== responseCode) throw new SmtpProtocolError(Number(match[1]));
      if (match[2] === " ") return Number(responseCode);
    }
    throw new Error("smtp_response_too_large");
  }

  private async readLine(): Promise<string> {
    while (true) {
      const newline = this.buffer.indexOf("\n");
      if (newline >= 0) {
        const line = this.buffer.slice(0, newline).replace(/\r$/, "");
        this.buffer = this.buffer.slice(newline + 1);
        return line;
      }
      const chunk = await withTimeout(this.reader.read());
      if (chunk.done) throw new Error("smtp_connection_closed");
      this.buffer += this.decoder.decode(chunk.value, { stream: true });
      if (this.buffer.length > 64 * 1024) throw new Error("smtp_response_too_large");
    }
  }
}

async function withTimeout<T>(operation: Promise<T>): Promise<T> {
  let timeoutID: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timeoutID = setTimeout(() => reject(new Error("smtp_timeout")), smtpTimeoutMs);
  });
  try {
    return await Promise.race([operation, timeout]);
  } finally {
    if (timeoutID !== undefined) clearTimeout(timeoutID);
  }
}

async function writeSmtp(writer: WritableStreamDefaultWriter<Uint8Array>, value: string): Promise<void> {
  await withTimeout(writer.write(encoder.encode(value)));
}

async function command(
  writer: WritableStreamDefaultWriter<Uint8Array>,
  reader: SmtpResponseReader,
  value: string,
  expectedCodes: number[],
): Promise<void> {
  await writeSmtp(writer, `${value}\r\n`);
  const code = await reader.readResponse();
  if (!expectedCodes.includes(code)) throw new SmtpProtocolError(code);
}

function validMailbox(value: string): boolean {
  return value.length <= 254 && !/[\r\n]/.test(value) &&
    /^[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,63}$/.test(value);
}

function base64UTF8(value: string): string {
  let binary = "";
  for (const byte of encoder.encode(value)) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function foldedBase64(value: string): string {
  return base64UTF8(value).match(/.{1,76}/g)?.join("\r\n") ?? "";
}

export function buildQQSmtpMessage(
  sender: string,
  recipient: string,
  message: QQSmtpMessage,
  messageUUID = crypto.randomUUID(),
  sentAt = new Date(),
): string {
  if (!validMailbox(sender) || !validMailbox(recipient)) throw new Error("invalid_smtp_mailbox");
  const safeUUID = messageUUID.replace(/[^A-Za-z0-9-]/g, "");
  if (!safeUUID) throw new Error("invalid_message_id");
  const boundary = `amber_${safeUUID.replace(/-/g, "")}`;
  const subject = message.subject.replace(/[\r\n]+/g, " ");
  return [
    `From: Amber Verification <${sender}>`,
    `To: <${recipient}>`,
    `Subject: =?UTF-8?B?${base64UTF8(subject)}?=`,
    `Date: ${sentAt.toUTCString()}`,
    `Message-ID: <${safeUUID}@amberapp.asia>`,
    "MIME-Version: 1.0",
    "Auto-Submitted: auto-generated",
    "X-Auto-Response-Suppress: All",
    `Content-Type: multipart/alternative; boundary="${boundary}"`,
    "",
    `--${boundary}`,
    "Content-Type: text/plain; charset=UTF-8",
    "Content-Transfer-Encoding: base64",
    "",
    foldedBase64(message.text),
    `--${boundary}`,
    "Content-Type: text/html; charset=UTF-8",
    "Content-Transfer-Encoding: base64",
    "",
    foldedBase64(message.html),
    `--${boundary}--`,
    "",
  ].join("\r\n");
}

export async function sendQQSmtpVerification(
  env: Pick<Bindings, "QQ_SMTP_USER" | "QQ_SMTP_AUTH_CODE">,
  recipient: string,
  message: QQSmtpMessage,
): Promise<{ id: string }> {
  const sender = env.QQ_SMTP_USER?.trim().toLowerCase();
  const authorizationCode = env.QQ_SMTP_AUTH_CODE?.trim();
  if (!sender || !authorizationCode) {
    throw new AppError(503, "mailer_not_configured", "Email delivery is not configured.");
  }
  if (!validMailbox(sender) || !validMailbox(recipient) || /[\r\n]/.test(authorizationCode)) {
    throw new AppError(503, "mailer_not_configured", "Email delivery is not configured.");
  }

  const messageUUID = crypto.randomUUID();
  const deliveryID = `qq_${messageUUID.replace(/-/g, "")}`;
  let stage = "connect";
  let socket: ReturnType<typeof connect> | undefined;
  let streamReader: ReadableStreamDefaultReader<Uint8Array> | undefined;
  let streamWriter: WritableStreamDefaultWriter<Uint8Array> | undefined;
  try {
    socket = connect({ hostname: smtpHost, port: smtpPort }, {
      secureTransport: "on",
      allowHalfOpen: false,
    });
    await withTimeout(socket.opened);
    streamReader = socket.readable.getReader();
    streamWriter = socket.writable.getWriter();
    const reader = new SmtpResponseReader(streamReader);

    stage = "greeting";
    if (await reader.readResponse() !== 220) throw new SmtpProtocolError(0);
    stage = "ehlo";
    await command(streamWriter, reader, "EHLO amberapp.asia", [250]);
    stage = "auth";
    await command(streamWriter, reader, "AUTH LOGIN", [334]);
    await command(streamWriter, reader, base64UTF8(sender), [334]);
    await command(streamWriter, reader, base64UTF8(authorizationCode), [235]);
    stage = "envelope";
    await command(streamWriter, reader, `MAIL FROM:<${sender}>`, [250]);
    await command(streamWriter, reader, `RCPT TO:<${recipient}>`, [250, 251]);
    stage = "data";
    await command(streamWriter, reader, "DATA", [354]);
    const mime = buildQQSmtpMessage(sender, recipient, message, messageUUID);
    await writeSmtp(streamWriter, `${mime.replace(/(^|\r\n)\./g, "$1..")}\r\n.\r\n`);
    if (await reader.readResponse() !== 250) throw new SmtpProtocolError(0);

    stage = "quit";
    try {
      await command(streamWriter, reader, "QUIT", [221]);
    } catch {
      // The message has already been accepted; QUIT failure must not trigger a duplicate send.
    }
    return { id: deliveryID };
  } catch (error) {
    console.error(JSON.stringify({
      event: "qq_smtp_send_failed",
      stage,
      error_type: error instanceof Error ? error.name : "unknown",
      smtp_status: error instanceof SmtpProtocolError ? error.responseCode : undefined,
    }));
    throw new AppError(503, "email_delivery_failed", "The verification email could not be sent.");
  } finally {
    try { streamReader?.releaseLock(); } catch { /* already released */ }
    try { streamWriter?.releaseLock(); } catch { /* already released */ }
    try { await socket?.close(); } catch { /* already closed */ }
  }
}
