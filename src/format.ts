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

const usdUnits: Array<{ threshold: number; suffix: string }> = [
  { threshold: 1e9, suffix: "B" },
  { threshold: 1e6, suffix: "M" },
  { threshold: 1e3, suffix: "K" },
];

// formatUSD keeps ordinary values precise and abbreviates large estimates so
// metric cards remain stable. exactUSD is used in the hover title.
export function formatUSD(n?: number | null): string {
  if (n === undefined || n === null || !Number.isFinite(n)) return "$0.0000";
  const negative = n < 0;
  const abs = Math.abs(n);
  for (const unit of usdUnits) {
    if (abs >= unit.threshold) {
      const scaled = (abs / unit.threshold).toFixed(4);
      return `${negative ? "-" : ""}$${scaled}${unit.suffix}`;
    }
  }
  return `${negative ? "-" : ""}$${abs.toFixed(4)}`;
}

export function exactUSD(n?: number | null): string {
  if (n === undefined || n === null || !Number.isFinite(n)) return "$0.0000";
  return n.toLocaleString("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 4, maximumFractionDigits: 4 });
}
