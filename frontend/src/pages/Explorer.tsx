// pages/Explorer.tsx — the public registry browser. No session needed: reads
// are open, which is the point of an open registry.
import { useEffect, useState, useCallback } from 'react';
import { Link } from 'react-router-dom';
import { api, type Agent, type Attestation, type Card, type ScoreOutput } from '../lib/api';
import { Spine, ThemeToggle, Mark } from '../components/Chrome';
import { TierBadge } from '../components/Tier';
import { tier } from '../lib/tier';

interface Detail {
  card: Card;
  score: ScoreOutput;
  atts: Attestation[];
}

export function Explorer() {
  const [q, setQ] = useState('');
  const [cap, setCap] = useState('');
  const [minScore, setMinScore] = useState(0);
  const [tags, setTags] = useState<string[]>([]);
  const [results, setResults] = useState<Agent[]>([]);
  const [total, setTotal] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.taxonomy().then((t) => setTags(t.tags || [])).catch(() => setTags([]));
  }, []);

  const search = useCallback(async () => {
    setLoading(true);
    try {
      const d = await api.search({ q, cap, min_score: minScore, limit: 50 });
      setResults(d.results || []);
      setTotal(d.total ?? 0);
    } catch {
      setResults([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [q, cap, minScore]);

  // debounce free-text; fire immediately for the select/range filters
  useEffect(() => {
    const t = setTimeout(search, 180);
    return () => clearTimeout(t);
  }, [search]);

  async function select(did: string) {
    setSelected(did);
    setDetail(null);
    try {
      const [a, at] = await Promise.all([api.agent(did), api.attestations(did)]);
      setDetail({ card: a.card, score: a.score, atts: at.attestations || [] });
    } catch {
      setDetail(null);
    }
  }

  return (
    <>
      <Spine middle="REGISTRY EXPLORER" />
      <nav className="topnav">
        <Mark />
        <span className="meta">registry explorer</span>
        <div className="sp" />
        <div className="act">
          <ThemeToggle />
          <Link className="btn btn--sm" to="/graph">Graph</Link>
          <Link className="btn btn--sm" to="/">Home</Link>
        </div>
      </nav>

      <div className="bound" style={{ paddingTop: 24 }}>
        <h1 className="display" style={{ fontSize: 24, margin: '18px 0 4px' }}>Registry Explorer</h1>
        <div className="meta" style={{ marginBottom: 18 }}>
          Live view of registered agents. Every score is recomputable from the raw attestation chain.
        </div>

        <div className="row g12 wrap" style={{ marginBottom: 20 }}>
          <input
            style={{ flex: 1, minWidth: 220 }}
            placeholder="search by name, capability, description…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <select value={cap} onChange={(e) => setCap(e.target.value)}>
            <option value="">any capability</option>
            {tags.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          <select value={minScore} onChange={(e) => setMinScore(Number(e.target.value))}>
            <option value={0}>min score: any</option>
            <option value={30}>30+</option>
            <option value={50}>50+</option>
            <option value={70}>70+</option>
            <option value={85}>85+ elite</option>
          </select>
        </div>

        <div className="ex-layout">
          <div className="box">
            <span className="box__l">
              <span className="s">▤</span> AGENTS · {loading ? '…' : `${results.length} of ${total}`}
            </span>
            {!loading && !results.length && <div className="empty">no agents match</div>}
            {results.map((a) => (
              <button
                key={a.id}
                className={'ex-row' + (selected === a.id ? ' sel' : '')}
                onClick={() => select(a.id)}
              >
                <div className="row between" style={{ gap: 10 }}>
                  <span className="nm">{a.name || 'unnamed'}</span>
                  <span className="row g8">
                    <TierBadge score={a.score} />
                    <span className="sc">{(a.score || 0).toFixed(1)}</span>
                  </span>
                </div>
                <div className="did">{a.id}</div>
                <div className="caps">
                  {(a.capabilities || []).map((c) => (
                    <span className="tag" key={c}>{c}</span>
                  ))}
                </div>
              </button>
            ))}
          </div>

          <div className="box" style={{ alignSelf: 'start' }}>
            <span className="box__l"><span className="s">◈</span> AGENT DETAIL</span>
            {!selected && <div className="empty">select an agent to inspect its card, score and chain.</div>}
            {selected && !detail && <div className="empty">loading…</div>}
            {detail && <AgentDetail did={selected!} d={detail} />}
          </div>
        </div>
        <div style={{ height: 60 }} />
      </div>
    </>
  );
}

function AgentDetail({ did, d }: { did: string; d: Detail }) {
  const t = tier(d.score?.score || 0);
  const inp = d.score?.inputs;
  const max = Math.max(inp?.completions || 0, 1);
  const bar = (label: string, v: number, neg = false) => (
    <div className="bar" key={label}>
      <div className="blbl"><span>{label}</span><span>{v || 0}</span></div>
      <div className="track">
        <div className={'fill' + (neg ? ' neg' : '')} style={{ width: `${Math.min(100, ((v || 0) / max) * 100)}%` }} />
      </div>
    </div>
  );

  return (
    <div style={{ paddingTop: 6 }}>
      <h2 style={{ fontFamily: 'var(--fd)', fontSize: 17 }}>{d.card.name || 'unnamed'}</h2>
      <div className="meta" style={{ margin: '2px 0 12px' }}>{d.card.description}</div>

      <div className="row g14" style={{ alignItems: 'flex-end', marginBottom: 12 }}>
        <span style={{ fontFamily: 'var(--fd)', fontWeight: 800, fontSize: 40, color: 'var(--ac)', lineHeight: 1 }}>
          {(d.score?.score || 0).toFixed(1)}
        </span>
        <span style={{ color: 'var(--ink-4)' }}>/100</span>
        <span className={'tier ' + t.cls}>{t.label}</span>
      </div>

      <div className="caps" style={{ marginBottom: 12 }}>
        {(d.card.capabilities || []).map((c) => (
          <span className="tag" key={c.tag}>{c.tag}</span>
        ))}
      </div>

      <div className="bars">
        {bar('completions', inp?.completions || 0)}
        {bar('distinct issuers', inp?.distinct_issuers || 0)}
        {bar('disputes', inp?.disputes || 0, true)}
        {bar('incidents', inp?.incidents || 0, true)}
      </div>

      <div className="kv"><span className="k">owner</span><span className="ld" /><span className="v">{(d.card.owner || '').slice(0, 28)}…</span></div>
      <div className="kv"><span className="k">version</span><span className="ld" /><span className="v">{d.card.version || '—'}</span></div>

      <div className="sub"><span className="n">▸</span><h3>Attestation chain · {d.atts.length}</h3><span className="ln" /></div>
      {!d.atts.length && <div className="empty">no attestations yet</div>}
      {d.atts.slice(0, 8).map((a, i) => (
        <div className="att" key={i}>
          <span className="t">{a.type}</span> <span className="pill">signed</span>
          <div className="meta" style={{ fontSize: 11 }}>
            issuer {(a.issuer || '').slice(0, 24)}… · {(a.issued_at || '').slice(0, 10)}
          </div>
        </div>
      ))}

      <Link className="btn btn--sig btn--sm" style={{ marginTop: 14 }} to={`/profile/${encodeURIComponent(did)}`}>
        Open full profile · verify locally →
      </Link>
    </div>
  );
}
