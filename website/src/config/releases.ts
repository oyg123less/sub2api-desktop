export interface PublicReleaseInfo {
  version: string;
  releaseUrl: string;
  downloadUrl: string;
  installerName: string;
  installerSizeBytes: number;
  sha256: string;
  publishedAt: string;
  status: "stable" | "upcoming";
}

export const stableRelease: PublicReleaseInfo = {
  version: "0.4.5",
  releaseUrl: "https://github.com/oyg123less/sub2api-desktop/releases/tag/v0.4.5",
  downloadUrl: "https://github.com/oyg123less/sub2api-desktop/releases/download/v0.4.5/Amber_0.4.5_x64-setup.exe",
  installerName: "Amber_0.4.5_x64-setup.exe",
  installerSizeBytes: 8_008_724,
  sha256: "81C2B1DB460317DFD3A35F9E06B364F753DE3F82377D0760506CEAAEE5D14528",
  publishedAt: "2026-07-24T18:42:48Z",
  status: "stable",
};

export const upcomingRelease: PublicReleaseInfo = {
  version: "0.4.x",
  releaseUrl: "",
  downloadUrl: "",
  installerName: "",
  installerSizeBytes: 0,
  sha256: "",
  publishedAt: "",
  status: "upcoming",
};

export function formatFileSize(bytes: number): string {
  return `${(bytes / 1024 / 1024).toFixed(2)} MiB`;
}

export function formatPublishedDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "long",
    timeZone: "Asia/Shanghai",
  }).format(new Date(value));
}
