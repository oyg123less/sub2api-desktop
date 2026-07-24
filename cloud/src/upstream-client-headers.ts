export const DEFAULT_CODEX_USER_AGENT = "codex_cli_rs/0.146.0 (Amber Cloud Share)";

const CODEX_USER_AGENT_PREFIXES = [
  "codex_cli_rs/",
  "codex-tui/",
  "codex_vscode/",
  "codex_vscode_copilot/",
  "codex_app/",
  "codex_chatgpt_desktop/",
  "codex_atlas/",
  "codex_exec/",
  "codex_sdk_ts/",
];

const CODEX_ORIGINATORS = new Set([
  "codex_cli_rs",
  "codex-tui",
  "codex_vscode",
  "codex_vscode_copilot",
  "codex_app",
  "codex_chatgpt_desktop",
  "codex_atlas",
  "codex_exec",
  "codex_sdk_ts",
]);

function isOfficialCodexUserAgent(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  if (normalized.startsWith("codex ") || CODEX_USER_AGENT_PREFIXES.some((prefix) => normalized.startsWith(prefix))) {
    return true;
  }
  const open = normalized.lastIndexOf("(");
  const close = open >= 0 ? normalized.indexOf(")", open + 1) : -1;
  if (open < 0 || close < 0) return false;
  const trailerOriginator = normalized.slice(open + 1, close).split(";", 1)[0]?.trim() || "";
  return isOfficialCodexOriginator(trailerOriginator);
}

function isOfficialCodexOriginator(value: string): boolean {
  const normalized = value.trim().toLowerCase();
  return normalized.startsWith("codex ") || CODEX_ORIGINATORS.has(normalized);
}

export function upstreamClientHeaders(incoming: Headers): Record<string, string> {
  const incomingUserAgent = incoming.get("User-Agent")?.trim() || "";
  const incomingOriginator = incoming.get("originator")?.trim() || "";
  const officialClient = isOfficialCodexUserAgent(incomingUserAgent) || isOfficialCodexOriginator(incomingOriginator);

  return {
    "User-Agent": isOfficialCodexUserAgent(incomingUserAgent) ? incomingUserAgent : DEFAULT_CODEX_USER_AGENT,
    originator: isOfficialCodexOriginator(incomingOriginator) ? incomingOriginator : "codex_cli_rs",
    "OpenAI-Beta": officialClient && incoming.get("OpenAI-Beta")?.trim()
      ? incoming.get("OpenAI-Beta")!.trim()
      : "responses=experimental",
    session_id: officialClient && incoming.get("session_id")?.trim()
      ? incoming.get("session_id")!.trim()
      : crypto.randomUUID(),
  };
}
