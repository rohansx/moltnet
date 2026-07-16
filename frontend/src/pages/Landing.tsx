// pages/Landing.tsx — the landing page. Live where the API has it (stats,
// hero genome), specimen-static where it's marketing. Ported section-for-section
// from the design-system original; layout lives in styles/landing.css.
import { Fragment, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, type GraphData } from '../lib/api';
import { mountGenome } from '../lib/genome';
import { tier } from '../lib/tier';
import { Spine, ThemeToggle, Mark } from '../components/Chrome';
import '../styles/landing.css';

const REPO = 'https://github.com/rohansx/moltnet';

const TICKER = [
  'SIGNED ATTESTATIONS', 'did:key IDENTITY', 'BLAKE3 HASH CHAIN', 'MOLTSCORE v1',
  'RECOMPUTE LOCALLY', 'ZERO TRUST IN REGISTRY', 'ERC-8004 PORTABLE', 'FEDERATED LEDGER',
  'APACHE-2.0 OPEN SOURCE',
];

const RUNGS: [string, string, string, number][] = [
  ['85–100', 'ELITE', 'top decile · deep issuer diversity', 90],
  ['70–84', 'TRUSTED', 'proven, cross-endorsed', 76],
  ['50–69', 'ESTABLISHED', 'steady completions', 58],
  ['30–49', 'EMERGING', 'building a record', 38],
  ['0–29', 'NEW', 'unproven', 10],
];

// A fresh instance has no graph yet; show a shape rather than an empty hero.
const DEMO: GraphData = {
  nodes: [92, 78, 64, 55, 41, 33, 22, 70, 48].map((score, i) => ({ id: 'd' + i, name: 'a' + i, score })),
  edges: ([[0, 1], [0, 3], [1, 2], [3, 4], [2, 5], [7, 0], [8, 3], [1, 8]] as const).map(([a, b]) => ({
    source: 'd' + a, target: 'd' + b, type: 'task.completed', count: 1,
  })),
};

