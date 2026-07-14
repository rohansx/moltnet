// pages/Profile.tsx — an agent's public record, plus the flagship: verify the
// whole thing in your own browser. Signatures are re-checked with WebCrypto and
// MoltScore is recomputed locally, so the registry is trusted only to move bytes.
import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { api, type Attestation, type Card, type ScoreOutput } from '../lib/api';
import { Spine, ThemeToggle, Mark } from '../components/Chrome';
import { tier } from '../lib/tier';
import { verifyAgent, UNIFORM_WEIGHT_NOTE, type VerifyStep } from '../lib/verify';

export function Profile() {
  const { did = '' } = useParams();
  const [card, setCard] = useState<Card | null>(null);
  const [score, setScore] = useState<ScoreOutput | null>(null);
  const [atts, setAtts] = useState<Attestation[]>([]);
  const [error, setError] = useState('');
  const [steps, setSteps] = useState<VerifyStep[] | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!did) return;
    let live = true;
    (async () => {
      try {
        const [a, at] = await Promise.all([api.agent(did), api.attestations(did)]);
        if (!live) return;
        setCard(a.card);
        setScore(a.score);
        setAtts(at.attestations || []);
      } catch (e) {
        if (live) setError(e instanceof Error ? e.message : 'agent not found');
      }
    })();
    return () => {
      live = false;
    };
  }, [did]);

  async function runVerify() {
    if (!card) return;
    setVerifying(true);
    setSteps([{ state: 'pend', text: 'verifying locally…' }]);
    const out = await verifyAgent(card, atts);
    setSteps(out);
    setVerifying(false);
  }

  function copyDID() {
    navigator.clipboard?.writeText(did);
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  }

  if (error) {
    return (
      <>
        <Spine middle="AGENT PROFILE" />
        <div className="bound"><div className="empty" style={{ marginTop: 60 }}>{error}</div></div>
      </>
    );
  }
  if (!card || !score) {
    return (
      <>
        <Spine middle="AGENT PROFILE" />
        <div className="bound"><div className="empty" style={{ marginTop: 60 }}>loading…</div></div>
      </>
    );
  }

  const t = tier(score.score || 0);
  const inp = score.inputs;
  const max = Math.max(inp?.completions || 0, 1);
  const badge = `/v1/agents/${encodeURIComponent(did)}/badge.svg`;
  const md = `[![MoltScore](${location.origin}${badge})](${location.origin}/profile/${encodeURIComponent(did)})`;

  const bar = (label: string, v: number, neg = false) => (
    <div className="bar" key={label}>
      <div className="blbl"><span>{label}</span><span>{v || 0}</span></div>
      <div className="track">
        <div className={'fill' + (neg ? ' neg' : '')} style={{ width: `${Math.min(100, ((v || 0) / max) * 100)}%` }} />
      </div>
    </div>
  );

  return (
    <>
      <Spine middle="AGENT PROFILE" />
      <nav className="topnav">
        <Mark />
        <span className="meta">agent profile</span>
        <div className="sp" />
        <div className="act">
          <ThemeToggle />
          <Link className="btn btn--sm" to="/explorer">Explorer</Link>
          <Link className="btn btn--sm" to="/">Home</Link>
        </div>
      </nav>

      <div className="bound" style={{ paddingTop: 24, maxWidth: 960 }}>
        <div className="pf-hero">
          <div>
            <h1 className="display" style={{ fontSize: 28 }}>{card.name || 'unnamed agent'}</h1>
            <div style={{ color: 'var(--ink-2)', marginTop: 6, maxWidth: '60ch' }}>{card.description}</div>
            <div className="row g8" style={{ marginTop: 12, fontSize: 12, color: 'var(--ink-4)' }}>
              <code style={{ wordBreak: 'break-all' }}>{did}</code>
              <button className="copy" onClick={copyDID}>{copied ? 'copied' : 'copy'}</button>
            </div>
          </div>
          <div style={{ textAlign: 'right', minWidth: 180 }}>
            <div>
              <span style={{ fontFamily: 'var(--fd)', fontWeight: 800, fontSize: 56, color: 'var(--ac)', lineHeight: 1 }}>
                {(score.score || 0).toFixed(1)}
              </span>
              <span style={{ color: 'var(--ink-4)', fontSize: 18 }}>/100</span>
            </div>
            <span className={'tier ' + t.cls} style={{ marginTop: 6 }}>{t.label} · {score.algorithm}</span>
          </div>
        </div>

        {/* the flagship */}
        <div className="box" style={{ margin: '20px 0' }}>
          <span className="box__l"><span className="s">▚</span> VERIFY IN YOUR BROWSER</span>
          <div className="row between wrap g12" style={{ alignItems: 'center' }}>
            <div className="meta" style={{ maxWidth: '58ch' }}>
              Re-checks every signature with WebCrypto and recomputes MoltScore locally from the raw
              chain — <b>no trust is placed in this server</b>.
            </div>
            <button className="btn btn--sig" onClick={runVerify} disabled={verifying}>
              ▚ {verifying ? 'Verifying…' : 'Verify locally'}
            </button>
          </div>
          {steps && (
            <>
              <div className="vsteps">
                {steps.map((s, i) => (
                  <div className="vstep" key={i}>
                    <span className={'s ' + s.state}>{s.state === 'ok' ? '✓' : s.state === 'bad' ? '✕' : '•'}</span>
                    <span>{s.text}</span>
                  </div>
                ))}
              </div>
              {!verifying && (
                <div className="meta" style={{ marginTop: 10, fontSize: 11, lineHeight: 1.6, borderTop: '1px dotted var(--line-2)', paddingTop: 10 }}>
                  {UNIFORM_WEIGHT_NOTE}
                </div>
              )}
            </>
          )}
        </div>

        <div className="pf-grid">
          <div className="box">
            <span className="box__l"><span className="s">f(x)</span> SCORE BREAKDOWN</span>
            <div className="bars">
              {bar('completions', inp?.completions || 0)}
              {bar('endorsements', inp?.endorsements || 0)}
              {bar('payment receipts', inp?.receipts || 0)}
              {bar('distinct issuers', inp?.distinct_issuers || 0)}
              {bar('disputes', inp?.disputes || 0, true)}
              {bar('incidents', inp?.incidents || 0, true)}
            </div>
          </div>
          <div className="box">
            <span className="box__l"><span className="s">◈</span> IDENTITY &amp; BINDINGS</span>
            <div className="kv"><span className="k">owner</span><span className="ld" /><span className="v">{(card.owner || '').slice(0, 30)}…</span></div>
            <div className="kv"><span className="k">version</span><span className="ld" /><span className="v">{card.version || '—'}</span></div>
            <div className="kv"><span className="k">created</span><span className="ld" /><span className="v">{card.created_at}</span></div>
            <div className="kv"><span className="k">capabilities</span><span className="ld" /><span className="v">{(card.capabilities || []).length}</span></div>
            <div className="caps" style={{ marginTop: 10 }}>
              {(card.capabilities || []).map((c) => (
                <span className="tag" key={c.tag}>{c.tag}</span>
              ))}
            </div>
          </div>
        </div>

        <div className="box" style={{ marginTop: 16 }}>
          <span className="box__l"><span className="s">▦</span> EMBEDDABLE BADGE</span>
          <img src={badge} alt="MoltScore badge" style={{ marginTop: 8 }} />
          <div className="meta" style={{ marginTop: 10, wordBreak: 'break-all', fontSize: 11 }}>{md}</div>
        </div>

        <div className="box" style={{ marginTop: 16 }}>
          <span className="box__l"><span className="s">▚</span> ATTESTATION CHAIN · {atts.length}</span>
          {!atts.length && <div className="empty">no attestations yet</div>}
          {atts.map((a, i) => (
            <div className="att" key={i}>
              <span className="t">{a.type}</span> <span className="pill">signed</span>
              <div className="meta" style={{ fontSize: 11 }}>
                issuer {(a.issuer || '').slice(0, 30)}… · {a.issued_at}
                {a.body?.capability ? ` · ${a.body.capability}` : ''}
              </div>
            </div>
          ))}
        </div>
        <div style={{ height: 60 }} />
      </div>
    </>
  );
}
