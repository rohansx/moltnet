/**
 * @moltnet/client — dependency-light verification for MoltNet Agent Cards,
 * attestations and MoltScore. Runs in Node (>=20) and the browser via the
 * WebCrypto Ed25519 API. Mirrors the Go reference implementation in `core/` and
 * `score/`.
 *
 * Scope: verifies authenticity (Ed25519 signatures) and reproduces MoltScore v1
 * locally. Per-attestation hash-chain linkage (which requires BLAKE3) is left to
 * the registry / `molt verify`; this library covers signatures + scoring, which
 * are the core trust operations for a consumer deciding whether to invoke.
 */

export interface Capability { tag: string; desc?: string }

export interface Card {
  spec: string;
  id: string;
  name: string;
  owner: string;
  description?: string;
  version?: string;
  prev?: string;
  capabilities?: Capability[];
  protocols?: Record<string, unknown>;
  links?: Record<string, string>;
  created_at: string;
  sig?: string;
  owner_sig?: string;
  [k: string]: unknown;
}

export interface Attestation {
  spec?: string;
  type: string;
  subject: string;
  subject_card?: string;
  issuer: string;
  prev?: string;
  body?: Record<string, unknown>;
  issued_at: string;
  sig?: string;
  [k: string]: unknown;
}

// ---- Canonicalization (JCS-compatible; mirrors core/canonical.go) ----

export function canonicalize(v: unknown): string {
  if (Array.isArray(v)) return '[' + v.map(canonicalize).join(',') + ']';
  if (v && typeof v === 'object') {
    const obj = v as Record<string, unknown>;
    const keys = Object.keys(obj).sort();
    return '{' + keys.map((k) => JSON.stringify(k) + ':' + canonicalize(obj[k])).join(',') + '}';
  }
  return JSON.stringify(v);
}

export function canonicalizeWithout(v: Record<string, unknown>, dropKeys: string[]): string {
  const clone: Record<string, unknown> = { ...v };
  for (const k of dropKeys) delete clone[k];
  return canonicalize(clone);
}

// ---- did:key <-> Ed25519 public key ----

const B58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';

function b58encode(bytes: Uint8Array): string {
  const digits: number[] = [0];
  for (const b of bytes) {
    let carry = b;
    for (let j = 0; j < digits.length; j++) {
      carry += digits[j] << 8;
      digits[j] = carry % 58;
      carry = (carry / 58) | 0;
    }
    while (carry) { digits.push(carry % 58); carry = (carry / 58) | 0; }
  }
  let str = '';
  for (const b of bytes) { if (b === 0) str += '1'; else break; }
  for (let k = digits.length - 1; k >= 0; k--) str += B58[digits[k]];
  return str;
}

function b58decode(str: string): Uint8Array {
  const bytes: number[] = [0];
  for (const ch of str) {
    const val = B58.indexOf(ch);
    if (val < 0) throw new Error('invalid base58 character: ' + ch);
    let carry = val;
    for (let j = 0; j < bytes.length; j++) {
      carry += bytes[j] * 58;
      bytes[j] = carry & 0xff;
      carry >>= 8;
    }
    while (carry) { bytes.push(carry & 0xff); carry >>= 8; }
  }
  for (const ch of str) { if (ch === '1') bytes.push(0); else break; }
  return new Uint8Array(bytes.reverse());
}

/** Encode an Ed25519 public key as a did:key DID. */
export function didFromPublicKey(pub: Uint8Array): string {
  const buf = new Uint8Array(2 + pub.length);
  buf[0] = 0xed; buf[1] = 0x01; // multicodec ed25519-pub varint
  buf.set(pub, 2);
  return 'did:key:z' + b58encode(buf);
}

/** Recover an Ed25519 public key from a did:key DID. */
export function publicKeyFromDid(did: string): Uint8Array {
  const prefix = 'did:key:z';
  if (!did.startsWith(prefix)) throw new Error('not a did:key with base58btc encoding: ' + did);
  const raw = b58decode(did.slice(prefix.length));
  if (raw.length !== 34 || raw[0] !== 0xed || raw[1] !== 0x01) {
    throw new Error('not an ed25519 did:key: ' + did);
  }
  return raw.slice(2);
}

// ---- Signature verification (WebCrypto Ed25519) ----

