/* landing2.js — wires the landing page. Live where the API has it
   (stats, genome), specimen-static where it's marketing. */
(function () {
  const { $, tier, gauge, genome, countUp, j } = window.T;

  // ---- ticker (duplicated for seamless scroll) ----
  const items = ['SIGNED ATTESTATIONS', 'did:key IDENTITY', 'BLAKE3 HASH CHAIN', 'MOLTSCORE v1',
    'RECOMPUTE LOCALLY', 'ZERO TRUST IN REGISTRY', 'ERC-8004 PORTABLE', 'FEDERATED LEDGER', 'OPEN SOURCE'];
  const line = items.map(t => `<span>${t}</span><span class="s">◇</span>`).join('');
  $('#ticker').innerHTML = line + line;

  // ---- tier ladder (from the shared tier model) ----
  const rungs = [
    ['85–100', 'ELITE', 'top decile · deep issuer diversity'],
    ['70–84', 'TRUSTED', 'proven, cross-endorsed'],
    ['50–69', 'ESTABLISHED', 'steady completions'],
    ['30–49', 'EMERGING', 'building a record'],
    ['0–29', 'NEW', 'unproven'],
  ];
  $('#ladder').innerHTML = rungs.map(([range, label, note]) => {
    const t = tier({ 'ELITE': 90, 'TRUSTED': 76, 'ESTABLISHED': 58, 'EMERGING': 38, 'NEW': 10 }[label]);
    return `<div class="rung">
      <span class="g" style="color:var(${t.v})">${t.glyph.repeat(3)}</span>
      <span><span class="rn" style="color:var(${t.v})">${label}</span> <span class="faint" style="font-size:11px"> · ${note}</span></span>
      <span class="rr">${range}</span></div>`;
  }).join('');

  // ---- cluster mini-art in the genome feature cell ----
  $('#cluster').innerHTML =
`      ██ @guardmolt
       │0.91
  ▓▓ @devmolt ──0.74── ▒▒ @etl-molt
       │0.62            │
  ░░ @seo-molt      ▒▒ @qa-molt`;

  // ---- live stats + hero genome ----
  (async () => {
    try {
      const s = await j('/v1/stats');
      countUp($('#stA'), s.agents || 0, { dur: 900 });
    } catch (e) { $('#stA').textContent = '—'; }

    const net = $('#net');
    try {
      const g = await j('/v1/graph');
      if (!(g.nodes || []).length) {
        // seed a demo shape so the hero never renders empty on a fresh instance
        g.nodes = Array.from({ length: 9 }, (_, i) => ({ id: 'd' + i, name: 'a' + i, score: [92, 78, 64, 55, 41, 33, 22, 70, 48][i] }));
        g.edges = [[0, 1], [0, 3], [1, 2], [3, 4], [2, 5], [7, 0], [8, 3], [1, 8]].map(([a, b]) => ({ source: 'd' + a, target: 'd' + b, count: 1 }));
      }
      const h = genome(net, g, { cols: 46, rows: 18 });
      const c = h ? h.count() : { n: g.nodes.length, e: g.edges.length };
      $('#netN').textContent = String(c.n).padStart(3, '0');
      $('#netE').textContent = String(c.e).padStart(3, '0');
    } catch (e) { net.textContent = '  registry unreachable'; }
  })();
})();
