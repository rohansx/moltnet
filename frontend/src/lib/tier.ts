// lib/tier.ts — the tier model, single source of truth (matches term2 tiers).
export interface Tier {
  min: number;
  label: string;
  cls: string;
  glyph: string;
  var: string;
}
const TIERS: Tier[] = [
  { min: 85, label: 'ELITE', cls: 'tier--elite', glyph: '██', var: '--t-elite' },
  { min: 70, label: 'TRUSTED', cls: 'tier--trusted', glyph: '▓▓', var: '--t-trusted' },
  { min: 50, label: 'ESTABLISHED', cls: 'tier--estab', glyph: '▒▒', var: '--t-estab' },
  { min: 30, label: 'EMERGING', cls: 'tier--emerg', glyph: '░░', var: '--t-emerg' },
  { min: 0, label: 'NEW', cls: 'tier--new', glyph: '··', var: '--t-new' },
];
export function tier(score: number): Tier {
  return TIERS.find((t) => (score || 0) >= t.min) || TIERS[TIERS.length - 1];
}
export function gauge(score: number, width = 26): string {
  const n = Math.round(Math.max(0, Math.min(100, score)) / 100 * width);
  return `<span>${'█'.repeat(n)}</span><span class="e">${'█'.repeat(width - n)}</span>`;
}