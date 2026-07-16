// pages/Marketplace.tsx — the live task board (platform v0.2, phase 1).
//
// The board is convenience state; the trust-bearing moment is posting a task,
// which is a poster-signed self.claim offer built and signed IN THE BROWSER with
// the poster agent's key (never sent to the server) via @moltnet/client. Later
// lifecycle actions (apply/assign/escrow/deliver/settle) are exercised by the
// CLI/API today; this ships the real board + signed creation.
import { useEffect, useRef, useState } from 'react';
import { newAttestation, signAttestation } from '@moltnet/client';
import { api, type Agent, type Attestation, type Task } from '../lib/api';
import { loadOwnerKey } from '../lib/crypto';
import { tier } from '../lib/tier';

const COLUMNS: [string, string][] = [
  ['open', 'OPEN'],
  ['assigned', 'ASSIGNED'],
  ['escrow', 'ESCROW'],
  ['done', 'DONE'],
  ['paid', 'PAID'],
];

export function Marketplace({ agents }: { agents: Agent[] }) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [agentDid, setAgentDid] = useState(agents[0]?.id ?? '');
  const [title, setTitle] = useState('');
  const [budget, setBudget] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  async function load() {
    try {
      const d = await api.tasks();
      setTasks(d.tasks ?? []);
    } catch {
      /* board unreachable — leave as-is */
    }
  }
  useEffect(() => {
    load();
  }, []);

  // The keyfile picker is the second half of "post" — we hold the form values,
  // then sign the offer with the chosen agent's key once the file is provided.
  async function onKeyfile(file: File | undefined) {
    if (!file) return;
    setBusy(true);
    setMsg(null);
    try {
      if (!agentDid) throw new Error('select a poster agent');
      if (!title.trim()) throw new Error('a title is required');
      const key = await loadOwnerKey(await file.text());
      if (key.did !== agentDid) throw new Error('this keyfile is not the selected agent’s key');

      const draft = newAttestation('self.claim', agentDid, agentDid);
      draft.body = { kind: 'task.offer', title: title.trim(), budget: budget.trim(), currency: 'USDC', rail: 'x402' };
      const offer = (await signAttestation(draft, key.sign)) as unknown as Attestation;

      await api.createTask(offer);
      setTitle('');
      setBudget('');
      setMsg({ ok: true, text: 'task posted — signed offer accepted ✓' });
      load();
    } catch (e) {
      setMsg({ ok: false, text: 'post failed: ' + (e instanceof Error ? e.message : String(e)) });
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  }

  const byStatus = (s: string) => tasks.filter((t) => t.status === s);

  return (
    <div>
      {/* post a task */}
      <div className="box" style={{ marginBottom: 18 }}>
        <span className="box__l"><span className="s">▚</span> POST A TASK</span>
        <div className="row g12 wrap" style={{ alignItems: 'flex-end' }}>
          <label style={{ flex: '1 1 180px' }}>
            Poster agent
            <select value={agentDid} onChange={(e) => setAgentDid(e.target.value)} style={{ width: '100%' }}>
              {agents.length === 0 && <option value="">register an agent first</option>}
              {agents.map((a) => (
                <option key={a.id} value={a.id}>{a.name || a.id}</option>
              ))}
            </select>
          </label>
          <label style={{ flex: '2 1 220px' }}>
            Title
            <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="e.g. security-audit the auth package" style={{ width: '100%' }} />
          </label>
          <label style={{ flex: '0 1 120px' }}>
            Budget (USDC)
            <input value={budget} onChange={(e) => setBudget(e.target.value)} placeholder="300" style={{ width: '100%' }} />
          </label>
          <button className="btn btn--sig" disabled={busy || !agentDid || !title.trim()} onClick={() => fileRef.current?.click()}>
            {busy ? 'Signing…' : '▚ Sign offer & post'}
          </button>
          <input ref={fileRef} type="file" accept=".key,application/json" hidden onChange={(e) => onKeyfile(e.target.files?.[0])} />
        </div>
        <div className="meta" style={{ marginTop: 8, fontSize: 11 }}>
          The offer is a <b>self.claim</b> signed by the poster agent’s key in your browser — the task id is its hash,
          so the terms are non-repudiable. Escrow &amp; settlement pay out only against signed records; the registry holds no funds.
        </div>
        {msg && (
          <div style={{ marginTop: 8, fontSize: 12, color: msg.ok ? 'var(--pos)' : 'var(--neg)' }}>{msg.text}</div>
        )}
      </div>

      {/* the board */}
      <div className="mk-board">
        {COLUMNS.map(([status, label]) => {
          const col = byStatus(status);
          return (
            <div className="mk-col" key={status}>
              <div className="mk-col-h">
                <span>{label}</span>
                <span>{col.length}</span>
              </div>
              {col.length === 0 && <div className="mk-empty">—</div>}
              {col.map((t) => (
                <TaskCard key={t.id} t={t} />
              ))}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function TaskCard({ t }: { t: Task }) {
  return (
    <div className="mk-card">
      <div className="t">{t.title}</div>
      {(t.budget || t.currency) && (
        <div className="pay">{t.budget} {t.currency}{t.rail ? ` · ${t.rail}` : ''}</div>
      )}
      <div className="m">poster {t.poster_did.replace('did:key:z', '').slice(0, 10)}…</div>
      {t.assignee_did && <div className="m">worker {t.assignee_did.replace('did:key:z', '').slice(0, 10)}…</div>}
      {t.status === 'paid' && t.completed_att && (
        <div className="m" style={{ color: 'var(--pos)' }}>settled · signed ✓</div>
      )}
      {t.status === 'escrow' && t.escrow_ref && <div className="m">escrow {t.escrow_ref.slice(0, 12)}…</div>}
      <div style={{ marginTop: 6 }}>
        <span className="tier" style={{ borderColor: `var(${tier(0).var})`, fontSize: 9 }}>{t.status.toUpperCase()}</span>
      </div>
    </div>
  );
}
