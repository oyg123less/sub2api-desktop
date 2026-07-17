import { describe, expect, it } from "vitest";
import { exactTokens, formatTokens } from "./format";

describe("formatTokens", () => {
  it("keeps small numbers unabbreviated", () => {
    expect(formatTokens(0)).toBe("0");
    expect(formatTokens(856)).toBe("856");
    expect(formatTokens(999)).toBe("999");
  });

  it("abbreviates thousands", () => {
    expect(formatTokens(1000)).toBe("1K");
    expect(formatTokens(12500)).toBe("12.5K");
    expect(formatTokens(999999)).toBe("1000K");
  });

  it("abbreviates millions and billions", () => {
    expect(formatTokens(1e6)).toBe("1M");
    expect(formatTokens(3421532)).toBe("3.42M");
    expect(formatTokens(1e9)).toBe("1B");
    expect(formatTokens(1070000000)).toBe("1.07B");
  });

  it("abbreviates trillions", () => {
    expect(formatTokens(1e12)).toBe("1T");
    expect(formatTokens(2.4e12)).toBe("2.4T");
  });

  it("trims trailing zeros", () => {
    expect(formatTokens(1500000)).toBe("1.5M");
    expect(formatTokens(2000)).toBe("2K");
  });

  it("handles missing and invalid values", () => {
    expect(formatTokens(undefined)).toBe("0");
    expect(formatTokens(null)).toBe("0");
    expect(formatTokens(Number.NaN)).toBe("0");
    expect(formatTokens(-12500)).toBe("-12.5K");
  });
});

describe("exactTokens", () => {
  it("renders precise grouped values", () => {
    expect(exactTokens(3421532)).toBe("3,421,532");
    expect(exactTokens(undefined)).toBe("0");
  });
});
