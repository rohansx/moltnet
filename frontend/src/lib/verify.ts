// lib/verify.ts — trustless verification in the browser.
//
// This is the flagship: the registry is trusted only to move bytes. We re-check
// every signature with WebCrypto and recompute MoltScore locally from the raw
// attestation chain. These functions mirror core/canonical.go, core/crypto.go
// and score/score.go — keep them in step.

import type { Attestation, Card } from './api';

const B58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';

// WebCrypto's BufferSource requires an ArrayBuffer-backed view (not the
// SharedArrayBuffer-compatible default), so copy into a fresh ArrayBuffer.
function toBuf(a: ArrayLike<number>): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(new ArrayBuffer(a.length));
  out.set(a);
  return out;
}

function b58decode(str: string): Uint8Array<ArrayBuffer> {
  const bytes = [0];
  for (const ch of str) {
    const val = B58.indexOf(ch);
    if (val < 0) throw new Error('bad base58');
    let carry = val;
    for (let j = 0; j < bytes.length; j++) {
      carry += bytes[j] * 58;
      bytes[j] = carry & 0xff;
      carry >>= 8;
    }
    while (carry) {
      bytes.push(carry & 0xff);
      carry >>= 8;
    }
  }
  for (const ch of str) {
    if (ch === '1') bytes.push(0);
    else break;
  }
  return toBuf(bytes.reverse());
}

/** Recover the Ed25519 public key from a did:key (strips the 0xed01 multicodec). */
export function pubFromDID(did: string): Uint8Array<ArrayBuffer> {
  return toBuf(b58decode(did.replace('did:key:z', '')).slice(2));
}

function hexToBytes(h: string): Uint8Array<ArrayBuffer> {
  const a = new Uint8Array(new ArrayBuffer(h.length / 2));
  for (let i = 0; i < a.length; i++) a[i] = parseInt(h.substr(i * 2, 2), 16);
  return a;
}

/**
 * JCS-compatible canonical JSON — the exact bytes that were signed.
 * Mirrors core.Canonicalize: object keys sorted, no insignificant whitespace.
 */
export function canon(v: unknown): string {
  if (Array.isArray(v)) return '[' + v.map(canon).join(',') + ']';
  if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>;
    const keys = Object.keys(o).sort();
    return '{' + keys.map((k) => JSON.stringify(k) + ':' + canon(o[k])).join(',') + '}';
  }
  return JSON.stringify(v);
}

/** Canonicalize an object with some keys dropped (the signature fields). */
export function canonWithout(obj: object, drop: string[]): string {
  const c: Record<string, unknown> = { ...(obj as Record<string, unknown>) };
  for (const k of drop) delete c[k];
  return canon(c);
}

/**
 * Verify a hex Ed25519 signature against a message and a signer's did:key.
 * Returns null (not false) when the browser lacks WebCrypto Ed25519, so callers
 * can distinguish "couldn't check" from "check failed".
 */
export async function verifySig(did: string, message: string, sigHex: string): Promise<boolean | null> {
  try {
    const key = await crypto.subtle.importKey('raw', pubFromDID(did), { name: 'Ed25519' }, false, ['verify']);
    return await crypto.subtle.verify(
      { name: 'Ed25519' },
      key,
      hexToBytes(sigHex),
      new TextEncoder().encode(message),
    );
  } catch {
    return null;
  }
}

export interface ScoreInputs {
  completions: number;
  disputes: number;
  incidents: number;
  endorsements: number;
  receipts: number;
  distinct_issuers: number;
}

/**
 * Recompute MoltScore v1 locally from raw attestations — mirrors score/score.go.
 *   x = 1.0·ln(1+Σ decayed positives) + 0.6·ln(1+distinct issuers)
 *       − 1.2·disputes − 2.0·incidents − 2.0
 *   score = 100·σ(x)
 * Half-lives: 180d for positives, 365d for incidents.
 */
