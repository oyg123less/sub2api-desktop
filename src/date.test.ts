import { describe, expect, it } from "vitest";
import { localDateString } from "./date";

describe("localDateString", () => {
  it("uses local calendar components instead of UTC", () => {
    expect(localDateString(new Date(2026, 6, 17, 0, 30))).toBe("2026-07-17");
  });
});
