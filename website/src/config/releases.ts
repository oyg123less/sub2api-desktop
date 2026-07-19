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
  version: "0.4.3",
  releaseUrl: "https://github.com/oyg123less/sub2api-desktop/releases/tag/v0.4.3",
  downloadUrl: "https://github.com/oyg123less/sub2api-desktop/releases/download/v0.4.3/Amber_0.4.3_x64-setup.exe",
  installerName: "Amber_0.4.3_x64-setup.exe",
  installerSizeBytes: 7_966_422,
  sha256: "724988948FD9B8E7CA8208C4D9046766CB9DAF79AB50AAA3014468DA5F1C7F12",
  publishedAt: "2026-07-19T10:46:29Z",
  status: "stable",
};

export const upcomingRelease: PublicReleaseInfo = {
  version: "0.4.4",
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