export function recomputeScore(atts: Attestation[]): { score: number; inputs: ScoreInputs } {
  const HL_POS = 180;
  const HL_INC = 365;
  const now = Date.now() / 1000;
  const decay = (iso: string, hl: number) => {
    const t = Date.parse(iso) / 1000;
    if (!t || t > now) return 1;
    return Math.pow(0.5, (now - t) / 86400 / hl);
  };

  let wc = 0;
  let wd = 0;
  let wi = 0;
  const issuers = new Set<string>();
  const inputs: ScoreInputs = {
    completions: 0,
    disputes: 0,
    incidents: 0,
    endorsements: 0,
    receipts: 0,
    distinct_issuers: 0,
  };

  for (const a of atts) {
    switch (a.type) {
      case 'task.completed':
        inputs.completions++;
        wc += decay(a.issued_at, HL_POS);
        issuers.add(a.issuer);
        break;
      case 'endorsement':
        inputs.endorsements++;
        wc += 0.25 * decay(a.issued_at, HL_POS);
        issuers.add(a.issuer);
        break;
      case 'payment.receipt':
        inputs.receipts++;
        wc += 0.5 * decay(a.issued_at, HL_POS);
        issuers.add(a.issuer);
        break;
      case 'task.disputed':
        inputs.disputes++;
        wd += decay(a.issued_at, HL_POS);
        break;
      case 'incident':
        inputs.incidents++;
        wi += decay(a.issued_at, HL_INC);
        break;
    }
  }
  inputs.distinct_issuers = issuers.size;

  const x = 1.0 * Math.log(1 + wc) + 0.6 * Math.log(1 + issuers.size) - 1.2 * wd - 2.0 * wi - 2.0;
  const score = Math.round(1000 / (1 + Math.exp(-x))) / 10;
  return { score, inputs };
}

export type StepState = 'ok' | 'bad' | 'pend';
export interface VerifyStep {
  state: StepState;
  text: string;
}

/**
 * The full local audit: card signatures, every attestation signature, and a
 * locally recomputed score. Nothing here trusts the registry's own numbers.
 */
export async function verifyAgent(card: Card, atts: Attestation[]): Promise<VerifyStep[]> {
  const steps: VerifyStep[] = [];

  const payload = canonWithout(card, ['sig', 'owner_sig']);
  const agentOK = await verifySig(card.id, payload, card.sig || '');
  const ownerOK = await verifySig(card.owner, payload, card.owner_sig || '');

  if (agentOK === null || ownerOK === null) {
    steps.push({ state: 'pend', text: 'card signatures — WebCrypto Ed25519 unavailable here; use `molt verify`' });
  } else {
    steps.push({ state: agentOK ? 'ok' : 'bad', text: `agent signature ${agentOK ? 'valid' : 'INVALID'}` });
    steps.push({ state: ownerOK ? 'ok' : 'bad', text: `owner signature ${ownerOK ? 'valid' : 'INVALID'}` });
  }

  let ok = 0;
  let bad = 0;
  let skipped = 0;
  for (const a of atts) {
    const res = await verifySig(a.issuer, canonWithout(a, ['sig']), a.sig || '');
    if (res === null) skipped++;
    else if (res) ok++;
    else bad++;
  }
  if (atts.length) {
    if (skipped === atts.length) {
      steps.push({ state: 'pend', text: `${atts.length} attestation signatures — not checkable in this browser` });
    } else {
      steps.push({
        state: bad ? 'bad' : 'ok',
        text: `${ok}/${atts.length} attestation signatures valid${bad ? `, ${bad} INVALID` : ''}`,
      });
    }
  }

  const rc = recomputeScore(atts);
  steps.push({
    state: 'ok',
    text: `MoltScore recomputed locally: ${rc.score.toFixed(1)} — completions ${rc.inputs.completions}, distinct issuers ${rc.inputs.distinct_issuers}`,
  });
  return steps;
}

/**
 * Recomputing from one agent's chain alone gives every issuer a weight of 1.0,
 * because a standalone verifier cannot know how trustworthy the issuers are.
 * The registry additionally weights each issuer by that issuer's OWN score (its
 * sybil defense), so the registry's published number is normally LOWER. Both are
 * "correct" — they answer different questions — and `molt verify` uses the same
 * uniform-weight basis this page does. Surfacing that keeps an honest gap from
 * looking like a bug.
 */
export const UNIFORM_WEIGHT_NOTE =
  'Recomputed with uniform issuer weights — the trustless default, and exactly what `molt verify` does. ' +
  'The registry also weights each issuer by their own score (anti-sybil), so its published figure is usually lower. ' +
  'The signatures above are the part that must be exact.';