function hexToBytes(h: string): Uint8Array {
  const a = new Uint8Array(h.length / 2);
  for (let i = 0; i < a.length; i++) a[i] = parseInt(h.substr(i * 2, 2), 16);
  return a;
}

/** Copy into a fresh ArrayBuffer so WebCrypto's BufferSource types are satisfied. */
function toBuf(u: Uint8Array): ArrayBuffer {
  const out = new ArrayBuffer(u.byteLength);
  new Uint8Array(out).set(u);
  return out;
}

/** Verify a hex Ed25519 signature over `message` against the signer's DID. */
export async function verifySignature(signerDid: string, message: string, sigHex: string): Promise<boolean> {
  try {
    const pub = publicKeyFromDid(signerDid);
    const key = await crypto.subtle.importKey('raw', toBuf(pub), { name: 'Ed25519' }, false, ['verify']);
    return await crypto.subtle.verify(
      { name: 'Ed25519' }, key, toBuf(hexToBytes(sigHex)), toBuf(new TextEncoder().encode(message))
    );
  } catch {
    return false;
  }
}

/** Verify a card's agent and owner signatures. */
export async function verifyCard(card: Card): Promise<boolean> {
  if (card.spec !== 'moltnet/card/v0.1' || !card.sig || !card.owner_sig) return false;
  const payload = canonicalizeWithout(card as Record<string, unknown>, ['sig', 'owner_sig']);
  const agentOk = await verifySignature(card.id, payload, card.sig);
  const ownerOk = await verifySignature(card.owner, payload, card.owner_sig);
  return agentOk && ownerOk;
}

/** Verify an attestation's issuer signature. */
export async function verifyAttestation(att: Attestation): Promise<boolean> {
  if (!att.sig) return false;
  const payload = canonicalizeWithout(att as Record<string, unknown>, ['sig']);
  return verifySignature(att.issuer, payload, att.sig);
}

// ---- MoltScore v1 (mirrors score/score.go) ----

export interface ScoreInputs {
  completions: number; disputes: number; incidents: number;
  endorsements: number; receipts: number; distinct_issuers: number;
}
export interface ScoreOutput { algorithm: string; score: number; inputs: ScoreInputs }

const HALF_LIFE_POS = 180, HALF_LIFE_INC = 365;

function decay(issuedAt: string, nowSec: number, halfLifeDays: number): number {
  const t = Date.parse(issuedAt) / 1000;
  if (!t || t > nowSec) return 1;
  const days = (nowSec - t) / 86400;
  return Math.pow(0.5, days / halfLifeDays);
}

/**
 * Compute MoltScore v1. With no issuerWeights every issuer weighs 1.0 (the
 * correct trustless default); with a weights map, unknown issuers weigh 0.25.
 */
export function computeScore(
  atts: Attestation[],
  issuerWeights: Record<string, number> | null = null,
  ownerOf: Record<string, string> | null = null,
  now: Date = new Date()
): ScoreOutput {
  const nowSec = now.getTime() / 1000;
  const weightOf = (issuer: string): number =>
    issuerWeights == null ? 1.0 : (issuer in issuerWeights ? issuerWeights[issuer] : 0.25);

  // Independence rule (mirrors score/score.go): drop any attestation whose issuer
  // shares an owner with the subject. Passing ownerOf=null disables it — the
  // trustless uniform basis a standalone verifier uses.
  const subjectOwner = ownerOf != null && atts.length > 0 ? ownerOf[atts[0].subject] : undefined;

  let wc = 0, wd = 0, wi = 0;
  const inputs: ScoreInputs = { completions: 0, disputes: 0, incidents: 0, endorsements: 0, receipts: 0, distinct_issuers: 0 };
  const issuers = new Set<string>();

  for (const a of atts) {
    if (subjectOwner !== undefined && ownerOf![a.issuer] === subjectOwner) continue; // self-dealing
    const iw = weightOf(a.issuer);
    switch (a.type) {
      case 'task.completed': inputs.completions++; wc += iw * decay(a.issued_at, nowSec, HALF_LIFE_POS); issuers.add(a.issuer); break;
      case 'endorsement': inputs.endorsements++; wc += 0.25 * iw * decay(a.issued_at, nowSec, HALF_LIFE_POS); issuers.add(a.issuer); break;
      case 'payment.receipt': inputs.receipts++; wc += 0.5 * iw * decay(a.issued_at, nowSec, HALF_LIFE_POS); issuers.add(a.issuer); break;
      case 'task.disputed': inputs.disputes++; wd += iw * decay(a.issued_at, nowSec, HALF_LIFE_POS); break;
      case 'incident': inputs.incidents++; wi += iw * decay(a.issued_at, nowSec, HALF_LIFE_INC); break;
      // self.claim and key.rotation contribute nothing.
    }
  }
  inputs.distinct_issuers = issuers.size;

  const x = 1.0 * Math.log(1 + wc) + 0.6 * Math.log(1 + issuers.size) - 1.2 * wd - 2.0 * wi - 2.0;
  const score = Math.round((100 / (1 + Math.exp(-x))) * 10) / 10;
  return { algorithm: 'moltscore/v1', score, inputs };
}

