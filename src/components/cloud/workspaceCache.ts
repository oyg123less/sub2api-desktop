import type {
  Account,
  CloudDevice,
  CloudFriend,
  CloudFriendRequest,
  CloudProfile,
  CloudReceivedShare,
  CloudShareGroup,
} from "../../api/control";

export interface CloudWorkspaceSnapshot {
  profile: CloudProfile;
  friends: CloudFriend[];
  friendRequests: CloudFriendRequest[];
  shareGroups: CloudShareGroup[];
  receivedShares: CloudReceivedShare[];
  devices: CloudDevice[];
  accounts: Account[];
  relayEnabled: boolean;
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
