/* term2.js — shared runtime for the MoltNet design system.
   Pure vanilla, no deps. Exposes window.T with the primitives every
   page reuses: DOM helpers, the tier model, reveal-on-scroll, count-up,
   and the ascii genome orbit renderer (hero + dashboard both use it). */
(function () {
  const $ = (s, r) => (r || document).querySelector(s);
  const $$ = (s, r) => [...(r || document).querySelectorAll(s)];
  const esc = s => (s || '').replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
  const clamp = (v, a, b) => Math.max(a, Math.min(b, v));

  // ---- tier model: density = trust. Single source of truth. ----
  // Colors resolve to CSS vars so they track the active theme.
  const TIERS = [
    { min: 85, label: 'ELITE', cls: 'tier--elite', glyph: '██', v: '--t-elite' },
    { min: 70, label: 'TRUSTED', cls: 'tier--trusted', glyph: '▓▓', v: '--t-trusted' },
    { min: 50, label: 'ESTABLISHED', cls: 'tier--estab', glyph: '▒▒', v: '--t-estab' },
    { min: 30, label: 'EMERGING', cls: 'tier--emerg', glyph: '░░', v: '--t-emerg' },
    { min: 0, label: 'NEW', cls: 'tier--new', glyph: '··', v: '--t-new' },
  ];
  const tier = s => TIERS.find(t => (s || 0) >= t.min) || TIERS[TIERS.length - 1];
  const cssVar = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

  // ---- reveal on scroll ----
  function reveal(root) {
    const els = $$('.reveal', root);
    if (!('IntersectionObserver' in window) || matchMedia('(prefers-reduced-motion: reduce)').matches) {
      els.forEach(e => e.classList.add('in')); return;
    }
    const io = new IntersectionObserver((ents) => {
      ents.forEach(e => { if (e.isIntersecting) { e.target.classList.add('in'); io.unobserve(e.target); } });
    }, { threshold: 0.12 });
    els.forEach(e => io.observe(e));
  }

  // ---- count up a number to data-to (or explicit target) ----
  function countUp(el, to, opt) {
    opt = opt || {};
    const dur = opt.dur || 1100, dp = opt.dp || 0, t0 = performance.now();
    const ease = x => 1 - Math.pow(1 - x, 3);
    (function step(now) {
      const p = clamp((now - t0) / dur, 0, 1);
      el.textContent = (to * ease(p)).toFixed(dp);
      if (p < 1) requestAnimationFrame(step);
    })(t0);
  }

  // ---- gauge bar string: filled + empty blocks to width ----
  function gauge(score, width) {
    width = width || 26;
    const n = Math.round(clamp(score / 100, 0, 1) * width);
    return `<span>${'█'.repeat(n)}</span><span class="e">${'█'.repeat(width - n)}</span>`;
  }

  /* ---- ascii genome: nodes on a sphere, drag to orbit ----
     el: <pre>. data: {nodes:[{id,name,score}], edges:[{source,target,count}]}.
     Renders a rotating projection into a fixed char grid. Falls back to a
     ring layout if there are few nodes. Auto-spins; pointer drag steers. */
  function genome(el, data, opt) {
    opt = opt || {};
    const COLS = opt.cols || 46, ROWS = opt.rows || 20;
    let nodes = (data.nodes || []).slice(0, opt.max || 40);
    if (!nodes.length) { el.textContent = '  no nodes yet — register an agent.'; return null; }
    const idx = {}; nodes.forEach((n, i) => idx[n.id] = i);
    const edges = (data.edges || []).filter(e => idx[e.source] != null && idx[e.target] != null && e.source !== e.target);

    // place on fibonacci sphere for even spread
    const N = nodes.length;
    nodes.forEach((n, i) => {
      const y = 1 - (i / Math.max(1, N - 1)) * 2;
      const r = Math.sqrt(Math.max(0, 1 - y * y));
      const th = Math.PI * (3 - Math.sqrt(5)) * i;
      n.p = [Math.cos(th) * r, y, Math.sin(th) * r];
    });

    let ax = 0.5, ay = 0, drag = false, px = 0, py = 0, vx = 0.004, vy = 0.0016, raf = 0;
    const glyph = s => tier(s).glyph[0];

    function rot(p) {
      let [x, y, z] = p;
      let c = Math.cos(ay), s = Math.sin(ay); [x, z] = [x * c - z * s, x * s + z * c];
      c = Math.cos(ax); s = Math.sin(ax); [y, z] = [y * c - z * s, y * s + z * c];
      return [x, y, z];
    }
    function frame() {
      if (!drag) { ay += vx; ax += vy * Math.sin(ay * 0.3); }
      const grid = Array.from({ length: ROWS }, () => Array(COLS).fill(' '));
      const proj = nodes.map(n => {
        const [x, y, z] = rot(n.p); const sc = 1 / (2.2 - z);
        return { cx: Math.round((x * sc + 0.5) * (COLS - 1)), cy: Math.round((y * sc * 0.5 + 0.5) * (ROWS - 1)), z, n };
      });
      // edges first (faint line chars), then nodes on top
      for (const e of edges) {
        const a = proj[idx[e.source]], b = proj[idx[e.target]];
        const steps = Math.max(Math.abs(a.cx - b.cx), Math.abs(a.cy - b.cy));
        for (let s = 1; s < steps; s++) {
          const gx = Math.round(a.cx + (b.cx - a.cx) * s / steps);
          const gy = Math.round(a.cy + (b.cy - a.cy) * s / steps);
          if (grid[gy] && grid[gy][gx] === ' ') grid[gy][gx] = '·';
        }
      }
      proj.sort((a, b) => a.z - b.z);
      for (const p of proj) if (grid[p.cy] && p.cx >= 0 && p.cx < COLS) grid[p.cy][p.cx] = glyph(p.n.score);
      el.textContent = grid.map(r => r.join('')).join('\n');
      raf = requestAnimationFrame(frame);
    }
    // drag to orbit
    el.style.touchAction = 'none'; el.style.cursor = 'grab';
    el.addEventListener('pointerdown', e => { drag = true; px = e.clientX; py = e.clientY; el.style.cursor = 'grabbing'; el.setPointerCapture(e.pointerId); });
    el.addEventListener('pointermove', e => { if (!drag) return; ay += (e.clientX - px) * 0.01; ax += (e.clientY - py) * 0.01; px = e.clientX; py = e.clientY; });
    const stop = () => { drag = false; el.style.cursor = 'grab'; };
    el.addEventListener('pointerup', stop); el.addEventListener('pointercancel', stop);
    frame();
    return {
      update(d) { nodes = (d.nodes || []).slice(0, opt.max || 40); nodes.forEach((n, i) => { const y = 1 - (i / Math.max(1, nodes.length - 1)) * 2; const r = Math.sqrt(Math.max(0, 1 - y * y)); const th = Math.PI * (3 - Math.sqrt(5)) * i; n.p = [Math.cos(th) * r, y, Math.sin(th) * r]; }); },
      stop() { cancelAnimationFrame(raf); },
      count() { return { n: nodes.length, e: edges.length }; },
    };
  }

  async function j(url) { const r = await fetch(url); if (!r.ok) throw new Error(r.status); return r.json(); }

  window.T = { $, $$, esc, clamp, tier, cssVar, reveal, countUp, gauge, genome, j };
  document.addEventListener('DOMContentLoaded', () => reveal());
})();
