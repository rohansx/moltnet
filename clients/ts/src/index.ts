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
  anchors?: Record<string, unknown>;
  links?: Record<string, string>;
  created_at: string;
  sig?: string;
  owner_sig?: string;
  [k: string]: unknown;
}

/** A parsed, validated ERC-8004 on-chain identity anchor (mirrors core/anchor.go). */
export interface ERC8004Anchor {
  protocol: 'erc8004';
  chain: string;      // CAIP-2, always eip155:<n>
  registry: string;   // Identity Registry address, EIP-55 checksummed
  agentId: string;    // on-chain agent id (uint256) as a decimal string
  caip10: string;     // <chain>:<registry>
  ref: string;        // <chain>:<registry>/<agentId>
  tx?: string;
  cardUri?: string;
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
  now: Date = new Date()
): ScoreOutput {
  const nowSec = now.getTime() / 1000;
  const weightOf = (issuer: string): number =>
    issuerWeights == null ? 1.0 : (issuer in issuerWeights ? issuerWeights[issuer] : 0.25);

  let wc = 0, wd = 0, wi = 0;
  const inputs: ScoreInputs = { completions: 0, disputes: 0, incidents: 0, endorsements: 0, receipts: 0, distinct_issuers: 0 };
  const issuers = new Set<string>();

  for (const a of atts) {
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

// ---- Keccak-256 (Ethereum padding) — for EIP-55 address checksums ----
// A dependency-free port of core/keccak.go. Only used on short inputs
// (addresses), so BigInt lanes are fine and clarity beats speed.

const KECCAK_RC: bigint[] = [
  0x0000000000000001n, 0x0000000000008082n, 0x800000000000808an, 0x8000000080008000n,
  0x000000000000808bn, 0x0000000080000001n, 0x8000000080008081n, 0x8000000000008009n,
  0x000000000000008an, 0x0000000000000088n, 0x0000000080008009n, 0x000000008000000an,
  0x000000008000808bn, 0x800000000000008bn, 0x8000000000008089n, 0x8000000000008003n,
  0x8000000000008002n, 0x8000000000000080n, 0x000000000000800an, 0x800000008000000an,
  0x8000000080008081n, 0x8000000000008080n, 0x0000000080000001n, 0x8000000080008008n,
];
const KECCAK_ROTC = [1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 2, 14, 27, 41, 56, 8, 25, 43, 62, 18, 39, 61, 20, 44];
const KECCAK_PILN = [10, 7, 11, 17, 18, 3, 5, 16, 8, 21, 24, 4, 15, 23, 19, 13, 12, 2, 20, 14, 22, 9, 6, 1];
const MASK64 = (1n << 64n) - 1n;

function rotl64(x: bigint, n: number): bigint {
  const s = BigInt(n);
  return ((x << s) | (x >> (64n - s))) & MASK64;
}

function keccakF(a: bigint[]): void {
  for (let round = 0; round < 24; round++) {
    const c: bigint[] = new Array(5);
    for (let i = 0; i < 5; i++) c[i] = a[i] ^ a[i + 5] ^ a[i + 10] ^ a[i + 15] ^ a[i + 20];
    for (let i = 0; i < 5; i++) {
      const t = c[(i + 4) % 5] ^ rotl64(c[(i + 1) % 5], 1);
      for (let j = 0; j < 25; j += 5) a[j + i] ^= t;
    }
    let t = a[1];
    for (let i = 0; i < 24; i++) {
      const j = KECCAK_PILN[i];
      const bc0 = a[j];
      a[j] = rotl64(t, KECCAK_ROTC[i]);
      t = bc0;
    }
    for (let j = 0; j < 25; j += 5) {
      const bc = [a[j], a[j + 1], a[j + 2], a[j + 3], a[j + 4]];
      for (let i = 0; i < 5; i++) a[j + i] = bc[i] ^ (~bc[(i + 1) % 5] & MASK64 & bc[(i + 2) % 5]);
    }
    a[0] ^= KECCAK_RC[round];
  }
}

/** Keccak-256 (as used by Ethereum), returned as 32 raw bytes. */
export function keccak256(data: Uint8Array): Uint8Array {
  const rate = 136;
  const a: bigint[] = new Array(25).fill(0n);
  const absorb = (block: Uint8Array): void => {
    for (let i = 0; i < block.length / 8; i++) {
      let lane = 0n;
      for (let j = 0; j < 8; j++) lane |= BigInt(block[i * 8 + j]) << BigInt(8 * j);
      a[i] ^= lane;
    }
  };
  let off = 0;
  while (data.length - off >= rate) {
    absorb(data.subarray(off, off + rate));
    keccakF(a);
    off += rate;
  }
  const block = new Uint8Array(rate);
  block.set(data.subarray(off));
  block[data.length - off] ^= 0x01;
  block[rate - 1] ^= 0x80;
  absorb(block);
  keccakF(a);
  const out = new Uint8Array(32);
  for (let i = 0; i < 4; i++) {
    for (let j = 0; j < 8; j++) out[i * 8 + j] = Number((a[i] >> BigInt(8 * j)) & 0xffn);
  }
  return out;
}

// ---- ERC-8004 anchor parsing (mirrors core/anchor.go) ----

const HEX_RE = /^[0-9a-fA-F]+$/;

/**
 * Validate a 20-byte hex address and return its EIP-55 checksum form.
 * All-lowercase/all-uppercase input is accepted and normalized; genuinely
 * mixed-case input must already carry a correct checksum (rejects typos).
 */
export function checksumAddress(addr: string): string {
  if (!addr.startsWith('0x')) throw new Error(`address "${addr}" must be 0x-prefixed`);
  const body = addr.slice(2);
  if (body.length !== 40 || !HEX_RE.test(body)) throw new Error(`address "${addr}" must be 20 hex bytes`);
  const lower = body.toLowerCase();
  const checksummed = eip55(lower);
  const mixed = body !== lower && body !== body.toUpperCase();
  if (mixed && body !== checksummed) throw new Error(`address "${addr}" has an invalid EIP-55 checksum`);
  return '0x' + checksummed;
}

function eip55(lower: string): string {
  const hash = keccak256(new TextEncoder().encode(lower));
  const out = lower.split('');
  for (let i = 0; i < 40; i++) {
    const ch = out[i];
    if (ch < 'a' || ch > 'f') continue; // digits are never uppercased
    const nibble = i % 2 === 0 ? hash[i >> 1] >> 4 : hash[i >> 1] & 0x0f;
    if (nibble >= 8) out[i] = ch.toUpperCase();
  }
  return out.join('');
}

function anchorReqStr(o: Record<string, unknown>, k: string): string {
  const v = o[k];
  if (v === undefined) throw new Error(`anchor erc8004: missing "${k}"`);
  if (typeof v !== 'string') throw new Error(`anchor erc8004: "${k}" must be a string`);
  if (v === '') throw new Error(`anchor erc8004: "${k}" must not be empty`);
  return v;
}

function anchorOptStr(o: Record<string, unknown>, k: string): string {
  const v = o[k];
  if (v === undefined) return '';
  if (typeof v !== 'string') throw new Error(`anchor erc8004: "${k}" must be a string`);
  return v;
}

function anchorUint(o: Record<string, unknown>, k: string): string {
  const v = o[k];
  if (v === undefined) throw new Error(`anchor erc8004: missing "${k}"`);
  if (typeof v === 'string') {
    if (!/^[0-9]+$/.test(v)) throw new Error(`anchor erc8004: "${k}" must be a decimal integer`);
    if (v.length > 1 && v[0] === '0') throw new Error(`anchor erc8004: "${k}" must not have leading zeros`);
    return v;
  }
  if (typeof v === 'number') {
    if (!Number.isInteger(v) || v < 0) throw new Error(`anchor erc8004: "${k}" must be a non-negative integer`);
    return String(v);
  }
  throw new Error(`anchor erc8004: "${k}" must be a decimal string or integer`);
}

/**
 * Parse and validate the ERC-8004 anchor carried by a card, mirroring the Go
 * reference exactly. Returns null when the card has no erc8004 anchor; throws
 * when an anchor is present but malformed. The anchor is read from the card's
 * own signed `anchors` object, so a caller that has verified the card can trust
 * the claim's authenticity without trusting the registry.
 */
export function parseAnchor(card: Card): ERC8004Anchor | null {
  const anchors = card.anchors;
  if (!anchors || typeof anchors !== 'object') return null;
  const raw = (anchors as Record<string, unknown>)['erc8004'];
  if (raw === undefined) return null;
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    throw new Error('anchor erc8004: must be an object');
  }
  const obj = raw as Record<string, unknown>;

  const chain = anchorReqStr(obj, 'chain');
  const m = /^eip155:([0-9]+)$/.exec(chain);
  if (!m) throw new Error(`anchor erc8004: chain "${chain}" must be a CAIP-2 eip155 identifier`);
  if (m[1].length > 1 && m[1][0] === '0') throw new Error(`anchor erc8004: chain "${chain}" has a leading zero`);

  const registry = checksumAddress(anchorReqStr(obj, 'registry'));
  const agentId = anchorUint(obj, 'agent_id');

  const tx = anchorOptStr(obj, 'tx');
  if (tx && !/^0x[0-9a-fA-F]{64}$/.test(tx)) {
    throw new Error(`anchor erc8004: tx "${tx}" must be a 0x-prefixed 32-byte hex hash`);
  }
  const cardUri = anchorOptStr(obj, 'card_uri');

  const caip10 = `${chain}:${registry}`;
  const anchor: ERC8004Anchor = { protocol: 'erc8004', chain, registry, agentId, caip10, ref: `${caip10}/${agentId}` };
  if (tx) anchor.tx = tx;
  if (cardUri) anchor.cardUri = cardUri;
  return anchor;
}

// ---- High-level: verify before invoke ----

export interface VerifyResult {
  verified: boolean;
  cardOk: boolean;
  attestationsOk: boolean;
  moltscore: number;
  inputs: ScoreInputs;
  attestationCount: number;
  /** ERC-8004 on-chain anchor parsed from the verified card, or null if none. */
  anchor: ERC8004Anchor | null;
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
  // Parse the anchor from the *verified* card, never from the registry's
  // convenience `anchor` field — trust must stay in the card's signatures.
  let anchor: ERC8004Anchor | null = null;
  if (cardOk) {
    try {
      anchor = parseAnchor(card);
    } catch {
      anchor = null;
    }
  }
  return {
    verified: cardOk && attestationsOk,
    cardOk,
    attestationsOk,
    moltscore: out.score,
    inputs: out.inputs,
    attestationCount: atts.length,
    anchor,
  };
}
