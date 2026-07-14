import { describe, expect, it } from "vitest";
import { validateCodexRemoteForm, type CodexRemoteFormValue } from "./codexRemote";

const valid: CodexRemoteFormValue = {
  host: "example.test",
  port: 22,
  user: "deploy",
  password: "secret",
  model: "gpt-5.6",
  remotePort: 8080,
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
});
