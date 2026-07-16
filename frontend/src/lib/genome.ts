// lib/genome.ts — the ascii genome: nodes projected onto a rotating sphere,
// drawn into a fixed character grid. Edges are real signed attestations, node
// glyph is its MoltScore tier. Auto-spins; pointer drag steers.
//
// This is imperative on purpose: it repaints at 60fps into a single <pre>, so
// React state would re-render the tree every frame for no gain. The component
// owns the element; this owns the pixels.
import { tier } from './tier';
import type { GraphData } from './api';

export interface GenomeHandle {
  stop(): void;
  count(): { n: number; e: number };
}

interface Placed {
  id: string;
  score: number;
  p: [number, number, number];
}

// Even spread over a sphere — deterministic, so the same graph always opens
// from the same angle.
function fibonacci(nodes: { id: string; score: number }[]): Placed[] {
  const N = nodes.length;
  return nodes.map((n, i) => {
    const y = 1 - (i / Math.max(1, N - 1)) * 2;
    const r = Math.sqrt(Math.max(0, 1 - y * y));
    const th = Math.PI * (3 - Math.sqrt(5)) * i;
    return { id: n.id, score: n.score, p: [Math.cos(th) * r, y, Math.sin(th) * r] as [number, number, number] };
  });
}

export function mountGenome(
  el: HTMLPreElement,
  data: GraphData,
  opt: { cols?: number; rows?: number; max?: number } = {},
): GenomeHandle | null {
  const COLS = opt.cols || 46;
  const ROWS = opt.rows || 20;
  const raw = (data.nodes || []).slice(0, opt.max || 40);
  if (!raw.length) {
    el.textContent = '  no nodes yet — register an agent.';
    return null;
  }
  const nodes = fibonacci(raw);
  const idx = new Map(nodes.map((n, i) => [n.id, i]));
  const edges = (data.edges || []).filter(
    (e) => idx.has(e.source) && idx.has(e.target) && e.source !== e.target,
  );

  let ax = 0.5;
  let ay = 0;
  let drag = false;
  let px = 0;
  let py = 0;
  let raf = 0;
  const vx = 0.004;
  const vy = 0.0016;
  const glyph = (s: number) => tier(s).glyph[0];

  function rot(p: [number, number, number]): [number, number, number] {
    let [x, y, z] = p;
    let c = Math.cos(ay);
    let s = Math.sin(ay);
    [x, z] = [x * c - z * s, x * s + z * c];
    c = Math.cos(ax);
    s = Math.sin(ax);
    [y, z] = [y * c - z * s, y * s + z * c];
    return [x, y, z];
  }

  function frame() {
    if (!drag) {
      ay += vx;
      ax += vy * Math.sin(ay * 0.3);
    }
    const grid = Array.from({ length: ROWS }, () => new Array<string>(COLS).fill(' '));
    const proj = nodes.map((n) => {
      const [x, y, z] = rot(n.p);
      const sc = 1 / (2.2 - z);
      return {
        cx: Math.round((x * sc + 0.5) * (COLS - 1)),
        cy: Math.round((y * sc * 0.5 + 0.5) * (ROWS - 1)),
        z,
        n,
      };
    });
    // edges first (faint line chars), then nodes on top
    for (const e of edges) {
      const a = proj[idx.get(e.source)!];
      const b = proj[idx.get(e.target)!];
      const steps = Math.max(Math.abs(a.cx - b.cx), Math.abs(a.cy - b.cy));
      for (let s = 1; s < steps; s++) {
        const gx = Math.round(a.cx + ((b.cx - a.cx) * s) / steps);
        const gy = Math.round(a.cy + ((b.cy - a.cy) * s) / steps);
        if (grid[gy] && grid[gy][gx] === ' ') grid[gy][gx] = '·';
      }
    }
    proj.sort((a, b) => a.z - b.z); // painter's algorithm: far nodes first
    for (const p of proj) if (grid[p.cy] && p.cx >= 0 && p.cx < COLS) grid[p.cy][p.cx] = glyph(p.n.score);
    el.textContent = grid.map((r) => r.join('')).join('\n');
    raf = requestAnimationFrame(frame);
  }

  el.style.touchAction = 'none';
  el.style.cursor = 'grab';
  const down = (e: PointerEvent) => {
    drag = true;
    px = e.clientX;
    py = e.clientY;
    el.style.cursor = 'grabbing';
    el.setPointerCapture(e.pointerId);
  };
  const move = (e: PointerEvent) => {
    if (!drag) return;
    ay += (e.clientX - px) * 0.01;
    ax += (e.clientY - py) * 0.01;
    px = e.clientX;
    py = e.clientY;
  };
  const stop = () => {
    drag = false;
    el.style.cursor = 'grab';
  };
  el.addEventListener('pointerdown', down);
  el.addEventListener('pointermove', move);
  el.addEventListener('pointerup', stop);
  el.addEventListener('pointercancel', stop);
  frame();

  return {
    stop() {
      cancelAnimationFrame(raf);
      el.removeEventListener('pointerdown', down);
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', stop);
      el.removeEventListener('pointercancel', stop);
    },
    count() {
      return { n: nodes.length, e: edges.length };
    },
  };
}
