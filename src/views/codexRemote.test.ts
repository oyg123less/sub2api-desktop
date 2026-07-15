import { describe, expect, it } from "vitest";
import {
  normalizeCodexRemoteTargets,
  sshUserForRequest,
  remoteForm,
  validateCodexRemoteForm,
  type CodexRemoteFormValue,
} from "./codexRemote";

describe("normalizeCodexRemoteTargets", () => {
  it("treats null and malformed list responses as empty", () => {
    expect(normalizeCodexRemoteTargets(null)).toEqual([]);
    expect(normalizeCodexRemoteTargets({ targets: null })).toEqual([]);
    expect(normalizeCodexRemoteTargets({ targets: [null, { id: 0 }] })).toEqual([]);
  });

  it("normalizes legacy targets and unknown statuses", () => {
    expect(normalizeCodexRemoteTargets({
      targets: [{
        id: 4,
        host: "legacy.example.test",
        user: "deploy",
        port: "22",
        model: "gpt-5.6",
        tunnel_status: "future_status",
      }],
    })).toEqual([expect.objectContaining({
      id: 4,
      name: "deploy@legacy.example.test",
      mode: "tunnel",
      port: 22,
      remote_port: 8080,
      tunnel_status: "not_injected",
      tunnel_enabled: false,
    })]);
  });
});

const valid: CodexRemoteFormValue = {
  host: "example.test",
  port: 22,
  user: "deploy",
  password: "secret",
  model: "gpt-5.6",
  remotePort: 8080,
  mode: "tunnel",
  baseUrl: "",
  apiKey: "",
};

describe("validateCodexRemoteForm", () => {
  it("requires host, user, and a password for new targets", () => {
    const errors = validateCodexRemoteForm({ ...valid, host: "", user: "", password: "" });
    expect(errors).toMatchObject({ host: "required", user: "required", password: "required" });
  });

  it("rejects SSH and remote ports outside 1-65535", () => {
    expect(validateCodexRemoteForm({ ...valid, port: 0 }).port).toBe("invalid");
    expect(validateCodexRemoteForm({ ...valid, port: 65536 }).port).toBe("invalid");
    expect(validateCodexRemoteForm({ ...valid, remotePort: Number.NaN }).remotePort).toBe("invalid");
  });

  it("allows an empty password when reinjecting a saved target", () => {
    expect(validateCodexRemoteForm({ ...valid, id: 4, password: "" }).password).toBeUndefined();
  });

  it("accepts user@host and lets the backend extract its user", () => {
    const value = { ...valid, host: "root@1.2.3.4", user: "ignored-user" };
    expect(validateCodexRemoteForm({ ...value, user: "" }).user).toBeUndefined();
    expect(sshUserForRequest(value.host, value.user)).toBe("");
    expect(sshUserForRequest("1.2.3.4", "root")).toBe("root");
  });

  it("requires a valid Base URL and API key in direct mode", () => {
    const direct = { ...valid, mode: "direct" as const, remotePort: Number.NaN };
    expect(validateCodexRemoteForm(direct)).toMatchObject({ baseUrl: "required", apiKey: "required" });
    expect(validateCodexRemoteForm({ ...direct, baseUrl: "ftp://api.example.test", apiKey: "key" }).baseUrl).toBe("invalid");
    expect(validateCodexRemoteForm({ ...direct, baseUrl: "https://key@api.example.test/v1", apiKey: "key" }).baseUrl).toBe("invalid");
    expect(validateCodexRemoteForm({ ...direct, baseUrl: "https://api.example.test/v1", apiKey: "key" })).toEqual({});
  });

  it("allows a saved direct target to reuse its API key", () => {
    const direct = { ...valid, id: 7, password: "", mode: "direct" as const, baseUrl: "https://api.example.test/v1", apiKey: "" };
    expect(validateCodexRemoteForm(direct)).toEqual({});
    expect(validateCodexRemoteForm({ ...direct, id: -1 }).apiKey).toBe("required");
  });

  it("keeps tunnel validation independent from direct-only fields", () => {
    expect(validateCodexRemoteForm({ ...valid, baseUrl: "invalid", apiKey: "" })).toEqual({});
    expect(validateCodexRemoteForm({ ...valid, remotePort: 0 }).remotePort).toBe("invalid");
  });

  it("keeps direct mode drafts in module-level state", () => {
    remoteForm.value.mode = "direct";
    remoteForm.value.baseUrl = "https://draft.example.test/v1";
    remoteForm.value.apiKey = "draft-key";
    expect(remoteForm.value).toMatchObject({
      mode: "direct",
      baseUrl: "https://draft.example.test/v1",
      apiKey: "draft-key",
    });
    remoteForm.value.mode = "tunnel";
    remoteForm.value.baseUrl = "";
    remoteForm.value.apiKey = "";
  });
});
