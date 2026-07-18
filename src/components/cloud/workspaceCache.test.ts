import { beforeEach, describe, expect, it, vi } from "vitest";
import { clearWorkspaceCache, readWorkspaceCache, writeWorkspaceCache, type CloudWorkspaceSnapshot } from "./workspaceCache";

function snapshot(): CloudWorkspaceSnapshot {
  return {
    profile: { display_name: "Owner", friend_code: "AMB-TEST-0001", encryption_public_key: "public", encryption_key_version: 1, created_at: "", updated_at: "" },
    friends: [], friendRequests: [], shareGroups: [], receivedShares: [], devices: [], accounts: [], relayEnabled: false,
  };
}

describe("cloud workspace memory cache", () => {
  beforeEach(() => {
    clearWorkspaceCache();
    vi.useRealTimers();
  });

  it("isolates cached data by cloud owner and clears it on logout", () => {
    const value = snapshot();
    writeWorkspaceCache("owner@example.test", value);
    expect(readWorkspaceCache("owner@example.test")).toBe(value);
    expect(readWorkspaceCache("other@example.test")).toBeNull();
    clearWorkspaceCache("owner@example.test");
    expect(readWorkspaceCache("owner@example.test")).toBeNull();
  });

  it("expires stale snapshots", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-18T00:00:00Z"));
    writeWorkspaceCache("owner@example.test", snapshot());
    vi.advanceTimersByTime(2 * 60 * 1000 + 1);
    expect(readWorkspaceCache("owner@example.test")).toBeNull();
  });
});
