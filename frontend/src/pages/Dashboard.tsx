import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from '../lib/auth';
import { api, type Agent, type GraphData, type Attestation, type APIKey } from '../lib/api';
import { tier, gauge } from '../lib/tier';
import { TierBadge, TierGlyph } from '../components/Tier';
import { ThemeToggle } from '../components/Chrome';
import '../styles/dashboard.css';

type View = 'mine' | 'overview' | 'discovery' | 'genome' | 'marketplace' | 'swarm' | 'streams' | 'alignment';

const NAV: { view: View; title: string; sub: string; no: string; soon?: boolean }[] = [
  { view: 'mine', title: 'My Agents', sub: 'agents you own', no: '01' },
  { view: 'overview', title: 'Overview', sub: 'network command center', no: '02' },
  { view: 'discovery', title: 'Discovery', sub: 'search the ledger by capability + score', no: '03' },
  { view: 'genome', title: 'Living Genome', sub: 'temporal collaboration graph', no: '04' },
  { view: 'marketplace', title: 'Marketplace', sub: 'task board · escrow · multi-rail', no: '05', soon: true },
  { view: 'swarm', title: 'Swarm', sub: 'multi-agent orchestration', no: '06', soon: true },
  { view: 'streams', title: 'Decision Streams', sub: 'real-time reasoning transparency', no: '07', soon: true },
  { view: 'alignment', title: 'Alignment', sub: 'continuous adversarial audit', no: '08', soon: true },
];

export function Dashboard() {
  const { owner, agents, signOut } = useAuth();
  const [view, setView] = useState<View>('mine');
  const [online, setOnline] = useState<number | null>(null);
  const current = NAV.find((n) => n.view === view)!;

  useEffect(() => {
    api.stats().then((s) => setOnline(s.agents)).catch(() => setOnline(null));
  }, []);

  const short = owner ? owner.replace('did:key:z', '').slice(0, 2).toUpperCase() : '··';

  return (
    <div className="app">
      <aside className="rail">
        <a className="mk" href="/">
          <span className="gx">▚▞</span>
          <span className="nm">MoltNet</span>
        </a>
        <div className="rgrp">
          <div className="gl">Workspace · live</div>
          {NAV.filter((n) => !n.soon).map((n) => (
            <RailNav key={n.view} n={n} active={view === n.view} onClick={() => setView(n.view)} />
          ))}
        </div>
        <div className="rgrp">
          <div className="gl">Roadmap · preview</div>
          {NAV.filter((n) => n.soon).map((n) => (
            <RailNav key={n.view} n={n} active={view === n.view} onClick={() => setView(n.view)} />
          ))}
        </div>
        <div className="rme">
          <span className="av">{short}</span>
          <div className="grow">
            <div className="h">owner</div>
            <div className="m">
              <span className="sig">▚</span> {owner ? owner.replace('did:key:z', '').slice(0, 12) + '…' : '—'}
            </div>
          </div>
          <button className="btn btn--ghost btn--sm" title="Sign out" style={{ padding: '4px 7px', fontSize: 11 }} onClick={signOut}>
            ⏻
          </button>
        </div>
      </aside>

      <div className="main">
        <header className="top">
          <div className="head">
            <div className="tt">{current.title}</div>
            <div className="ts">{current.sub}</div>
          </div>
          <div className="stat">
            <span className="live">
              <span className="b" />
            </span>{' '}
            <span>{online ?? '—'}</span> agents
          </div>
          <div className="act row g8">
            <ThemeToggle />
            <Link className="btn btn--sig btn--sm" to="/register">
              + Register
            </Link>
          </div>
        </header>
        <div className="area">
          {view === 'mine' && <MyAgents agents={agents} owner={owner || ''} />}
          {view === 'overview' && <Overview />}
          {view === 'discovery' && <Discovery />}
          {view === 'genome' && <Genome />}
          {view === 'marketplace' && <Preview title="Marketplace" desc="task board, escrow and multi-rail payments are on the roadmap — the v0.1 core ships identity + reputation only." />}
          {view === 'swarm' && <Preview title="Swarm Composer" desc="swarm composition and orchestration are on the roadmap." />}
          {view === 'streams' && <Preview title="Decision Streams" desc="decision streams over WebSocket are on the roadmap." />}
          {view === 'alignment' && <Preview title="Alignment Oracle" desc="daily adversarial audits are on the roadmap." />}
        </div>
      </div>
    </div>
  );
}

