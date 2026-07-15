import { api, type ReleaseInfo } from "./control";

export const UPDATE_CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000;
export const UPDATE_PREFERENCE_EVENT = "amber:update-check-preference";

const enabledKey = "amber_update_checks_enabled";
const cacheKey = "amber_update_release_cache";

interface CachedRelease {
  cachedAt: number;
  release: ReleaseInfo;
}

interface SemVer {
  major: number;
  minor: number;
  patch: number;
  prerelease: string[];
}

export function isUpdateCheckEnabled(): boolean {
  return localStorage.getItem(enabledKey) !== "false";
}

export function setUpdateCheckEnabled(enabled: boolean): void {
  localStorage.setItem(enabledKey, String(enabled));
  window.dispatchEvent(new CustomEvent(UPDATE_PREFERENCE_EVENT, { detail: { enabled } }));
}

export async function checkForUpdate(currentVersion: string, force = false): Promise<ReleaseInfo | null> {
  if (!isUpdateCheckEnabled()) return null;

  const cached = readCache();
  if (!force && cached && Date.now() - cached.cachedAt < UPDATE_CHECK_INTERVAL_MS) {
    return isNewerVersion(cached.release.tag_name, currentVersion) ? cached.release : null;
  }

  try {
    const release = await api.latestRelease();
    writeCache(release);
    return isNewerVersion(release.tag_name, currentVersion) ? release : null;
  } catch (error) {
    if (cached) {
      return isNewerVersion(cached.release.tag_name, currentVersion) ? cached.release : null;
    }
    throw error;
  }
}

export function isNewerVersion(candidate: string, current: string): boolean {
  const left = parseSemVer(candidate);
  const right = parseSemVer(current);
  if (!left || !right) return false;

  for (const key of ["major", "minor", "patch"] as const) {
    if (left[key] !== right[key]) return left[key] > right[key];
  }
  if (left.prerelease.length === 0 || right.prerelease.length === 0) {
    return left.prerelease.length === 0 && right.prerelease.length > 0;
  }
  const count = Math.max(left.prerelease.length, right.prerelease.length);
  for (let index = 0; index < count; index++) {
    const a = left.prerelease[index];
    const b = right.prerelease[index];
    if (a === undefined || b === undefined) return a !== undefined;
    if (a === b) continue;
    const aNumber = /^\d+$/.test(a) ? Number(a) : null;
    const bNumber = /^\d+$/.test(b) ? Number(b) : null;
    if (aNumber !== null && bNumber !== null) return aNumber > bNumber;
    if (aNumber !== null || bNumber !== null) return aNumber === null;
    return a > b;
  }
  return false;
}

function parseSemVer(value: string): SemVer | null {
  const match = value.trim().match(/^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/);
  if (!match) return null;
  return {
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
    prerelease: match[4] ? match[4].split(".") : [],
  };
}

function readCache(): CachedRelease | null {
  try {
    const value = JSON.parse(localStorage.getItem(cacheKey) || "null") as Partial<CachedRelease> | null;
    if (!value || typeof value.cachedAt !== "number" || !value.release?.tag_name || !value.release?.html_url) return null;
    return value as CachedRelease;
  } catch {
    return null;
  }
}

function writeCache(release: ReleaseInfo): void {
  try {
    localStorage.setItem(cacheKey, JSON.stringify({ cachedAt: Date.now(), release } satisfies CachedRelease));
  } catch {
    // A disabled or full web storage must not make update checks visible as errors.
  }
}
