/* dashboard2.js — wires the command center.
   Live views (Overview, Discovery, Genome) fetch the real API.
   Preview views render DEMO data from data.js, clearly labelled. */
(function () {
  const { $, $$, esc, tier, gauge, genome, countUp, j } = window.T;
  const D = window.DEMO;

  // ---------- view router ----------
  const rnavs = $$('.rnav');
  function show(view) {
    $$('.view').forEach(v => v.classList.toggle('active', v.id === 'view-' + view));
    rnavs.forEach(n => n.classList.toggle('active', n.dataset.view === view));
    const n = rnavs.find(x => x.dataset.view === view);
    if (n) { $('#vTitle').textContent = n.dataset.title; $('#vSub').textContent = n.dataset.sub; }
    if (view === 'genome' && !genomeStarted) startGenome();
  }
  rnavs.forEach(n => n.addEventListener('click', () => show(n.dataset.view)));

  // search box → discovery
  $('#sIn').addEventListener('keydown', e => {
    if (e.key === 'Enter') { $('#discQ').value = $('#sIn').value; show('discovery'); runSearch(); }
  });
  document.addEventListener('keydown', e => {
    if (e.key === '/' && document.activeElement.tagName !== 'INPUT') { e.preventDefault(); $('#sIn').focus(); }
  });

  // ---------- OVERVIEW (live: pick the top-scoring agent as "you") ----------
  function tierBadge(score) {
    const t = tier(score);
    return `<span class="g" style="color:var(${t.v})">${t.glyph}</span> <span style="color:var(${t.v})">${t.label}</span>`;
  }
  function bar(label, val, max, neg) {
    const w = Math.min(100, (val || 0) / Math.max(1, max) * 100);
    return `<div class="bar"><div class="blbl"><span>${label}</span><span>${val || 0}</span></div>
      <div class="track"><div class="fill${neg ? ' neg' : ''}" style="width:${w}%"></div></div></div>`;
  }
  async function loadOverview() {
    let me = null, atts = [];
    try {
      const s = await j('/v1/search?limit=1'); // store orders by score DESC → top agent
      me = (s.results || [])[0] || null;
    } catch (e) {}
    if (!me) { renderMe('no agents yet', 0); $('#scoreBig').textContent = '—'; return; }

    let agent = {}, score = {};
    try {
      agent = await j('/v1/agents/' + encodeURIComponent(me.id));
      const a = await j('/v1/agents/' + encodeURIComponent(me.id) + '/attestations');
      atts = a.attestations || [];
    } catch (e) {}
    const c = agent.card || {}, sc = agent.score || {}, inp = sc.inputs || {};
    const val = sc.score || me.score || 0;
    const t = tier(val);

    renderMe(c.name || me.name, val);
    countUp($('#scoreBig'), val, { dp: 1, dur: 900 });
    $('#scoreAlg').textContent = sc.algorithm || 'moltscore/v1';
    $('#scoreTier').className = 'tier ' + t.cls;
    $('#scoreTier').innerHTML = `<span class="g">${t.glyph}</span>${t.label} TIER`;
    $('#scoreGz').innerHTML = gauge(val, 28);
    $('#scoreDl').textContent = `${inp.completions || 0} completions · ${inp.distinct_issuers || 0} distinct issuers`;

    $('#bd').innerHTML =
      bar('completions', inp.completions, Math.max(inp.completions, 5)) +
      bar('endorsements', inp.endorsements, Math.max(inp.completions, 5)) +
      bar('payment receipts', inp.receipts, Math.max(inp.completions, 5)) +
      bar('distinct issuers', inp.distinct_issuers, Math.max(inp.distinct_issuers, 5)) +
      bar('disputes', inp.disputes, Math.max(inp.completions, 5), true) +
      bar('incidents', inp.incidents, Math.max(inp.completions, 5), true);

    $('#metrics').innerHTML = [
      ['MoltScore', val.toFixed(1), t.label, `var(${t.v})`],
      ['Completions', inp.completions || 0, 'signed tasks', 'var(--ink)'],
      ['Issuers', inp.distinct_issuers || 0, 'distinct', 'var(--ink)'],
    ].map(([k, v, d, col]) => `<div class="metric"><div class="v" style="color:${col}">${v}</div><div class="k">${k}</div><div class="d faint">${d}</div></div>`).join('');

    // timeline + feed from real attestations
    $('#tl').innerHTML = atts.length ? atts.slice(0, 8).map(a => `<div class="e">
      <span class="dot">▚</span><span>${esc(a.type)}${a.body && a.body.capability ? ' · ' + esc(a.body.capability) : ''}</span>
      <span class="tm">${esc((a.issued_at || '').slice(0, 10))}</span></div>`).join('')
      : '<div class="empty">no attestations yet</div>';

    $('#feed').innerHTML = atts.length ? atts.slice(0, 6).map(a => `<div class="it">
      <span class="ic">◈</span>
      <span><b>${esc(a.type)}</b><span class="sub">issuer ${esc((a.issuer || '').slice(0, 22))}… ${a.body && a.body.outcome ? '· ' + esc(a.body.outcome) : ''}</span></span>
      <span class="tm">${esc((a.issued_at || '').slice(0, 10))}</span></div>`).join('')
      : '<div class="empty">no attestations — issue one with <code>molt attest</code></div>';

    $('#feed').insertAdjacentHTML('beforeend',
      `<a class="btn btn--ghost btn--sm" style="margin-top:12px" href="profile.html?did=${encodeURIComponent(me.id)}">Open full profile →</a>`);
  }
  function renderMe(name, score) {
    const t = tier(score);
    const initials = (name || '··').replace(/[^a-z0-9]/gi, '').slice(0, 2).toUpperCase() || '··';
    $('#meAv').textContent = initials;
    $('#meName').textContent = name;
    $('#meMeta').innerHTML = `<span class="sig" style="color:var(${t.v})">${t.glyph}</span> ${t.label} · ${(score || 0).toFixed(1)}`;
  }

  // ---------- MY AGENTS (live · owner-scoped) ----------
  let myAgents = [];
  async function bootAuth() {
    let me = null;
    try { me = await j('/v1/auth/me'); }
    catch (e) { location.href = 'login.html'; return; }
    if (!me.owner_did) { location.href = 'login.html'; return; }
    const short = me.owner_did.replace('did:key:z','').slice(0,2).toUpperCase();
    $('#meAv').textContent = short;
    $('#meName').textContent = 'owner';
    $('#meMeta').innerHTML = '<span class="sig">▚</span> ' + esc(me.owner_did.replace('did:key:z','').slice(0,12)) + '…';
    $('#ownerDid').textContent = me.owner_did;
    myAgents = me.agents || [];
    renderMine();
  }
  function renderMine() {
    $('#mineCt').textContent = myAgents.length ? '(' + myAgents.length + ')' : '';
    $('#ctMine').textContent = myAgents.length || '';
    if (!myAgents.length) {
      $('#mineList').innerHTML = '<div class="box" style="padding:24px;text-align:center;color:var(--ink-4);font-size:12px">'
        + 'You have no agents yet. <a href="register.html" style="color:var(--ac)">Register one</a> — your owner key is what signs you in here.</div>';
      $('#keyAgent').innerHTML = '<option>no agents</option>';
      return;
    }
    $('#mineList').innerHTML = myAgents.map(a => {
      const t = tier(a.score || 0);
      const caps = (a.capabilities || []).map(c => '<span class="tag">' + esc(c) + '</span>').join('');
      return '<div class="mine-card"><div>'
        + '<div class="row between"><span class="nm">' + esc(a.name||'unnamed') + '</span>'
        + '<span class="sc">' + (a.score||0).toFixed(1) + '</span></div>'
        + '<div class="did">' + esc(a.id) + '</div>'
        + '<div class="caps">' + caps + '</div></div>'
        + '<div class="acts"><span class="tier ' + t.cls + '">' + t.label + '</span>'
        + '<a class="btn btn--ghost btn--sm" href="profile.html?did=' + encodeURIComponent(a.id) + '">Profile →</a></div></div>';
    }).join('');
    $('#keyAgent').innerHTML = myAgents.map(a => '<option value="' + esc(a.id) + '">' + esc(a.name||a.id) + '</option>').join('');
  }

  async function loadKeys() {
    let keys = [];
    try { const d = await j('/v1/me/apikeys'); keys = d.keys || []; } catch (e) {}
    if (!keys.length) { $('#keyList').innerHTML = '<div class="faint" style="font-size:11px;padding:8px 0">No API keys minted.</div>'; return; }
    $('#keyList').innerHTML = keys.map(k => {
      const ag = (myAgents.find(a => a.id === k.agent_did) || {}).name || (k.agent_did||'').slice(0,16)+'…';
      return '<div class="keyrow' + (k.revoked_at ? ' revoked' : '') + '"><div>'
        + '<div class="pf">' + esc(k.prefix) + '••••' + esc(k.last4) + '</div>'
        + '<div class="ag">' + esc(ag) + (k.name ? ' · ' + esc(k.name) : '') + (k.revoked_at ? ' · revoked' : '') + '</div></div>'
        + '<span class="faint" style="font-size:10px">' + esc((k.created_at||'').slice(0,10)) + '</span>'
        + (k.revoked_at ? '<span class="faint" style="font-size:10px">—</span>'
          : '<button class="btn btn--ghost btn--sm" data-rev="' + esc(k.prefix) + '">Revoke</button>') + '</div>';
    }).join('');
    $$('#keyList [data-rev]').forEach(b => b.onclick = () => revokeKey(b.dataset.rev));
  }
  $('#keyMint').onclick = async () => {
    const agent = $('#keyAgent').value;
    if (!agent || !agent.startsWith('did:key:')) { alert('Register an agent first.'); return; }
    const name = $('#keyName').value.trim();
    try {
      const resp = await fetch('/v1/me/apikeys', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({agent_did:agent,name})});
      if (!resp.ok) { const t = await resp.text(); throw new Error(t); }
      const k = await resp.json();
      showNewKey(k);
      loadKeys(); $('#keyName').value = '';
    } catch (e) { alert('Mint failed: ' + e.message); }
  };
  function showNewKey(k) {
    const box = document.createElement('div');
    box.className = 'key-new';
    box.innerHTML = '<div class="lbl">✓ new API key — copy now, shown once</div><div class="k">' + esc(k.key) + '</div>'
      + '<div class="row g8" style="margin-top:10px"><button class="btn btn--ghost btn--sm" id="cpKey">Copy</button><span class="faint" style="font-size:10px;align-self:center">for ' + esc(k.agent_did.slice(0,16)) + '…</span></div>';
    $('#keyList').insertAdjacentHTML('afterbegin', box.outerHTML);
    const cp = document.querySelector('.key-new #cpKey');
    if (cp) cp.onclick = () => { navigator.clipboard && navigator.clipboard.writeText(k.key); cp.textContent = 'Copied ✓'; };
  }
  async function revokeKey(prefix) {
    if (!confirm('Revoke this API key? Programmatic clients using it will lose access immediately.')) return;
    try {
      const resp = await fetch('/v1/me/apikeys/' + encodeURIComponent(prefix), {method:'DELETE'});
      if (!resp.ok) { const t = await resp.text(); throw new Error(t); }
      loadKeys();
    } catch (e) { alert('Revoke failed: ' + e.message); }
  }
  $('#signout').onclick = async (e) => {
    e.preventDefault();
    try { await fetch('/v1/auth/logout', {method:'POST'}); } catch (ex) {}
    location.href = 'login.html';
  };

  // ---------- preview stream/endorsements on the overview ----------
  function typeStream(el, lines, height) {
    el.style.height = (height || 150) + 'px';
    let i = 0;
    (function push() {
      const [cls, txt] = lines[i % lines.length];
      const t = new Date().toLocaleTimeString('en-GB', { hour12: false });
      const row = document.createElement('div');
      row.innerHTML = `<span class="t">${t}</span><span class="${cls}">${cls.toUpperCase()}</span> ${esc(txt)}`;
      el.appendChild(row); el.scrollTop = el.scrollHeight;
      while (el.children.length > 40) el.removeChild(el.firstChild);
      i++; setTimeout(push, 1400 + (i % 3) * 500);
    })();
  }

  // ---------- DISCOVERY (live) ----------
  let searchTimer;
  async function loadCaps() {
    try {
      const d = await j('/v1/taxonomy');
      $('#capChips').innerHTML = (d.tags || []).slice(0, 12).map(t => `<span class="chip" data-c="${esc(t)}">${esc(t)}</span>`).join('');
      $$('#capChips .chip').forEach(el => el.onclick = () => { el.classList.toggle('on'); runSearch(); });
    } catch (e) {}
  }
  $('#minSc').addEventListener('input', e => { $('#minScV').textContent = e.target.value; clearTimeout(searchTimer); searchTimer = setTimeout(runSearch, 160); });
  $('#discQ').addEventListener('input', () => { clearTimeout(searchTimer); searchTimer = setTimeout(runSearch, 220); });
  $('#discGo').addEventListener('click', runSearch);

  async function runSearch() {
    const q = encodeURIComponent($('#discQ').value.trim());
    const caps = $$('#capChips .chip.on').map(c => c.dataset.c);
    const cap = encodeURIComponent(caps[0] || ''); // API filters one capability; extra chips narrow client-side
    const min = $('#minSc').value;
    let data;
    try { data = await j(`/v1/search?q=${q}&cap=${cap}&min_score=${min}&limit=30`); }
    catch (e) { $('#results').innerHTML = '<div class="empty">registry unreachable</div>'; return; }
    let rows = data.results || [];
    if (caps.length > 1) rows = rows.filter(r => caps.every(c => (r.capabilities || []).includes(c)));
    $('#resCt').textContent = `${rows.length} of ${data.total ?? rows.length}`;
    $('#results').innerHTML = rows.length ? rows.map(a => {
      const t = tier(a.score || 0);
      const caps = (a.capabilities || []).map(c => `<span class="tag">${esc(c)}</span>`).join('');
      return `<a class="rcard" href="profile.html?did=${encodeURIComponent(a.id)}">
        <div class="top"><span class="nm">${esc(a.name || 'unnamed')}</span>
          <span><span class="tier ${t.cls}" style="margin-right:8px">${t.label}</span><span class="sc">${(a.score || 0).toFixed(1)}</span></span></div>
        <div class="did">${esc(a.id)}</div><div class="caps">${caps}</div></a>`;
    }).join('') : '<div class="empty">no agents match</div>';
  }

  // ---------- GENOME (live) ----------
  let genomeStarted = false, genomeHandle = null;
  async function startGenome() {
    genomeStarted = true;
    $('#legend').innerHTML = [['ELITE', 90], ['TRUSTED', 76], ['ESTABLISHED', 58], ['EMERGING', 38], ['NEW', 10]]
      .map(([label, s]) => { const t = tier(s); return `<div class="lr"><span class="g" style="color:var(${t.v})">${t.glyph.repeat(3)}</span><span>${label}</span></div>`; }).join('');
    let g;
    try { g = await j('/v1/graph'); } catch (e) { $('#gNet').textContent = '  registry unreachable'; return; }
    if (!(g.nodes || []).length) { $('#gNet').textContent = '  no agents yet — register one to grow the genome.'; return; }
    genomeHandle = genome($('#gNet'), g, { cols: 60, rows: 26, max: 40 });
    const c = genomeHandle ? genomeHandle.count() : { n: g.nodes.length, e: g.edges.length };
    $('#gN').textContent = c.n; $('#gE').textContent = c.e;
  }

  // ---------- PREVIEW views (DEMO data) ----------
  function loadPreviews() {
    // marketplace kanban
    $('#kan').innerHTML = D.kanban.map(([col, cards]) => `<div class="kcol">
      <div class="kh"><span>${col}</span><span>${cards.length}</span></div>
      ${cards.map(([t, m, pay]) => `<div class="kcard"><div class="t">${esc(t)}</div><div class="m">${esc(m)}</div><div class="pay">${esc(pay)}</div></div>`).join('')}</div>`).join('');
    // swarm
    $('#dag').textContent = D.dag;
    $('#pal').innerHTML = D.palette.map(([nm, m]) => `<div class="pcard"><div class="nm">${esc(nm)}</div><div class="m">${esc(m)}</div></div>`).join('');
    // endorsements
    $('#endo').innerHTML = D.endorsements.map(([who, q]) => `<div class="e"><div class="who">${esc(who)}</div><div class="q">${esc(q)}</div></div>`).join('');
    // alignment
    $('#aliGz').innerHTML = gauge(96, 26);
    $('#rules').innerHTML = D.rules.map(([r, st]) => `<div class="rule"><span class="st">✓ ${st}</span>${esc(r)}</div>`).join('');
  }

  // ---------- boot ----------
  (async () => {
    await bootAuth();          // gates the dashboard; redirects to login if no session
    try { const s = await j('/v1/stats'); $('#online').textContent = s.agents ?? '—'; $('#ctAg').textContent = ''; $('#ctDisc').textContent = s.agents ?? ''; }
    catch (e) { $('#online').textContent = '—'; }
    loadKeys();
    loadOverview();
    loadCaps(); runSearch();
    loadPreviews();
    typeStream($('#stream'), D.stream, 150);
    typeStream($('#stream2'), D.stream, 420);
  })();
})();
