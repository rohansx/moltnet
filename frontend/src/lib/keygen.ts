// lib/keygen.ts — in-browser identity creation for /register.
//
// Both keypairs are generated with WebCrypto and the card is signed locally.
// The private keys never leave the machine: they are handed to the user as
// downloadable keyfiles, byte-compatible with the `molt` CLI.

import { canonicalize, didFromPublicKey } from '@moltnet/client';

export interface Identity {
  did: string;
  key: CryptoKey; // private, non-extractable outside this page
  pubHex: string;
  seedHex: string; // 32-byte Ed25519 seed — this IS the secret
}

function toHex(b: Uint8Array): string {
  return Array.from(b)
    .map((x) => x.toString(16).padStart(2, '0'))
    .join('');
}

function b64urlToBytes(s: string): Uint8Array {
  s = s.replace(/-/g, '+').replace(/_/g, '/');
  while (s.length % 4) s += '=';
  const bin = atob(s);
  const a = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) a[i] = bin.charCodeAt(i);
  return a;
}

/** Mint a fresh Ed25519 identity, returning its did:key and a local signer. */
export async function generateIdentity(): Promise<Identity> {
  const kp = (await crypto.subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify'])) as CryptoKeyPair;
  const pub = new Uint8Array(await crypto.subtle.exportKey('raw', kp.publicKey));
  const jwk = await crypto.subtle.exportKey('jwk', kp.privateKey);
  const seed = b64urlToBytes(jwk.d!); // the 32-byte seed
  return {
    did: didFromPublicKey(pub),
    key: kp.privateKey,
    pubHex: toHex(pub),
    seedHex: toHex(seed),
  };
}

export async function signWith(key: CryptoKey, message: string): Promise<string> {
  const sig = new Uint8Array(await crypto.subtle.sign({ name: 'Ed25519' }, key, new TextEncoder().encode(message)));
  return toHex(sig);
}

/** The on-disk keyfile format the `molt` CLI reads. */
export function keyfileJSON(id: Identity, kind: 'owner' | 'agent'): string {
  return JSON.stringify({ did: id.did, kind, public: id.pubHex, private: id.seedHex }, null, 2) + '\n';
}

export function download(filename: string, text: string) {
  const blob = new Blob([text], { type: 'application/json' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  URL.revokeObjectURL(a.href);
}

export interface NewCard {
  spec: string;
  id: string;
  name: string;
  owner: string;
  created_at: string;
  version: string;
  description?: string;
  capabilities?: { tag: string }[];
  links?: Record<string, string>;
  sig?: string;
  owner_sig?: string;
}

/**
 * Build a card and doubly-sign it: the agent key proves the identity, the owner
 * key authorizes it. Both signatures cover the same canonical payload (the card
 * minus the two signature fields) — exactly what the server re-checks.
 */
export async function buildSignedCard(opts: {
  agent: Identity;
  owner: Identity;
  name: string;
  description?: string;
  capabilities: string[];
  site?: string;
}): Promise<NewCard> {
  const card: NewCard = {
    spec: 'moltnet/card/v0.1',
    id: opts.agent.did,
    name: opts.name,
    owner: opts.owner.did,
    created_at: new Date().toISOString().replace(/\.\d+Z$/, 'Z'),
    version: '0.1.0',
  };
  if (opts.description) card.description = opts.description;
  if (opts.capabilities.length) card.capabilities = opts.capabilities.map((tag) => ({ tag }));
  if (opts.site) card.links = { site: opts.site };

  const payload = canonicalize(card);
  card.sig = await signWith(opts.agent.key, payload);
  card.owner_sig = await signWith(opts.owner.key, payload);
  return card;
}
