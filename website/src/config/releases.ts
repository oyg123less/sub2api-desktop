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
  version: "0.4.4",
  releaseUrl: "https://github.com/oyg123less/sub2api-desktop/releases/tag/v0.4.4",
  downloadUrl: "https://github.com/oyg123less/sub2api-desktop/releases/download/v0.4.4/Amber_0.4.4_x64-setup.exe",
  installerName: "Amber_0.4.4_x64-setup.exe",
  installerSizeBytes: 8_005_866,
  sha256: "BA956575A2F326ECF7D29F42CE48938C42A927DB9805BCB30018347FFBFBE6FD",
  publishedAt: "2026-07-19T23:50:36Z",
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
