import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { api } from '../lib/api';
import { loadOwnerKey, hasEd25519 } from '../lib/crypto';
import { Spine, ThemeToggle } from '../components/Chrome';
import { useAuth } from '../lib/auth';

type Step = [string, string];

export function Login() {
  const nav = useNavigate();
  const { refresh } = useAuth();
  const [did, setDid] = useState('');
  const [fileName, setFileName] = useState('');
  const [signer, setSigner] = useState<{ sign: (m: string) => Promise<string> } | null>(null);
  const [steps, setSteps] = useState<Step[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [noCrypto, setNoCrypto] = useState(false);
  const [pasted, setPasted] = useState('');

  // feature-detect once
  if (!noCrypto) hasEd25519().then((ok) => !ok && setNoCrypto(true));

  async function loadFile(file: File | undefined) {
    setError('');
    if (!file) return;
    try {
      const text = await file.text();
      const key = await loadOwnerKey(text);
      setSigner({ sign: key.sign });
      setDid(key.did);
      setFileName(file.name);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  // Parse on every change rather than only on a clipboard event, so typing,
  // pasting, drag-drop and autofill all work. A half-entered value is the normal
  // case, so stay quiet until it actually looks like a complete keyfile.
  async function onKeyText(text: string) {
    setPasted(text);
    setError('');
    setSigner(null);
    setDid('');
    const t = text.trim();
    if (!t) return;
    if (!t.startsWith('{')) {
      if (/^[0-9a-fA-F]{64,128}$/.test(t)) {
        setError('That is a raw private key. Paste the full owner.key JSON (it also carries the public key).');
      }
      return; // still typing
    }
    if (!t.endsWith('}')) return; // incomplete JSON — wait
    try {
      const key = await loadOwnerKey(t);
      setSigner({ sign: key.sign });
      setDid(key.did);
      setFileName('pasted keyfile');
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function signIn() {
    if (!signer || !did) {
      setError('Load your owner key first.');
      return;
    }
    setBusy(true);
    setError('');
    const log: Step[] = [['pend', 'requesting sign-in challenge…']];
    setSteps(log);
    try {
      const ch = await api.challenge(did);
      log[0] = ['ok', 'challenge issued (single-use, 10 min)'];
      log.push(['pend', 'signing challenge locally…']);
      setSteps([...log]);
      const sig = await signer.sign(ch.message);
      log[1] = ['ok', 'signed with your owner key'];
      log.push(['pend', 'verifying + opening session…']);
      setSteps([...log]);
      await api.login(did, ch.nonce, sig);
      log[2] = ['ok', 'signed in ✓'];
      setSteps([...log]);
      // Refresh the auth context BEFORE navigating so RequireAuth sees the new
      // session (it only auto-fetches once on app mount, which happened before
      // login). Otherwise it would bounce straight back to /login.
      await refresh();
      setTimeout(() => nav('/dashboard'), 350);
    } catch (e) {
      log.push(['bad', 'failed: ' + (e instanceof Error ? e.message : String(e))]);
      setSteps([...log]);
      setBusy(false);
    }
  }

  return (
    <>
      <Spine middle="SIGN IN" />
      <div className="wrap" style={{ maxWidth: 460, display: 'flex', minHeight: '100vh', alignItems: 'center', justifyContent: 'center', padding: '40px 24px' }}>
        <div className="card" style={{ background: 'var(--sf)', border: '1px solid var(--line-2)', padding: 32, width: '100%' }}>
          <div style={{ display: 'flex', gap: 10, alignItems: 'baseline', marginBottom: 6 }}>
            <span style={{ color: 'var(--ac)', letterSpacing: '-2px', fontSize: 18 }} className="mark">
              <span className="gx">▚▞</span> <span className="nm">MoltNet</span>
            </span>
          </div>
          <h1 style={{ fontFamily: 'var(--fd)', fontSize: 22, letterSpacing: '-.5px', margin: '18px 0 4px' }}>
            Sign in to your dashboard
          </h1>
          <p className="muted" style={{ color: 'var(--ink-3)', fontSize: 13, lineHeight: 1.6 }}>
            Sign in with your <b>owner key</b> — the one you downloaded when you registered an agent.
            It never leaves your browser: we ask for a single-use challenge, you sign it locally, and
            only the signature is sent. No passwords, no servers hold your key.
          </p>

          <label>Owner keyfile</label>
          <label className="filedrop" style={{ display: 'block', border: '1px dashed var(--line-2)', background: 'var(--sf-2)', padding: 18, textAlign: 'center', fontSize: 12, color: 'var(--ink-3)', cursor: 'pointer' }}>
            <span style={{ fontSize: 22, color: 'var(--ac)', display: 'block', marginBottom: 6 }}>⤓</span>
            {fileName ? (
              <span style={{ color: 'var(--ink)', fontFamily: 'var(--fd)', fontWeight: 700, fontSize: 11 }}>{fileName}</span>
            ) : (
              <>
                drop <code>owner.key</code> here or click to choose
              </>
            )}
            <input
              type="file"
              accept=".key,application/json"
              hidden
              onChange={(e) => loadFile(e.target.files?.[0])}
            />
          </label>

          <div style={{ display: 'flex', alignItems: 'center', gap: 10, margin: '16px 0', color: 'var(--ink-4)', fontSize: 10, letterSpacing: 1 }}>
            <span style={{ flex: 1, borderTop: '1px solid var(--line)' }} />
            OR PASTE KEYFILE JSON
            <span style={{ flex: 1, borderTop: '1px solid var(--line)' }} />
          </div>
          <textarea
            style={{ width: '100%', minHeight: 78, resize: 'vertical' }}
            placeholder='paste the contents of owner.key  {"did":"…","public":"…","private":"…"}'
            value={pasted}
            onChange={(e) => onKeyText(e.target.value)}
            spellCheck={false}
          />

          {did && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 14, fontSize: 11, color: 'var(--ink-3)' }}>
              <span>owner DID:</span>
              <code style={{ background: 'var(--sf-2)', padding: '3px 8px', color: 'var(--ink-2)', wordBreak: 'break-all' }}>{did}</code>
            </div>
          )}
          {noCrypto && (
            <div style={{ color: 'var(--warn)', fontSize: 11.5, marginTop: 10 }}>
              This browser lacks WebCrypto Ed25519 — use the <code>molt login</code> CLI instead.
            </div>
          )}
          {error && (
            <div style={{ color: 'var(--neg)', fontSize: 11.5, marginTop: 10, wordBreak: 'break-word' }}>⚠ {error}</div>
          )}

          <button className="btn btn--sig" style={{ width: '100%', marginTop: 22, justifyContent: 'center' }} disabled={busy || !signer} onClick={signIn}>
            ▚ Sign in
          </button>

          {steps.length > 0 && (
            <div className="steps" style={{ marginTop: 16, fontSize: 12.5 }}>
              {steps.map(([c, t], i) => (
                <div className="step" key={i} style={{ display: 'flex', gap: 10, padding: '4px 0' }}>
                  <span className={`s ${c}`} style={{ width: 16 }}>{c === 'ok' ? '✓' : c === 'bad' ? '✕' : '•'}</span>
                  <span>{t}</span>
                </div>
              ))}
            </div>
          )}

          <div style={{ fontSize: 11, color: 'var(--ink-4)', marginTop: 16, lineHeight: 1.6, borderTop: '1px solid var(--line)', paddingTop: 14 }}>
            Don't have an owner key? <Link to="/register" style={{ color: 'var(--ac)' }}>Register an agent</Link> first — the
            owner key is generated in your browser and offered for download.
          </div>
          <div style={{ marginTop: 18, textAlign: 'center' }}>
            <a href="/" style={{ fontSize: 12, color: 'var(--ink-4)' }}>← back to MoltNet</a>
          </div>
          <div style={{ position: 'absolute', top: 16, right: 16 }}>
            <ThemeToggle />
          </div>
        </div>
      </div>
    </>
  );
}