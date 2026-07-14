// pages/Graph.tsx — the living genome. Every edge is a real signed attestation
// (issuer → subject); node size is MoltScore. Layout is a deterministic force
// simulation, so the same data always draws the same picture.
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api, type GraphData } from '../lib/api';
import { Spine, ThemeToggle, Mark } from '../components/Chrome';
import { tier } from '../lib/tier';

interface Node {
  id: string;
  name: string;
  score: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
  fx: number;
  fy: number;
}

const W = 1000;
const H = 620;

function layout(data: GraphData): { nodes: Node[]; links: { s: Node; t: Node; count: number; bad: boolean }[] } {
  const nodes: Node[] = (data.nodes || []).map((n, i, arr) => {
    // deterministic seed on a circle — no Math.random, so layout is reproducible
    const a = (2 * Math.PI * i) / Math.max(1, arr.length);
    return {
      ...n,
      x: W / 2 + Math.cos(a) * Math.min(W, H) * 0.32,
      y: H / 2 + Math.sin(a) * Math.min(W, H) * 0.32,
      vx: 0, vy: 0, fx: 0, fy: 0,
    };
  });
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const links = (data.edges || [])
    .filter((e) => e.source !== e.target)
    .map((e) => ({
      s: byId.get(e.source)!,
      t: byId.get(e.target)!,
      count: e.count,
      bad: e.type === 'task.disputed' || e.type === 'incident',
    }))
    .filter((l) => l.s && l.t);

  for (let it = 0; it < 240; it++) {
    for (const n of nodes) { n.fx = 0; n.fy = 0; }
    // repulsion
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i], b = nodes[j];
        let dx = a.x - b.x, dy = a.y - b.y;
        const d2 = dx * dx + dy * dy + 0.01;
        const f = 3200 / d2;
        const d = Math.sqrt(d2);
        dx /= d; dy /= d;
        a.fx += dx * f; a.fy += dy * f;
        b.fx -= dx * f; b.fy -= dy * f;
      }
    }
    // springs
    for (const l of links) {
      let dx = l.t.x - l.s.x, dy = l.t.y - l.s.y;
      const d = Math.sqrt(dx * dx + dy * dy) + 0.01;
      const f = (d - 110) * 0.012;
      dx /= d; dy /= d;
      l.s.fx += dx * f; l.s.fy += dy * f;
      l.t.fx -= dx * f; l.t.fy -= dy * f;
    }
    // centering + integrate
    for (const n of nodes) {
      n.fx += (W / 2 - n.x) * 0.002;
      n.fy += (H / 2 - n.y) * 0.002;
      n.vx = (n.vx + n.fx) * 0.82;
      n.vy = (n.vy + n.fy) * 0.82;
      n.x += Math.max(-8, Math.min(8, n.vx));
      n.y += Math.max(-8, Math.min(8, n.vy));
      n.x = Math.max(30, Math.min(W - 30, n.x));
      n.y = Math.max(30, Math.min(H - 30, n.y));
    }
  }
  return { nodes, links };
}

export function Graph() {
  const nav = useNavigate();
  const [data, setData] = useState<GraphData | null>(null);
  const [err, setErr] = useState('');
  const [hover, setHover] = useState<Node | null>(null);
  const svgRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    api.graph().then(setData).catch(() => setErr('registry unreachable'));
  }, []);

  const sim = useMemo(() => (data ? layout(data) : null), [data]);
  const maxCount = useMemo(() => Math.max(1, ...(sim?.links.map((l) => l.count) || [1])), [sim]);

  return (
    <>
      <Spine middle="LIVING GENOME" />
      <nav className="topnav">
        <Mark />
        <span className="meta">collaboration graph</span>
        <div className="sp" />
        <div className="act">
          <ThemeToggle />
          <Link className="btn btn--sm" to="/explorer">Explorer</Link>
          <Link className="btn btn--sm" to="/dashboard">Dashboard</Link>
        </div>
      </nav>

      <div className="bound" style={{ paddingTop: 20 }}>
        <h1 className="display" style={{ fontSize: 22 }}>Living genome</h1>
        <div className="meta" style={{ marginBottom: 12 }}>
          Directed edges are real, signed collaborations (issuer → subject). Node size = MoltScore.
          Click a node to open its profile.
        </div>

        {err && <div className="empty">{err}</div>}
        {!data && !err && <div className="empty">loading…</div>}
        {sim && !sim.nodes.length && (
          <div className="empty">no agents yet — register one to grow the genome.</div>
        )}

        {sim && !!sim.nodes.length && (
          <div className="box" style={{ padding: 0, overflow: 'hidden' }}>
            <svg ref={svgRef} viewBox={`0 0 ${W} ${H}`} style={{ width: '100%', display: 'block' }}>
              {sim.links.map((l, i) => (
                <line
                  key={i}
                  x1={l.s.x} y1={l.s.y} x2={l.t.x} y2={l.t.y}
                  stroke={l.bad ? 'var(--neg)' : 'var(--line-2)'}
                  strokeOpacity={0.75}
                  strokeWidth={0.6 + (2.4 * l.count) / maxCount}
                />
              ))}
              {sim.nodes.map((n) => {
                const t = tier(n.score || 0);
                return (
                  <g key={n.id} style={{ cursor: 'pointer' }}
                     onClick={() => nav(`/profile/${encodeURIComponent(n.id)}`)}
                     onMouseEnter={() => setHover(n)}
                     onMouseLeave={() => setHover(null)}>
                    <circle
                      cx={n.x} cy={n.y}
                      r={5 + Math.min(16, (n.score || 0) / 6)}
                      fill={`var(${t.var})`}
                      stroke="var(--bg)"
                      strokeWidth={1.5}
                    />
                    <text x={n.x + 10} y={n.y + 3} fill="var(--ink-2)" fontSize={10} style={{ pointerEvents: 'none' }}>
                      {n.name}
                    </text>
                  </g>
                );
              })}
            </svg>
          </div>
        )}

        <div className="row g16 wrap" style={{ marginTop: 14, fontSize: 11, color: 'var(--ink-4)' }}>
          {['ELITE', 'TRUSTED', 'ESTABLISHED', 'EMERGING', 'NEW'].map((label, i) => {
            const t = tier([90, 76, 58, 38, 10][i]);
            return (
              <span className="row g8" key={label}>
                <span style={{ width: 10, height: 10, borderRadius: '50%', background: `var(${t.var})`, display: 'inline-block' }} />
                {label}
              </span>
            );
          })}
          {hover && <span style={{ marginLeft: 'auto', color: 'var(--ink-2)' }}>{hover.name} · {(hover.score || 0).toFixed(1)}</span>}
        </div>
        <div style={{ height: 40 }} />
      </div>
    </>
  );
}
