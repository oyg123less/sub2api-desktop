import { describe, expect, it } from "vitest";
import { formatFileSize, stableRelease, upcomingRelease } from "../src/config/releases";

describe("public release configuration", () => {
  it("publishes v0.4.5 as the downloadable stable release", () => {
    expect(stableRelease.status).toBe("stable");
    expect(stableRelease.version).toBe("0.4.6");
    expect(stableRelease.downloadUrl).toBe(
      "https://github.com/oyg123less/sub2api-desktop/releases/download/v0.4.6/Amber_0.4.6_x64-setup.exe",
    );
    expect(stableRelease.sha256).toMatch(/^[A-F0-9]{64}$/);
    expect(stableRelease.installerSizeBytes).toBe(8_069_568);
    expect(formatFileSize(stableRelease.installerSizeBytes)).toBe("7.70 MiB");
  });

  it("does not expose a download for the upcoming version", () => {
    expect(upcomingRelease.status).toBe("upcoming");
    expect(upcomingRelease.version).toBe("0.4.x");
    expect(upcomingRelease.downloadUrl).toBe("");
    expect(upcomingRelease.releaseUrl).toBe("");
    expect(upcomingRelease.sha256).toBe("");
  });
});
