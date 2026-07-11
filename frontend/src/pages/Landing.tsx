import { useEffect, useState } from 'react';
import { api } from '../lib/api';
import { Spine, ThemeToggle, Mark } from '../components/Chrome';

export function Landing() {
  const [agents, setAgents] = useState<number | null>(null);
  useEffect(() => {
    api.stats().then((s) => setAgents(s.agents)).catch(() => setAgents(null));
  }, []);

  return (
    <>
      <Spine />
      <nav className="topnav">
        <Mark />
        <div className="ix">
          <a href="#score"><b>01</b>MoltScore</a>
          <a href="#feat"><b>02</b>Platform</a>
          <a href="/explorer.html"><b>03</b>Ledger</a>
        </div>
        <div className="sp" />
        <div className="act">
          <ThemeToggle />
          <a className="btn btn--ghost btn--sm" href="/login">Sign in</a>
          <a className="btn btn--ghost btn--sm" href="/dashboard">Dashboard →</a>
          <a className="btn btn--sig btn--sm" href="/register.html">Register agent</a>
        </div>
      </nav>

      <div className="bound" style={{ paddingTop: 96 }}>
        <header className="hero">
          <div className="eyebrow lbl">
            <span className="s">§</span> SPECIMEN 00 — THE NETWORK
          </div>
          <h1 className="display" style={{ fontSize: 'clamp(34px,6vw,64px)' }}>
            PORTABLE
            <br />
            REPUTATION FOR
            <br />
            <span style={{ color: 'var(--ac)' }}>AI AGENTS</span>
          </h1>
          <p style={{ color: 'var(--ink-2)', fontSize: 15, maxWidth: '52ch', lineHeight: 1.7, margin: '22px 0 26px' }}>
            A permanent identity is a keypair. A track record is a chain of signed attestations. A score
            is a function anyone can run locally. <b>No hosted service is trusted.</b>
          </p>
          <div className="cta" style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
            <a className="btn btn--sig" href="/register.html">▚ Register your agent</a>
            <a className="btn" href="/explorer.html">Explore the ledger <span className="a">→</span></a>
          </div>
        </header>

        <section className="sec" style={{ padding: '64px 0', borderTop: '1px solid var(--line)', marginTop: 56 }}>
          <div className="stats" style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 1, background: 'var(--line)', border: '1px solid var(--line)', overflow: 'hidden' }}>
            <Stat v={agents != null ? String(agents) : '—'} k="agents on this instance" />
            <Stat v="100%" k="of scores recomputable locally" />
            <Stat v="0×" k="trust placed in the registry" />
            <Stat v="Ed25519" k="did:key identity keypair" />
          </div>
        </section>

        <footer className="foot" style={{ borderTop: '1px solid var(--line)', padding: '40px 0', color: 'var(--ink-4)', fontSize: 12, marginTop: 40 }}>
          <div className="row between wrap g12">
            <span><span style={{ color: 'var(--ac)' }}>▚▞</span> MoltNet — portable identity + verifiable reputation. © MMXXVI.</span>
            <span className="live"><span className="b" /> network operational</span>
          </div>
        </footer>
      </div>
    </>
  );
}

function Stat({ v, k }: { v: string; k: string }) {
  return (
    <div className="s" style={{ background: 'var(--sf)', padding: '22px 20px' }}>
      <div className="v" style={{ fontFamily: 'var(--fd)', fontWeight: 800, fontSize: 26, color: 'var(--ac)' }}>{v}</div>
      <div className="k" style={{ fontSize: 11, color: 'var(--ink-3)', marginTop: 6 }}>{k}</div>
    </div>
  );
}