function RailNav({ n, active, onClick }: { n: { no: string; title: string; soon?: boolean }; active: boolean; onClick: () => void }) {
  return (
    <div className={`rnav ${active ? 'active' : ''}`} onClick={onClick}>
      <span className="no">{n.no}</span>
      <span>{n.title}</span>
      {n.soon && <span className="prev">SOON</span>}
    </div>
  );
}

function Preview({ title, desc }: { title: string; desc: string }) {
  return (
    <>
      <div className="pvw">
        <b>PREVIEW</b> {desc}
      </div>
      <div className="box" style={{ marginTop: 16 }}>
        <span className="box__l">
          <span className="s">◓</span> {title.toUpperCase()}
        </span>
        <div className="faint" style={{ fontSize: 12, padding: '8px 0' }}>
          Data for this view is illustrative in v0.1.
        </div>
      </div>
    </>
  );
}

function MyAgents({ agents, owner }: { agents: Agent[]; owner: string }) {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [keyAgent, setKeyAgent] = useState('');
  const [keyName, setKeyName] = useState('');
  const [newKey, setNewKey] = useState('');
  const [err, setErr] = useState('');

  async function loadKeys() {
    try {
      const r = await api.listKeys();
      setKeys(r.keys || []);
    } catch {
      setKeys([]);
    }
  }
  useEffect(() => {
    loadKeys();
  }, []);
  useEffect(() => {
    if (agents.length && !keyAgent) setKeyAgent(agents[0].id);
  }, [agents]);

  async function mint() {
    setErr('');
    if (!keyAgent) {
      setErr('Register an agent first.');
      return;
    }
    try {
      const r = await api.createKey(keyAgent, keyName || 'default');
      setNewKey(r.key);
      setKeyName('');
      loadKeys();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }

  async function revoke(id: string) {
    if (!confirm('Revoke this API key? Programmatic clients using it lose access immediately.')) return;
    try {
      await api.revokeKey(id);
      loadKeys();
    } catch (e) {
      alert('Revoke failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return (
    <>
      <div className="mine-head">
        <div style={{ fontFamily: 'var(--fd)', fontWeight: 800, fontSize: 15 }}>Signed in as owner</div>
        <div className="faint" style={{ fontSize: 11, marginTop: 4, wordBreak: 'break-all' }}>
          {owner}
        </div>
        <div className="row g8 wrap" style={{ marginTop: 16, marginBottom: 20 }}>
          <Link className="btn btn--sig btn--sm" to="/register">
            + Register a new agent
          </Link>
          <span className="faint" style={{ fontSize: 11, alignSelf: 'center' }}>
            keys generated in your browser · card signed locally
          </span>
        </div>
      </div>

      <SectionHead>Your agents {agents.length ? `(${agents.length})` : ''}</SectionHead>
      {agents.length === 0 ? (
        <div className="box" style={{ padding: 24, textAlign: 'center', color: 'var(--ink-4)', fontSize: 12 }}>
          You have no agents yet. <Link to="/register" style={{ color: 'var(--ac)' }}>Register one</Link> — your owner key is what signs you in here.
        </div>
      ) : (
        <div className="mine-list">
          {agents.map((a) => {
            const t = tier(a.score || 0);
            return (
              <div className="mine-card" key={a.id}>
                <div>
                  <div className="row between">
                    <span className="nm" style={{ fontFamily: 'var(--fd)', fontWeight: 700, fontSize: 14 }}>
                      {a.name || 'unnamed'}
                    </span>
                    <span className="sc" style={{ fontFamily: 'var(--fd)', fontWeight: 800, color: 'var(--ac)', fontSize: 15 }}>
                      {(a.score || 0).toFixed(1)}
                    </span>
                  </div>
                  <div className="did" style={{ color: 'var(--ink-4)', fontSize: 10, marginTop: 4, wordBreak: 'break-all' }}>
                    {a.id}
                  </div>
                  <div className="caps" style={{ marginTop: 8, display: 'flex', gap: 5, flexWrap: 'wrap' }}>
                    {(a.capabilities || []).map((c) => (
                      <span className="tag" key={c}>{c}</span>
                    ))}
                  </div>
                </div>
                <div className="acts" style={{ display: 'flex', flexDirection: 'column', gap: 6, alignItems: 'flex-end' }}>
                  <span className={`tier ${t.cls}`}>{t.label}</span>
                  <a className="btn btn--ghost btn--sm" href={`/profile/${encodeURIComponent(a.id)}`}>
                    Profile →
                  </a>
                </div>
              </div>
            );
          })}
        </div>
      )}

      <SectionHead>
        API keys <span className="faint">per-agent · programmatic access</span>
      </SectionHead>
      <div className="box" style={{ marginBottom: 16 }}>
        <div className="row g10 wrap" style={{ alignItems: 'flex-end' }}>
          <div className="grow" style={{ minWidth: 200 }}>
            <div className="fl" style={{ fontSize: 10, letterSpacing: 1, color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 8 }}>
              Agent
            </div>
            <select value={keyAgent} onChange={(e) => setKeyAgent(e.target.value)} style={{ width: '100%' }}>
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name || a.id}
                </option>
              ))}
            </select>
          </div>
          <div className="grow" style={{ minWidth: 160 }}>
            <div className="fl" style={{ fontSize: 10, letterSpacing: 1, color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 8 }}>
              Label
            </div>
            <input value={keyName} onChange={(e) => setKeyName(e.target.value)} placeholder="e.g. prod-bot" style={{ width: '100%' }} />
          </div>
          <button className="btn btn--sig btn--sm" onClick={mint}>
            Mint key
          </button>
        </div>
        <div className="faint" style={{ fontSize: 11, marginTop: 12, lineHeight: 1.6 }}>
          A key authenticates a programmatic client (CLI, MCP, agent runtime) to read its own state. It is <b>shown once</b> — copy it now. It never authorizes signed writes; those stay signature-authenticated.
        </div>
      </div>

      {newKey && (
        <div className="key-new">
          <div className="lbl" style={{ fontSize: 10, letterSpacing: 1, color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 6 }}>
            ✓ new API key — copy now, shown once
          </div>
          <div className="k" style={{ fontFamily: 'var(--fm)', fontSize: 11, color: 'var(--ac)', wordBreak: 'break-all', userSelect: 'all' }}>
            {newKey}
          </div>
          <div className="row g8" style={{ marginTop: 10 }}>
            <button
              className="btn btn--ghost btn--sm"
              onClick={() => {
                navigator.clipboard?.writeText(newKey);
              }}
            >
              Copy
            </button>
          </div>
        </div>
      )}
      {err && <div style={{ color: 'var(--neg)', fontSize: 11.5, marginTop: 8 }}>⚠ {err}</div>}

      {keys.length === 0 ? (
        <div className="faint" style={{ fontSize: 11, padding: '8px 0' }}>
          No API keys minted.
        </div>
      ) : (
        <div>
          {keys.map((k) => (
            <div className={`keyrow ${k.revoked_at ? 'revoked' : ''}`} key={k.id}>
              <div>
                <div className="pf" style={{ fontFamily: 'var(--fd)', color: 'var(--ink-2)' }}>
                  {k.prefix}••••{k.last4}
                </div>
                <div className="ag" style={{ color: 'var(--ink-4)', fontSize: 10 }}>
                  {(agents.find((a) => a.id === k.agent_did)?.name) || (k.agent_did || '').slice(0, 16) + '…'}
                  {k.name ? ' · ' + k.name : ''}
                  {k.revoked_at ? ' · revoked' : ''}
                </div>
              </div>
              <span className="faint" style={{ fontSize: 10 }}>{(k.created_at || '').slice(0, 10)}</span>
              {k.revoked_at ? (
                <span className="faint" style={{ fontSize: 10 }}>—</span>
              ) : (
                <button className="btn btn--ghost btn--sm" onClick={() => revoke(k.id)}>
                  Revoke
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </>
  );
}

function SectionHead({ children }: { children: React.ReactNode }) {
  return (
    <div className="sub">
      <span className="n">▸</span>
      <h3 style={{ fontFamily: 'var(--fd)', fontSize: 13, fontWeight: 700 }}>{children}</h3>
      <span className="ln" />
    </div>
  );
}

function Overview() {
  const [agent, setAgent] = useState<Agent | null>(null);
  const [card, setCard] = useState<{ name?: string } | null>(null);
  const [score, setScore] = useState<{ score: number; algorithm?: string; inputs?: { completions: number; distinct_issuers: number; endorsements: number; receipts: number; disputes: number; incidents: number } } | null>(null);
  const [atts, setAtts] = useState<Attestation[]>([]);

  useEffect(() => {
    api.search({ limit: 1 }).then((r) => {
      const a = (r.results || [])[0];
      if (!a) return;
      setAgent(a);
      api.agent(a.id).then((d) => {
        setCard(d.card);
        setScore(d.score);
      });
      api.attestations(a.id).then((d) => setAtts(d.attestations || []));
    });
  }, []);

  if (!agent) return <div className="empty">no agents yet</div>;
  const t = tier(score?.score ?? agent.score ?? 0);
  const inp = score?.inputs;

  return (
    <>
      <div className="ov">
        <div className="ovcol">
          <div className="box box--sig score">
            <span className="box__l">
              <span className="s">◈</span> MOLTSCORE
            </span>
            <span className="box__r">{score?.algorithm || 'moltscore/v1'}</span>
            <div>
              <span className="big" style={{ fontFamily: 'var(--fd)', fontWeight: 800, fontSize: 60, letterSpacing: -2, lineHeight: 1 }}>
                {(score?.score ?? agent.score ?? 0).toFixed(1)}
              </span>
              <span className="of" style={{ color: 'var(--ink-4)', fontSize: 16 }}> /100</span>
            </div>
            <TierBadge score={score?.score ?? agent.score ?? 0} />
            <div className="gz" style={{ marginTop: 14 }} dangerouslySetInnerHTML={{ __html: gauge(score?.score ?? agent.score ?? 0, 28) }} />
            <div className="dl" style={{ fontSize: 11, color: 'var(--ink-3)', marginTop: 10 }}>
              {inp?.completions ?? 0} completions · {inp?.distinct_issuers ?? 0} distinct issuers
            </div>
            <div className="ch" style={{ fontSize: 10, color: 'var(--ink-4)', marginTop: 6 }}>⛓ recomputable from the raw chain</div>
          </div>
          <div className="box">
            <span className="box__l">
              <span className="s">f(x)</span> SCORE BREAKDOWN
            </span>
            <div className="bars">
              <Bar label="completions" val={inp?.completions ?? 0} max={Math.max(inp?.completions ?? 0, 5)} />
              <Bar label="endorsements" val={inp?.endorsements ?? 0} max={Math.max(inp?.completions ?? 0, 5)} />
              <Bar label="payment receipts" val={inp?.receipts ?? 0} max={Math.max(inp?.completions ?? 0, 5)} />
              <Bar label="distinct issuers" val={inp?.distinct_issuers ?? 0} max={Math.max(inp?.distinct_issuers ?? 0, 5)} />
              <Bar label="disputes" val={inp?.disputes ?? 0} max={Math.max(inp?.completions ?? 0, 5)} neg />
              <Bar label="incidents" val={inp?.incidents ?? 0} max={Math.max(inp?.completions ?? 0, 5)} neg />
            </div>
          </div>
        </div>
        <div className="ovcol">
          <div className="metrics" style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 10 }}>
            <Metric v={(score?.score ?? agent.score ?? 0).toFixed(1)} k="MoltScore" d={t.label} color={`var(${t.var})`} />
            <Metric v={String(inp?.completions ?? 0)} k="Completions" d="signed tasks" />
            <Metric v={String(inp?.distinct_issuers ?? 0)} k="Issuers" d="distinct" />
          </div>
          <div className="box">
            <span className="box__l">
              <span className="s">▚</span> ATTESTATION TIMELINE
            </span>
            <div className="tl" style={{ display: 'flex', flexDirection: 'column', gap: 2, fontSize: 11 }}>
              {atts.length ? (
                atts.slice(0, 8).map((a, i) => (
                  <div className="e" key={i} style={{ display: 'grid', gridTemplateColumns: 'auto 1fr auto', gap: 8, padding: '5px 0', borderBottom: '1px solid var(--line)' }}>
                    <span className="dot" style={{ color: 'var(--ac)' }}>▚</span>
                    <span>{a.type}{a.body?.capability ? ' · ' + a.body.capability : ''}</span>
                    <span className="tm" style={{ color: 'var(--ink-4)', fontSize: 10 }}>{(a.issued_at || '').slice(0, 10)}</span>
                  </div>
                ))
              ) : (
                <div className="empty">no attestations yet</div>
              )}
            </div>
          </div>
        </div>
      </div>
      <div className="box feed" style={{ marginTop: 16 }}>
        <span className="box__l">
          <span className="s">▦</span> RECENT ATTESTATIONS
        </span>
        {atts.length ? (
          atts.slice(0, 6).map((a, i) => (
            <div className="it" key={i} style={{ display: 'grid', gridTemplateColumns: 'auto 1fr auto', gap: 10, padding: '11px 0', borderTop: '1px solid var(--line)', fontSize: 12, alignItems: 'baseline' }}>
              <span className="ic" style={{ color: 'var(--ac)' }}>◈</span>
              <span>
                <b>{a.type}</b>
                <span className="sub" style={{ color: 'var(--ink-4)', fontSize: 10.5, display: 'block', margin: '2px 0 0' }}>
                  issuer {(a.issuer || '').slice(0, 22)}…{a.body?.outcome ? ' · ' + a.body.outcome : ''}
                </span>
              </span>
              <span className="tm" style={{ color: 'var(--ink-4)', fontSize: 10, whiteSpace: 'nowrap' }}>{(a.issued_at || '').slice(0, 10)}</span>
            </div>
          ))
        ) : (
          <div className="empty">no attestations — issue one with <code>molt attest</code></div>
        )}
        <a className="btn btn--ghost btn--sm" style={{ marginTop: 12 }} href={`/profile/${encodeURIComponent(agent.id)}`}>Open full profile →</a>
      </div>
    </>
  );
}

function Bar({ label, val, max, neg }: { label: string; val: number; max: number; neg?: boolean }) {
  const w = Math.min(100, (val || 0) / Math.max(1, max) * 100);
  return (
    <div className="bar">
      <div className="blbl" style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: 'var(--ink-3)', marginBottom: 3 }}>
        <span>{label}</span>
        <span>{val || 0}</span>
      </div>
      <div className="track" style={{ height: 6, background: 'var(--sf-2)', overflow: 'hidden' }}>
        <div className={`fill${neg ? ' neg' : ''}`} style={{ height: '100%', width: `${w}%`, background: neg ? 'var(--neg)' : 'var(--ac)' }} />
      </div>
    </div>
  );
}

function Metric({ v, k, d, color }: { v: string; k: string; d: string; color?: string }) {
  return (
    <div className="metric" style={{ border: '1px solid var(--line-2)', background: 'var(--sf)', padding: 14 }}>
      <div className="v" style={{ fontFamily: 'var(--fd)', fontWeight: 800, fontSize: 22, letterSpacing: -1, color: color || 'var(--ink)' }}>{v}</div>
      <div className="k" style={{ fontSize: 10, color: 'var(--ink-4)', marginTop: 4 }}>{k}</div>
      <div className="d faint" style={{ fontSize: 10, marginTop: 4 }}>{d}</div>
    </div>
  );
}

function Discovery() {
  const [q, setQ] = useState('');
  const [caps, setCaps] = useState<string[]>([]);
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [minScore, setMinScore] = useState(0);
  const [results, setResults] = useState<Agent[]>([]);
  const [total, setTotal] = useState(0);

  useEffect(() => {
    api.taxonomy().then((d) => setCaps(d.tags || [])).catch(() => {});
  }, []);

  async function run() {
    const capArr = [...chosen];
    try {
      const r = await api.search({ q, cap: capArr[0] || '', min_score: minScore, limit: 30 });
      let rows = r.results || [];
      if (capArr.length > 1) rows = rows.filter((a) => capArr.every((c) => (a.capabilities || []).includes(c)));
      setResults(rows);
      setTotal(r.total ?? rows.length);
    } catch {
      setResults([]);
      setTotal(0);
    }
  }
  useEffect(() => {
    const t = setTimeout(run, 200);
    return () => clearTimeout(t);
  }, [q, chosen, minScore]);

  return (
    <>
      <div className="bigq" style={{ display: 'flex', alignItems: 'center', gap: 12, border: '1px solid var(--line-2)', background: 'var(--sf)', padding: '12px 16px', marginBottom: 20 }}>
        <span style={{ color: 'var(--ac)' }}>⌕</span>
        <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="code review agent, score 70+…" style={{ flex: 1, border: 'none', background: 'none', fontSize: 14 }} />
      </div>
      <div className="disc" style={{ display: 'grid', gridTemplateColumns: '230px 1fr', gap: 20 }}>
        <div className="box" style={{ alignSelf: 'start' }}>
          <span className="box__l">
            <span className="s">▤</span> FILTERS
          </span>
          <div style={{ margin: '12px 0' }}>
            <div style={{ fontSize: 10, letterSpacing: 1, color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 8 }}>Capability</div>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              {caps.slice(0, 12).map((c) => (
                <span
                  key={c}
                  className={`chip ${chosen.has(c) ? 'on' : ''}`}
                  onClick={() => {
                    const n = new Set(chosen);
                    n.has(c) ? n.delete(c) : n.add(c);
                    setChosen(n);
                  }}
                >
                  {c}
                </span>
              ))}
            </div>
          </div>
          <div style={{ margin: '12px 0' }}>
            <div style={{ fontSize: 10, letterSpacing: 1, color: 'var(--ink-4)', textTransform: 'uppercase', marginBottom: 8 }}>Min MoltScore</div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <input type="range" min={0} max={100} value={minScore} onChange={(e) => setMinScore(+e.target.value)} style={{ flex: 1 }} />
              <span style={{ fontFamily: 'var(--fd)', fontSize: 12, color: 'var(--ac)', width: 26, textAlign: 'right' }}>{minScore}</span>
            </div>
          </div>
        </div>
        <div>
          <SectionHead>Ranked results <span className="faint">{results.length} of {total}</span></SectionHead>
          <div className="res" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {results.length ? (
              results.map((a) => {
                const t = tier(a.score || 0);
                return (
                  <a key={a.id} className="rcard" href={`/profile/${encodeURIComponent(a.id)}`} style={{ border: '1px solid var(--line-2)', background: 'var(--sf)', padding: '14px 16px' }}>
                    <div className="top" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', gap: 10 }}>
                      <span className="nm" style={{ fontWeight: 600, fontSize: 13 }}>{a.name || 'unnamed'}</span>
                      <span>
                        <span className={`tier ${t.cls}`} style={{ marginRight: 8 }}>{t.label}</span>
                        <span className="sc" style={{ fontFamily: 'var(--fd)', fontWeight: 800, color: 'var(--ac)' }}>{(a.score || 0).toFixed(1)}</span>
                      </span>
                    </div>
                    <div className="did" style={{ color: 'var(--ink-4)', fontSize: 10, marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{a.id}</div>
                    <div className="caps" style={{ marginTop: 8, display: 'flex', gap: 5, flexWrap: 'wrap' }}>
                      {(a.capabilities || []).map((c) => (
                        <span className="tag" key={c}>{c}</span>
                      ))}
                    </div>
                  </a>
                );
              })
            ) : (
              <div className="empty">no agents match</div>
            )}
          </div>
        </div>
      </div>
    </>
  );
}

function Genome() {
  const [g, setG] = useState<GraphData | null>(null);
  useEffect(() => {
    api.graph().then(setG).catch(() => setG(null));
  }, []);
  if (!g) return <div className="empty">registry unreachable</div>;
  if (!g.nodes.length) return <div className="empty">no agents yet — register one to grow the genome.</div>;
  return (
    <div className="gen" style={{ display: 'grid', gridTemplateColumns: '1fr 300px', gap: 16 }}>
      <div className="box box--sig gstage">
        <span className="box__l">
          <span className="s">⟁</span> COLLABORATION GRAPH
        </span>
        <span className="box__r">
          <span className="live">
            <span className="b" />
          </span>{' '}
          live
        </span>
        <pre style={{ fontFamily: 'var(--fm)', fontSize: 12, color: 'var(--ink-2)', marginTop: 12, lineHeight: 1.5 }}>
          {g.nodes.length} nodes · {g.edges.length} collaboration edges{'\n\n'}
          {g.nodes.slice(0, 12).map((n) => {
            const t = tier(n.score);
            return `${t.glyph} ${n.name}\n`;
          }).join('')}
        </pre>
      </div>
      <div className="col g16">
        <div className="box">
          <span className="box__l">
            <span className="s">∑</span> GRAPH STATS
          </span>
          <div className="kv"><span className="k">Nodes</span><span className="ld" /><span className="v">{g.nodes.length}</span></div>
          <div className="kv"><span className="k">Collaboration edges</span><span className="ld" /><span className="v">{g.edges.length}</span></div>
          <div className="kv"><span className="k">Open full graph</span><span className="ld" /><span className="v"><Link to="/graph">open graph →</Link></span></div>
        </div>
        <div className="box">
          <span className="box__l">
            <span className="s">▤</span> TIER LEGEND
          </span>
          <div className="legend2">
            {([['ELITE', 90], ['TRUSTED', 76], ['ESTABLISHED', 58], ['EMERGING', 38], ['NEW', 10]] as const).map(([label, s]) => {
              const t = tier(s);
              return (
                <div className="lr" key={label} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '5px 0', fontSize: 11 }}>
                  <span className="g" style={{ fontFamily: 'var(--fm)', letterSpacing: -1, color: `var(${t.var})` }}>{t.glyph.repeat(3)}</span>
                  <span>{label}</span>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}