export interface ParsedConnectionText {
  connectionCode: string;
  password: string;
}

export function formatConnectionCode(value: string): string {
  const digits = value.replace(/\D/g, "").slice(0, 9);
  return digits.replace(/(\d{3})(?=\d)/g, "$1 ");
}

export function formatConnectionText(code: string, password: string, expiresAt?: string): string {
  const lines = [
    "Amber 共享连接",
    `连接码：${formatConnectionCode(code)}`,
    `临时密码：${password.trim().toUpperCase()}`,
  ];
  if (expiresAt) lines.push(`有效期至：${new Date(expiresAt).toLocaleString()}`);
  return lines.join("\n");
}

export function parseConnectionText(value: string): ParsedConnectionText | null {
  const text = value.trim().toUpperCase();
  if (!text) return null;
  const codeLabel = text.match(/(?:连接码|CONNECTION\s*CODE)\s*[：:]\s*([\d\s-]{9,15})/i);
  const passwordLabel = text.match(/(?:临时密码|TEMPORARY\s*PASSWORD|PASSWORD)\s*[：:]\s*([A-HJ-NP-Z2-9]{6})/i);
  const looseCode = text.match(/(?:^|\D)(\d{3}[\s-]?\d{3}[\s-]?\d{3})(?:\D|$)/);
  const loosePassword = text.match(/(?:^|\s)([A-HJ-NP-Z2-9]{6})(?:\s|$)/);
  const connectionCode = (codeLabel?.[1] || looseCode?.[1] || "").replace(/\D/g, "");
  const password = passwordLabel?.[1] || loosePassword?.[1] || "";
  return /^\d{9}$/.test(connectionCode) && /^[A-HJ-NP-Z2-9]{6}$/.test(password)
    ? { connectionCode, password }
    : null;
}