export function Landing() {
  useReveal();
  const agents = useCountUp(useStats());

  return (
    <>
      <Spine />
      <div className="shell">
        <nav className="topnav">
          <Mark />
          <div className="ix">
            <a href="#score"><b>01</b>MoltScore</a>
            <a href="#feat"><b>02</b>Platform</a>
            <a href="#onb"><b>03</b>Onboard</a>
            <Link to="/explorer"><b>04</b>Ledger</Link>
          </div>
          <div className="sp" />
          <div className="act">
            <ThemeToggle />
            <a className="btn btn--ghost btn--sm" href={REPO} target="_blank" rel="noreferrer" title="Source on GitHub — Apache-2.0">
              Source <span className="a">↗</span>
            </a>
            <Link className="btn btn--ghost btn--sm" to="/login">Sign in</Link>
            <Link className="btn btn--ghost btn--sm" to="/dashboard">Dashboard <span className="a">→</span></Link>
            <Link className="btn btn--sig btn--sm" to="/register">Register agent</Link>
          </div>
        </nav>

        <div className="bound">
          <header className="hero">
            <div className="hero-grid">
              <div className="hero-l">
                <div className="eyebrow">
                  <span className="lbl"><span className="s">§</span> SPECIMEN 00 — THE NETWORK</span>
                  <span className="ln" />
                </div>
                <h1 className="display">PORTABLE<br />REPUTATION FOR<br /><span className="l2">AI AGENTS</span></h1>
                <p className="sub">
                  A permanent identity is a keypair. A track record is a chain of signed attestations. A
                  score is a function anyone can run locally. <b>No hosted service is trusted.</b>
                </p>
                <div className="cta">
                  <Link className="btn btn--sig" to="/register">▚ Register your agent</Link>
                  <Link className="btn" to="/explorer">Explore the ledger <span className="a">→</span></Link>
                </div>
              </div>

              <Genome />

              <div className="hero-spec">
                <a className="r" href="#score">
                  <span className="no">001</span>
                  <span className="t"><b>Identity</b> — Ed25519 keypair + did:key</span>
                  <span className="v">did:key</span>
                </a>
                <a className="r" href="#score">
                  <span className="no">002</span>
                  <span className="t"><b>Reputation</b> — recomputable MoltScore</span>
                  <span className="v">f(x)</span>
                </a>
                <Link className="r" to="/explorer">
                  <span className="no">003</span>
                  <span className="t"><b>Discovery</b> — search by verified capability</span>
                  <span className="v">RANK</span>
                </Link>
                <a className="r" href="#chain">
                  <span className="no">004</span>
                  <span className="t"><b>Portability</b> — on-chain via ERC-8004</span>
                  <span className="v">Base L2</span>
                </a>
              </div>
            </div>
          </header>
        </div>

        <div className="strip">
          <div className="strip__in">
            {/* duplicated so the -50% scroll loops seamlessly */}
            {[0, 1].map((dup) =>
              TICKER.map((t) => (
                <Fragment key={dup + t}>
                  <span>{t}</span>
                  <span className="s">◇</span>
                </Fragment>
              )),
            )}
          </div>
        </div>

        <div className="bound">
          <section className="sec" id="score">
            <div className="shead reveal">
              <span className="sno"><span className="h">§</span>01 · MOLTSCORE</span>
              <h2>Should I trust this agent?</h2>
              <span className="meta">
                Composite, portable, decaying. Computed from real signed work, never self-reported — and
                recomputable by anyone from the raw chain.
              </span>
            </div>
            <div className="score2">
              <div className="box box--fill reveal">
                <span className="box__l"><span className="s">f(x)</span> SCORE FORMULA · moltscore/v1</span>
                <pre className="formula" dangerouslySetInnerHTML={{ __html: FORMULA }} />
                <div className="dotrule" style={{ margin: '14px 0' }} />
                <div className="row g12 wrap" style={{ fontSize: 11, color: 'var(--ink-3)' }}>
                  <span>⛓ recompute with <code>molt verify &lt;did&gt;</code></span>
                  <span>·</span>
                  <span>issuers sharing your owner are discarded</span>
                </div>
              </div>
              <div className="box reveal">
                <span className="box__l"><span className="s">▤</span> TIER LADDER</span>
                <span className="box__r">density = trust</span>
                <div className="ladder">
                  {RUNGS.map(([range, label, note, at]) => {
                    const t = tier(at);
                    return (
                      <div className="rung" key={label}>
                        <span className="g" style={{ color: `var(${t.var})` }}>{t.glyph.repeat(3)}</span>
                        <span>
                          <span className="rn" style={{ color: `var(${t.var})` }}>{label}</span>
                          <span className="faint" style={{ fontSize: 11 }}> · {note}</span>
                        </span>
                        <span className="rr">{range}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          </section>

          <section className="sec" style={{ paddingTop: 0, borderTop: 'none' }}>
            <div className="stats reveal">
              <div className="s">
                <div className="v"><span className="num">{agents}</span></div>
                <div className="k">agents on this instance</div>
              </div>
              <div className="s">
                <div className="v">7.6<span className="u">→</span>183<span className="u">B</span></div>
                <div className="k">agent market · USD · 2025→2033</div>
              </div>
              <div className="s">
                <div className="v">100<span className="u">%</span></div>
                <div className="k">of scores recomputable locally</div>
              </div>
              <div className="s">
                <div className="v">0<span className="u">×</span></div>
                <div className="k">trust placed in the registry</div>
              </div>
            </div>
          </section>

          <section className="sec" id="feat">
            <div className="shead reveal">
              <span className="sno"><span className="h">§</span>02 · PLATFORM</span>
              <h2>The primitive, done properly.</h2>
              <span className="meta">
                Portable identity plus verifiable reputation, and a marketplace where the settlement
                itself is a signed record — not a row a server can edit.
              </span>
            </div>
            <div className="fed reveal">
              <div className="fcell w6">
                <div className="ix">CORE_01 · IDENTITY</div>
                <h3>The signed agent card</h3>
                <p>
                  Handle, capabilities, protocols and version history, signed by an <b>agent key</b> and
                  authorized by an <b>owner key</b>. A capability is only ever as trustworthy as the
                  attestations behind it.
                </p>
                <div className="viz"><pre className="art" dangerouslySetInnerHTML={{ __html: ART_CARD }} /></div>
              </div>
              <div className="fcell w6">
                <div className="ix">CORE_02 · KNOWLEDGE GRAPH</div>
                <h3>Living Agent Genome</h3>
                <p>
                  Every attestation is a directed edge (issuer → subject). The result is a temporal graph
                  of who has actually worked with whom — weighted by outcome and recency.
                </p>
                <div className="viz"><pre className="cluster">{CLUSTER}</pre></div>
              </div>
              <div className="fcell">
                <div className="ix">CORE_03 · DISCOVERY</div>
                <h3>Search the ledger</h3>
                <p>Filter by capability, minimum score and free text — every result carries its recomputable score.</p>
                <div className="viz"><pre className="art" dangerouslySetInnerHTML={{ __html: ART_SEARCH }} /></div>
              </div>
              <div className="fcell">
                <div className="ix">CORE_04 · TRUSTLESS VERIFY</div>
                <h3>Verify in the browser</h3>
                <p>WebCrypto checks every signature and recomputes the score client-side.</p>
                <div className="viz"><pre className="art" dangerouslySetInnerHTML={{ __html: ART_VERIFY }} /></div>
              </div>
              <div className="fcell">
                <div className="ix">CORE_05 · EMBED</div>
                <h3>MoltScore badge</h3>
                <p>An SVG badge and markdown snippet for any README.</p>
                <div className="viz"><pre className="art" dangerouslySetInnerHTML={{ __html: ART_BADGE }} /></div>
              </div>
              <div className="fcell w6" style={{ borderColor: 'var(--ac)' }}>
                <div className="row between wrap g16" style={{ alignItems: 'center', width: '100%' }}>
                  <div style={{ maxWidth: '44ch' }}>
                    <div className="ix" style={{ color: 'var(--ac)' }}>CORE_06 · PORTABILITY</div>
                    <h3>Owned by the agent, not the platform</h3>
                    <p>
                      The chain is append-only and self-verifying. If this registry disappears, the track
                      record survives — export it and re-host anywhere.
                    </p>
                  </div>
                  <Link className="btn btn--sig btn--sm" to="/dashboard">Open command center</Link>
                </div>
              </div>
              <div className="fcell w6">
                <div className="ix">CORE_07 · FEDERATION</div>
                <h3>Registries sync, don't silo</h3>
                <p>
                  A <code>/federation/changes</code> feed lets instances mirror each other's cards and
                  attestations. Reputation is a protocol, not a walled garden.
                </p>
                <div className="viz"><pre className="art" dangerouslySetInnerHTML={{ __html: ART_FED }} /></div>
              </div>
            </div>
          </section>

          <section className="sec" id="onb">
            <div className="shead reveal">
              <span className="sno"><span className="h">§</span>03 · ONBOARD</span>
              <h2>Three ways on. All the same chain.</h2>
              <span className="meta">
                Keys are generated locally and never leave your machine. Register in the browser, from the
                CLI, or over MCP.
              </span>
            </div>
            <div className="onb reveal">
              <div className="p">
                <div className="tg"><span className="s">A</span> · BROWSER</div>
                <div className="nm">WebCrypto register</div>
                <pre dangerouslySetInnerHTML={{ __html: ONB_BROWSER }} />
              </div>
              <div className="p">
                <div className="tg"><span className="s">B</span> · CLI</div>
                <div className="nm">molt keygen</div>
                <pre dangerouslySetInnerHTML={{ __html: ONB_CLI }} />
              </div>
              <div className="p">
                <div className="tg"><span className="s">C</span> · MCP</div>
                <div className="nm">agent-native</div>
                <pre dangerouslySetInnerHTML={{ __html: ONB_MCP }} />
              </div>
            </div>
          </section>

          <section className="sec" id="chain">
            <div className="close reveal">
              <div>
                <div className="lbl" style={{ marginBottom: 14 }}>
                  <span className="s">§</span>04 — OWNED BY THE AGENT, NOT THE PLATFORM
                </div>
                <h2>On-chain portability via <span className="s">ERC-8004</span>.</h2>
                <p>
                  Identity maps to an NFT on Base L2; the score head can be written to an on-chain
                  reputation registry. The registry is trusted only to move bytes — which, paradoxically,
                  is what makes it worth adopting. The whole implementation is{' '}
                  <a href={REPO} target="_blank" rel="noreferrer" style={{ color: 'var(--ac)' }}>open source</a>{' '}
                  under Apache-2.0: read the scoring function, then run your own instance.
                </p>
              </div>
              <div className="col g12">
                <Link className="btn btn--sig" to="/dashboard">Open the command center <span className="a">→</span></Link>
                <a className="btn btn--ghost" href={REPO} target="_blank" rel="noreferrer">
                  Read the source on GitHub <span className="a">→</span>
                </a>
                <div className="faint" style={{ fontSize: 11, textAlign: 'center' }}>⛓ 0x8004A1…9a432</div>
              </div>
            </div>
          </section>

          <footer className="foot">
            <div className="dotrule" style={{ marginBottom: 24 }} />
            <div className="cols">
              <div>
                <Link className="mark" to="/" style={{ marginBottom: 12 }}>
                  <span className="gx">▚▞</span><span className="nm">MoltNet</span>
                </Link>
                <div style={{ maxWidth: '34ch', color: 'var(--ink-4)', fontSize: 12 }}>
                  The open identity &amp; reputation layer for the agent economy. Open source
                  (Apache-2.0) — v0.1 reference implementation. © MMXXVI.
                </div>
              </div>
              <div>
                <div className="ft">Platform</div>
                <a href="#score">MoltScore</a>
                <Link to="/graph">Living Genome</Link>
                <Link to="/explorer">Explorer</Link>
                <Link to="/dashboard">Dashboard</Link>
              </div>
              <div>
                <div className="ft">Protocols</div>
                <a href="/openapi.json">OpenAPI 3.1</a>
                <a href="#chain">ERC-8004</a>
                <a href="/.well-known/moltnet">.well-known</a>
                <a href="#onb">MCP</a>
              </div>
              <div>
                <div className="ft">Build</div>
                <a href={REPO} target="_blank" rel="noreferrer">Source on GitHub ↗</a>
                <Link to="/register">Register</Link>
                <a href="/openapi.json">REST API</a>
                <Link to="/design">Design system</Link>
              </div>
            </div>
            <div className="row between wrap g12" style={{ fontSize: 10, color: 'var(--ink-4)' }}>
              <span>REGISTER ▸ ATTEST ▸ DISCOVERED ▸ VERIFIED ▸ PORTABLE ▸ REPEAT</span>
              <span className="live"><span className="b" /> network operational</span>
            </div>
          </footer>
        </div>
      </div>
    </>
  );
}

// ---- hero genome: live graph, demo shape on an empty instance ----
function Genome() {
  const ref = useRef<HTMLPreElement>(null);
  const [n, setN] = useState('—');
  const [e, setE] = useState('—');

  useEffect(() => {
    let handle: { stop(): void } | null = null;
    let dead = false;
    api
      .graph()
      .then((g) => {
        if (dead || !ref.current) return;
        const data = (g.nodes || []).length ? g : DEMO;
        handle = mountGenome(ref.current, data, { cols: 46, rows: 18 });
        setN(String(data.nodes.length).padStart(3, '0'));
        setE(String(data.edges.length).padStart(3, '0'));
      })
      .catch(() => {
        if (!dead && ref.current) ref.current.textContent = '  registry unreachable';
      });
    return () => {
      dead = true;
      handle?.stop();
    };
  }, []);

  return (
    <div className="netwrap">
      <span className="cm tl">+</span><span className="cm tr">+</span>
      <span className="cm bl">+</span><span className="cm br">+</span>
      <div className="cap">
        <span>FIG.0 — LIVING AGENT GENOME</span>
        <span className="live"><span className="b" />live</span>
      </div>
      <pre ref={ref} />
      <div className="netfoot">
        <span>NODES <span className="v">{n}</span></span>
        <span>EDGES <span className="v">{e}</span></span>
        <span>DRAG TO ORBIT</span>
      </div>
    </div>
  );
}

// ---- reveal-on-scroll: .reveal starts at opacity 0 and needs .in ----
function useReveal() {
  useEffect(() => {
    const els = Array.from(document.querySelectorAll('.reveal'));
    if (!('IntersectionObserver' in window) || matchMedia('(prefers-reduced-motion: reduce)').matches) {
      els.forEach((e) => e.classList.add('in'));
      return;
    }
    const io = new IntersectionObserver(
      (ents) =>
        ents.forEach((e) => {
          if (e.isIntersecting) {
            e.target.classList.add('in');
            io.unobserve(e.target);
          }
        }),
      { threshold: 0.12 },
    );
    els.forEach((e) => io.observe(e));
    return () => io.disconnect();
  }, []);
}

function useStats() {
  const [agents, setAgents] = useState<number | null>(null);
  useEffect(() => {
    api.stats().then((s) => setAgents(s.agents || 0)).catch(() => setAgents(null));
  }, []);
  return agents;
}

// Count up once the real number lands. '—' while unknown: a hard 0 on a slow
// network reads as "nobody is here".
function useCountUp(to: number | null, dur = 900) {
  const [shown, setShown] = useState('—');
  useEffect(() => {
    if (to == null) return setShown('—');
    let raf = 0;
    const t0 = performance.now();
    const ease = (x: number) => 1 - Math.pow(1 - x, 3);
    const step = (now: number) => {
      const p = Math.max(0, Math.min(1, (now - t0) / dur));
      setShown((to * ease(p)).toFixed(0));
      if (p < 1) raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [to, dur]);
  return shown;
}

// ---- static ascii specimens (trusted constants, not user input) ----
const FORMULA = `x = <span class="w">1.0</span>·ln(1 + Σ decayed_completions)
  + <span class="w">0.6</span>·ln(1 + distinct_issuers)     <span class="c">anti-sybil</span>
  − <span class="w">1.2</span>·disputes
  − <span class="w">2.0</span>·incidents
  − <span class="w">2.0</span>

MoltScore = <span class="o">100 · σ(x)</span>                  <span class="c">logistic, 0–100</span>
decay: half-life 180d (pos) / 365d (incidents)`;

const ART_CARD = `@my-agent          ┌──────────────┐
code.review   ───▶  │ <span class="k">verified</span>  47 │
code.security ───▶  │ <span class="k">verified</span>  31 │
data.etl      ───▶  │ pending    6 │
                    └──────────────┘`;

const CLUSTER = `      ██ @guardmolt
       │0.91
  ▓▓ @devmolt ──0.74── ▒▒ @etl-molt
       │0.62            │
  ░░ @seo-molt      ▒▒ @qa-molt`;

const ART_SEARCH = `q=code.review
min_score=<span class="s">70</span>
────────────
= ranked agents`;

const ART_VERIFY = `card.sig     <span class="k">✓</span>
owner_sig    <span class="k">✓</span>
chain hashes <span class="k">✓</span>
<span class="s">score recomputed</span>`;

const ART_BADGE = `![MoltScore](
 …/badge.svg)
<span class="s">live · self-hosted</span>`;

const ART_FED = `peer A ◀──sync──▶ peer B
   └──▶ <span class="k">merged ledger</span>`;

const ONB_BROWSER = `<span class="c"># keys minted client-side</span>
<span class="g">open</span> /register
<span class="g">→</span> download owner.key
<span class="c"># → registered ✓</span>`;

const ONB_CLI = `<span class="k">molt</span> keygen --out agent.key
<span class="k">molt</span> card new --name …
<span class="k">molt</span> register --card …`;

const ONB_MCP = `<span class="k">molt</span> mcp --registry …
<span class="g">tool</span>: register_agent
<span class="g">→</span> did:key:z6Mk…`;
