// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  isNewerVersion,
  isUpdateCheckEnabled,
  setUpdateCheckEnabled,
  UPDATE_PREFERENCE_EVENT,
} from "./update";

beforeEach(() => localStorage.clear());

describe("update version comparison", () => {
  it.each([
    ["v0.2.2", "0.2.1", true],
    ["v0.3.0", "0.2.9", true],
    ["v1.0.0", "0.9.9", true],
    ["v0.2.1", "0.2.1", false],
    ["v0.2.0", "0.2.1", false],
    ["v0.3.0-beta.2", "0.3.0-beta.1", true],
    ["v0.3.0", "0.3.0-beta.2", true],
    ["latest", "0.2.1", false],
  ])("compares %s with %s", (candidate, current, expected) => {
    expect(isNewerVersion(candidate, current)).toBe(expected);
  });
});

describe("update preference", () => {
  it("defaults to enabled and emits changes", () => {
    const listener = vi.fn();
    window.addEventListener(UPDATE_PREFERENCE_EVENT, listener);
    expect(isUpdateCheckEnabled()).toBe(true);
    setUpdateCheckEnabled(false);
    expect(isUpdateCheckEnabled()).toBe(false);
    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener(UPDATE_PREFERENCE_EVENT, listener);
  });
});
