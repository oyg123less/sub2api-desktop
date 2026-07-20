import { describe, expect, it } from "vitest";
import { faqs, homeFaqIds, homeFaqs } from "../src/data/faq";

describe("shared FAQ data", () => {
  it("keeps every FAQ id unique", () => {
    const ids = faqs.map((faq) => faq.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("selects the five homepage questions from the full FAQ source in a stable order", () => {
    expect(homeFaqIds).toEqual([
      "bad-gateway",
      "proxy-tun",
      "remote-codex-access",
      "owner-device-offline",
      "cloud-account-boundary",
    ]);
    expect(homeFaqs.map((faq) => faq.id)).toEqual(homeFaqIds);

    for (const faq of homeFaqs) {
      expect(faqs).toContain(faq);
      expect(faq.question).toBeTruthy();
    }
  });
});
