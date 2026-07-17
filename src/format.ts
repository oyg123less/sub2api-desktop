const tokenUnits: Array<{ threshold: number; suffix: string }> = [
  { threshold: 1e12, suffix: "T" },
  { threshold: 1e9, suffix: "B" },
  { threshold: 1e6, suffix: "M" },
  { threshold: 1e3, suffix: "K" },
];

function trimZeros(value: string): string {
  return value.replace(/\.?0+$/, "");
}

// formatTokens abbreviates a token count for display only (856 / 12.5K /
// 3.42M / 1.07B / 2.4T). Stored and transmitted values stay exact.
export function formatTokens(n?: number | null): string {
  if (n === undefined || n === null || !Number.isFinite(n)) return "0";
  const negative = n < 0;
  const abs = Math.abs(n);
  for (const unit of tokenUnits) {
    if (abs >= unit.threshold) {
      const scaled = trimZeros((abs / unit.threshold).toFixed(2));
      return `${negative ? "-" : ""}${scaled}${unit.suffix}`;
    }
  }
  return `${negative ? "-" : ""}${Math.round(abs).toLocaleString()}`;
}

// exactTokens renders the precise value, used in hover tooltips next to the
// abbreviated number.
export function exactTokens(n?: number | null): string {
  if (n === undefined || n === null || !Number.isFinite(n)) return "0";
  return Math.round(n).toLocaleString("en-US");
}
