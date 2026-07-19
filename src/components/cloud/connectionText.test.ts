import { describe, expect, it } from "vitest";
import { formatConnectionCode, formatConnectionText, parseConnectionText } from "./connectionText";

describe("connection text", () => {
  it("formats a stable nine-digit code", () => {
    expect(formatConnectionCode("572814639")).toBe("572 814 639");
  });

  it("round trips the copied Chinese connection block", () => {
    const text = formatConnectionText("572814639", "AB3D5F", "2026-07-19T12:00:00Z");
    expect(parseConnectionText(text)).toEqual({ connectionCode: "572814639", password: "AB3D5F" });
  });

  it("accepts compact English labels and rejects partial details", () => {
    expect(parseConnectionText("Connection Code: 572-814-639\nPassword: AB3D5F")).toEqual({ connectionCode: "572814639", password: "AB3D5F" });
    expect(parseConnectionText("572 814 639")).toBeNull();
  });
});
