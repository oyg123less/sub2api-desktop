import type {
  Account,
  CloudDevice,
  CloudProfile,
  CloudConnectHost,
  CloudReceivedShare,
} from "../../api/control";

export interface CloudWorkspaceSnapshot {
  profile: CloudProfile;
  receivedShares: CloudReceivedShare[];
  devices: CloudDevice[];
  accounts: Account[];
  relayEnabled: boolean;
  connectHost: CloudConnectHost;
}

const cacheLifetimeMs = 2 * 60 * 1000;
let cached: { owner: string; storedAt: number; snapshot: CloudWorkspaceSnapshot } | null = null;

export function readWorkspaceCache(owner: string): CloudWorkspaceSnapshot | null {
  if (!cached || cached.owner !== owner || Date.now() - cached.storedAt > cacheLifetimeMs) return null;
  return cached.snapshot;
}

export function writeWorkspaceCache(owner: string, snapshot: CloudWorkspaceSnapshot) {
  cached = { owner, storedAt: Date.now(), snapshot };
}

export function clearWorkspaceCache(owner?: string) {
  if (!owner || cached?.owner === owner) cached = null;
}
