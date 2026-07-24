import { describe, expect, it } from "vitest";
import { DEFAULT_CODEX_USER_AGENT, upstreamClientHeaders } from "../src/upstream-client-headers";

describe("upstreamClientHeaders", () => {
  it("passes through an official Codex client identity", () => {
    const incoming = new Headers({
      "User-Agent": "codex_chatgpt_desktop/0.146.0 (Windows 11; x86_64)",
      originator: "Codex Desktop",
      "OpenAI-Beta": "responses=stable",
      session_id: "session-123",
    });

    expect(upstreamClientHeaders(incoming)).toEqual({
      "User-Agent": incoming.get("User-Agent"),
      originator: "Codex Desktop",
      "OpenAI-Beta": "responses=stable",
      session_id: "session-123",
    });
  });

  it("uses the current fallback for non-Codex clients", () => {
    const headers = upstreamClientHeaders(new Headers({
      "User-Agent": "cursor/1.0",
      originator: "cursor",
      "OpenAI-Beta": "untrusted-beta",
      session_id: "untrusted-session",
    }));

    expect(headers["User-Agent"]).toBe(DEFAULT_CODEX_USER_AGENT);
    expect(headers.originator).toBe("codex_cli_rs");
    expect(headers["OpenAI-Beta"]).toBe("responses=experimental");
    expect(headers.session_id).toBeTruthy();
    expect(headers.session_id).not.toBe("untrusted-session");
  });

  it("does not trust an official token embedded in a third-party user agent", () => {
    const headers = upstreamClientHeaders(new Headers({
      "User-Agent": "third-party/1.0 codex_cli_rs/0.146.0",
      originator: "third-party",
      "OpenAI-Beta": "untrusted-beta",
      session_id: "untrusted-session",
    }));

    expect(headers["User-Agent"]).toBe(DEFAULT_CODEX_USER_AGENT);
    expect(headers.originator).toBe("codex_cli_rs");
    expect(headers["OpenAI-Beta"]).toBe("responses=experimental");
    expect(headers.session_id).not.toBe("untrusted-session");
  });

  it("recognizes a real Codex originator in the user-agent trailer", () => {
    const incoming = new Headers({
      "User-Agent": "custom/0.146.0 (Windows 11; x86_64) (codex-tui; 0.146.0)",
      originator: "custom",
      "OpenAI-Beta": "responses=stable",
      session_id: "official-session",
    });

    expect(upstreamClientHeaders(incoming)).toEqual({
      "User-Agent": incoming.get("User-Agent"),
      originator: "codex_cli_rs",
      "OpenAI-Beta": "responses=stable",
      session_id: "official-session",
    });
  });
});
