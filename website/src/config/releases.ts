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
  version: "0.4.6",
  releaseUrl: "https://github.com/oyg123less/sub2api-desktop/releases/tag/v0.4.6",
  downloadUrl: "https://github.com/oyg123less/sub2api-desktop/releases/download/v0.4.6/Amber_0.4.6_x64-setup.exe",
  installerName: "Amber_0.4.6_x64-setup.exe",
  installerSizeBytes: 8_069_568,
  sha256: "0C5561A3AB833FDBC225729F593FC0E8191C3C72F6C0503527FF2E0DBC0FA628",
  publishedAt: "2026-09-18T06:00:00Z",
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
