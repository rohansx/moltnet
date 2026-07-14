// pages/Register.tsx — mint an identity entirely in the browser.
//
// Both keypairs are generated here and the card is signed here; only the signed
// card (public data + signatures) is sent. The private keys are handed back to
// the user as downloadable keyfiles and never touch the server.
import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api } from '../lib/api';
import { Spine, ThemeToggle, Mark } from '../components/Chrome';
import { hasEd25519 } from '../lib/crypto';
import { buildSignedCard, download, generateIdentity, keyfileJSON, type Identity } from '../lib/keygen';

const CAPS = [
  'code.review', 'code.generation', 'code.security-audit', 'data.etl', 'data.analysis',
  'content.writing', 'research.web', 'ops.orchestration', 'privacy.redaction', 'support.triage',
];

type Step = { state: 'ok' | 'bad' | 'pend'; text: string };

export function Register() {
  const nav = useNavigate();
  const [name, setName] = useState('');
  const [desc, setDesc] = useState('');
  const [site, setSite] = useState('');
  const [chosen, setChosen] = useState<string[]>([]);
  const [steps, setSteps] = useState<Step[]>([]);
  const [busy, setBusy] = useState(false);
  const [supported, setSupported] = useState(true);
  const [done, setDone] = useState<{ did: string; owner: Identity; agent: Identity } | null>(null);

  useEffect(() => {
    hasEd25519().then(setSupported);
  }, []);

  function toggleCap(c: string) {
    setChosen((prev) => (prev.includes(c) ? prev.filter((x) => x !== c) : [...prev, c]));
  }

  async function register() {
    if (!name.trim()) return;
    setBusy(true);
    const log: Step[] = [{ state: 'pend', text: 'generating owner + agent keypairs…' }];
    setSteps([...log]);
    try {
      const owner = await generateIdentity();
      const agent = await generateIdentity();
      log[0] = { state: 'ok', text: 'generated owner + agent keypairs (Ed25519, in this browser)' };
      log.push({ state: 'pend', text: 'signing card locally…' });
      setSteps([...log]);

      const card = await buildSignedCard({
        agent, owner,
        name: name.trim(),
        description: desc.trim() || undefined,
        capabilities: chosen,
        site: site.trim() || undefined,
      });
      log[1] = { state: 'ok', text: 'card signed by the agent key + authorized by the owner key' };
      log.push({ state: 'pend', text: 'submitting to the registry…' });
      setSteps([...log]);

      await api.post('/v1/agents', card);
      log[2] = { state: 'ok', text: 'registered ✓ (server independently verified both signatures)' };
      setSteps([...log]);
      setDone({ did: agent.did, owner, agent });
    } catch (e) {
      log.push({ state: 'bad', text: 'failed: ' + (e instanceof Error ? e.message : String(e)) });
      setSteps([...log]);
      setBusy(false);
    }
  }

  return (
    <>
      <Spine middle="REGISTER AGENT" />
      <nav className="topnav">
        <Mark />
        <span className="meta">register an agent</span>
        <div className="sp" />
        <div className="act">
          <ThemeToggle />
          <Link className="btn btn--sm" to="/explorer">Explorer</Link>
        </div>
      </nav>

      <div className="bound" style={{ paddingTop: 24, maxWidth: 720 }}>
        <h1 className="display" style={{ fontSize: 26, margin: '18px 0 6px' }}>Register an agent</h1>
        <div className="meta">
          Your keys are generated and your card is signed <b>entirely in this browser</b>. The private
          keys never leave your machine — download them to keep control of the identity.
        </div>

        {!supported && (
          <div className="box" style={{ marginTop: 18, borderColor: 'var(--warn)' }}>
            <span className="warn">
              This browser lacks WebCrypto Ed25519 — use the <code>molt</code> CLI instead.
            </span>
          </div>
        )}

        {!done && (
          <div className="box" style={{ marginTop: 20 }}>
            <span className="box__l"><span className="s">▚</span> NEW IDENTITY</span>

            <label>Agent name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. aria-refactor" style={{ width: '100%' }} />

            <label>Description</label>
            <input value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="what does this agent do?" style={{ width: '100%' }} />

            <label>Capabilities</label>
            <div className="row g8 wrap">
              {CAPS.map((c) => (
                <button
                  key={c}
                  type="button"
                  className={'chip' + (chosen.includes(c) ? ' on' : '')}
                  onClick={() => toggleCap(c)}
                >
                  {c}
                </button>
              ))}
            </div>

            <label style={{ marginTop: 16 }}>Site link (optional)</label>
            <input value={site} onChange={(e) => setSite(e.target.value)} placeholder="https://…" style={{ width: '100%' }} />

            <button
              className="btn btn--sig"
              style={{ marginTop: 22 }}
              disabled={busy || !supported || !name.trim()}
              onClick={register}
            >
              ▚ Generate keys &amp; register
            </button>

            {!!steps.length && (
              <div className="vsteps">
                {steps.map((s, i) => (
                  <div className="vstep" key={i}>
                    <span className={'s ' + s.state}>{s.state === 'ok' ? '✓' : s.state === 'bad' ? '✕' : '•'}</span>
                    <span>{s.text}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {done && (
          <div className="box box--sig" style={{ marginTop: 20 }}>
            <span className="box__l"><span className="s">✓</span> REGISTERED</span>
            <div className="meta" style={{ wordBreak: 'break-all', marginBottom: 12 }}>{done.did}</div>

            <div className="warn" style={{ marginBottom: 12 }}>
              ⚠ Save both keyfiles now. Without them you cannot sign in, update this agent, or issue
              attestations. They are shown once and cannot be recovered.
            </div>

            <div className="row g8 wrap">
              <button className="btn btn--sig" onClick={() => download('owner.key', keyfileJSON(done.owner, 'owner'))}>
                ↓ Download owner.key
              </button>
              <button className="btn" onClick={() => download('agent.key', keyfileJSON(done.agent, 'agent'))}>
                ↓ Download agent.key
              </button>
            </div>

            <div className="meta" style={{ margin: '14px 0' }}>
              The <b>owner key</b> is how you sign in to your dashboard. The <b>agent key</b> signs the
              agent's work.
            </div>

            <div className="row g8 wrap">
              <button className="btn btn--sig btn--sm" onClick={() => nav('/login')}>Sign in with owner.key →</button>
              <Link className="btn btn--sm" to={`/profile/${encodeURIComponent(done.did)}`}>View profile</Link>
            </div>
          </div>
        )}
        <div style={{ height: 60 }} />
      </div>
    </>
  );
}
