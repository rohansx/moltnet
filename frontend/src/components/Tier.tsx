// components/Tier.tsx — tier badge component.
import { tier } from '../lib/tier';

export function TierBadge({ score }: { score: number }) {
  const t = tier(score);
  return (
    <span className={`tier ${t.cls}`}>
      <span className="g">{t.glyph}</span>
      {t.label}
    </span>
  );
}

export function TierGlyph({ score }: { score: number }) {
  const t = tier(score);
  return (
    <span className="g" style={{ color: `var(${t.var})` }}>
      {t.glyph}
    </span>
  );
}