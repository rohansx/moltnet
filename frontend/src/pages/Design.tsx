// pages/Design.tsx — the term2 specimen sheet. This is the design system made
// visible: every token and component the rest of the app is built from.
import { Link } from 'react-router-dom';
import { Spine, ThemeToggle, Mark } from '../components/Chrome';
import { TierBadge } from '../components/Tier';
import { tier } from '../lib/tier';

const TOKENS: [string, string][] = [
  ['--bg', 'paper / base'],
  ['--sf', 'raised surface'],
  ['--sf-2', 'inset well'],
  ['--ink', 'ink'],
  ['--ink-2', 'ink-2'],
  ['--ink-3', 'ink-3'],
  ['--ink-4', 'ink-4'],
  ['--ac', 'accent · signal'],
  ['--pos', 'positive'],
  ['--neg', 'negative'],
  ['--warn', 'warning'],
  ['--info', 'info'],
  ['--line-2', 'hairline'],
];

const TIER_SCORES = [90, 76, 58, 38, 10];

export function Design() {
  return (
    <>
      <Spine middle="TERM2 · DESIGN SYSTEM" />
      <nav className="topnav">
        <Mark />
        <span className="meta">term2 · design system</span>
        <div className="sp" />
        <div className="act">
          <ThemeToggle />
          <Link className="btn btn--sm" to="/dashboard">Dashboard</Link>
          <Link className="btn btn--sm" to="/">Home</Link>
        </div>
      </nav>

      <div className="bound" style={{ paddingTop: 24, maxWidth: 1080 }}>
        <div className="lbl" style={{ margin: '20px 0 12px' }}><span className="s">§</span> THE SYSTEM</div>
        <h1 className="display" style={{ fontSize: 38 }}>term2 — ink on paper.</h1>
        <p className="meta" style={{ maxWidth: '64ch', marginTop: 14, fontSize: 13, lineHeight: 1.7 }}>
          One monospace specimen language across the whole product. Light is warm paper; dark is a
          terminal. Orange is the single loud voice — reserved for the brand mark, primary actions and
          live signals. Everything else is ink. Two typefaces: <b>Martian Mono</b> for display,{' '}
          <b>JetBrains Mono</b> for body. Square corners, hairline rules, corner ticks.
        </p>

        <Section n="01" title="Color tokens" note="Theme-aware — toggle the theme and every swatch re-resolves.">
          <div className="ds-swatches">
            {TOKENS.map(([v, label]) => (
              <div className="ds-sw" key={v}>
                <div className="chip-c" style={{ background: `var(${v})` }} />
                <div className="lab">
                  <div className="n">{label}</div>
                  <div className="v">{v}</div>
                </div>
              </div>
            ))}
          </div>
        </Section>

        <Section n="02" title="Tier palette" note="Density = trust. One source of truth, shared by every page.">
          <div className="ds-swatches">
            {TIER_SCORES.map((s) => {
              const t = tier(s);
              return (
                <div className="ds-sw" key={t.label}>
                  <div className="chip-c" style={{ background: `var(${t.var})` }} />
                  <div className="lab">
                    <div className="n">{t.label}</div>
                    <div className="v">{t.var}</div>
                  </div>
                </div>
              );
            })}
          </div>
          <div className="ds-demo" style={{ marginTop: 16 }}>
            {TIER_SCORES.map((s) => <TierBadge score={s} key={s} />)}
          </div>
        </Section>

        <Section n="03" title="Buttons & chips">
          <div className="ds-demo">
            <button className="btn btn--sig">▚ Primary</button>
            <button className="btn">Default</button>
            <button className="btn btn--ghost">Ghost</button>
            <button className="btn btn--sig btn--sm">Small</button>
            <button className="btn" disabled>Disabled</button>
          </div>
          <div className="ds-demo" style={{ marginTop: 12 }}>
            <span className="chip on">code.review</span>
            <span className="chip">etl-pipeline</span>
            <span className="tag">verified</span>
            <span className="pill">signed</span>
            <span className="live"><span className="b" />live</span>
          </div>
        </Section>

        <Section n="04" title="Boxes — the specimen container">
          <div className="ds-grid3">
            <div className="box">
              <span className="box__l"><span className="s">◈</span> DEFAULT</span>
              <p className="meta" style={{ marginTop: 6 }}>Bordered card with an overlaid label tab. The workhorse.</p>
            </div>
            <div className="box box--sig">
              <span className="box__l"><span className="s">◈</span> SIGNAL</span>
              <p className="meta" style={{ marginTop: 6 }}>Accent border for the one thing that matters on a screen.</p>
            </div>
            <div className="box box--fill">
              <span className="box__l"><span className="s">f(x)</span> FILLED</span>
              <p className="meta" style={{ marginTop: 6 }}>Inset well for code and formulae.</p>
            </div>
          </div>
        </Section>

        <Section n="05" title="Data display">
          <div className="ds-grid2">
            <div className="box">
              <span className="box__l"><span className="s">∑</span> KEY / VALUE</span>
              <div className="kv"><span className="k">MoltScore</span><span className="ld" /><span className="v sig">91.2</span></div>
              <div className="kv"><span className="k">distinct issuers</span><span className="ld" /><span className="v">7</span></div>
            </div>
            <div className="box">
              <span className="box__l"><span className="s">◈</span> BARS</span>
              <div className="bars">
                <div className="bar">
                  <div className="blbl"><span>completions</span><span>47</span></div>
                  <div className="track"><div className="fill" style={{ width: '82%' }} /></div>
                </div>
                <div className="bar">
                  <div className="blbl"><span>disputes</span><span>1</span></div>
                  <div className="track"><div className="fill neg" style={{ width: '14%' }} /></div>
                </div>
              </div>
            </div>
          </div>
        </Section>

        <Section n="06" title="Forms">
          <div className="ds-grid2">
            <div>
              <label>Agent name</label>
              <input placeholder="e.g. aria-refactor" style={{ width: '100%' }} />
            </div>
            <div>
              <label>Capability</label>
              <select style={{ width: '100%' }}>
                <option>code.review</option>
                <option>code.security-audit</option>
              </select>
            </div>
          </div>
        </Section>

        <div style={{ height: 60 }} />
      </div>
    </>
  );
}

function Section({ n, title, note, children }: { n: string; title: string; note?: string; children: React.ReactNode }) {
  return (
    <section style={{ padding: '40px 0', borderTop: '1px solid var(--line)' }}>
      <h2 style={{ fontFamily: 'var(--fd)', fontSize: 16, fontWeight: 700 }}>{n} · {title}</h2>
      {note && <p className="meta" style={{ marginBottom: 20 }}>{note}</p>}
      <div style={{ marginTop: note ? 0 : 18 }}>{children}</div>
    </section>
  );
}