// ---- High-level: verify before invoke ----

export interface VerifyResult {
  verified: boolean;
  cardOk: boolean;
  attestationsOk: boolean;
  moltscore: number;
  inputs: ScoreInputs;
  attestationCount: number;
}

/**
 * Fetch an agent's card and attestation chain from a registry, verify every
 * signature, and recompute MoltScore locally — trusting the registry only for
 * transport. This is the "verify before invoke" primitive.
 */
export async function verifyAgent(
  registryUrl: string,
  did: string,
  fetchImpl: typeof fetch = fetch
): Promise<VerifyResult> {
  const base = registryUrl.replace(/\/$/, '');
  const agentResp = await (await fetchImpl(`${base}/v1/agents/${encodeURIComponent(did)}`)).json();
  const card: Card = agentResp.card;
  const attResp = await (await fetchImpl(`${base}/v1/agents/${encodeURIComponent(did)}/attestations?limit=500`)).json();
  const atts: Attestation[] = attResp.attestations || [];

  const cardOk = card ? await verifyCard(card) : false;
  let attestationsOk = true;
  for (const a of atts) {
    if (!(await verifyAttestation(a))) { attestationsOk = false; break; }
  }
  const out = computeScore(atts, null);
  return {
    verified: cardOk && attestationsOk,
    cardOk,
    attestationsOk,
    moltscore: out.score,
    inputs: out.inputs,
    attestationCount: atts.length,
  };
}

// ---- Signing + posting (the piece browser/CLI clients were missing) ----

export type Signer = (message: string) => Promise<string>;

/**
 * Sign an attestation: canonicalize it WITHOUT `sig` (the exact bytes the server
 * re-checks), sign them, and return a copy with `sig` set. The signer maps a
 * canonical string to a hex Ed25519 signature — a WebCrypto key in the browser
 * or a molt keyfile in Node. The library could verify but never sign; this
 * closes that gap for every write path (settlement, consent, audit).
 */
export async function signAttestation(att: Attestation, sign: Signer): Promise<Attestation> {
  const payload = canonicalizeWithout(att as Record<string, unknown>, ['sig']);
  const sig = await sign(payload);
  return { ...att, sig };
}

/**
 * Post an attestation, chaining it onto the issuer's current head. The registry
 * enforces prev == IssuerHead and 409s a stale prev, so a hot signer that races
 * other posts must refetch the head and re-sign. `att.issuer` must be set; `prev`
 * and `sig` are (re)computed here on each attempt.
 */
export async function postAttestation(
  registryUrl: string,
  att: Attestation,
  sign: Signer,
  fetchImpl: typeof fetch = fetch,
  maxRetries = 3,
): Promise<{ hash: string }> {
  const base = registryUrl.replace(/\/$/, '');
  for (let attempt = 0; ; attempt++) {
    const headResp = await fetchImpl(`${base}/v1/issuers/${encodeURIComponent(att.issuer)}/head`);
    const head = headResp.ok ? (((await headResp.json()) as { head?: string }).head ?? '') : '';
    const signed = await signAttestation({ ...att, prev: head }, sign);
    const resp = await fetchImpl(`${base}/v1/attestations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(signed),
    });
    if (resp.ok) return (await resp.json()) as { hash: string };
    if (resp.status === 409 && attempt < maxRetries) continue; // head advanced — refetch + re-sign
    throw new Error(`post attestation failed: ${resp.status} ${await resp.text()}`);
  }
}
