// lib/crypto.ts — Ed25519 key loading for SIWK sign-in.
//
// did:key encoding comes from @moltnet/client (the same code the conformance
// vectors pin), so there is exactly one did:key implementation in the tree.
import { didFromPublicKey } from '@moltnet/client';
export { didFromPublicKey as didFromPub };
//
// WebCrypto can't import a raw Ed25519 *private* key (raw format is public
// only). So we build a JWK from the keyfile's `public` + the seed and import
// that — the spec-compliant, cross-browser path. This handles both keyfile
// formats: register.html's 32-byte seed and the `molt` CLI's 64-byte private.

export interface Keyfile {
  did: string;
  kind?: string;
  public: string; // 32-byte public key hex
  private: string; // 32-byte seed hex (browser) OR 64-byte key hex (CLI)
}

function hexToBytes(hex: string): Uint8Array {
  return new Uint8Array(hex.match(/.{2}/g)!.map((x) => parseInt(x, 16)));
}

function bytesToB64url(b: Uint8Array): string {
  let s = '';
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function b64urlToBytes(s: string): Uint8Array {
  s = s.replace(/-/g, '+').replace(/_/g, '/');
  while (s.length % 4) s += '=';
  const bin = atob(s);
  const a = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) a[i] = bin.charCodeAt(i);
  return a;
}

function toHex(b: Uint8Array): string {
  return Array.from(b)
    .map((x) => x.toString(16).padStart(2, '0'))
    .join('');
}

// Extract the 32-byte seed from a keyfile's private field.
function seedHexFromKeyfile(kf: Keyfile): string {
  const p = kf.private.toLowerCase();
  if (!/^[0-9a-f]+$/.test(p)) throw new Error('private key is not hex');
  if (p.length === 64) return p; // 32-byte seed (browser keyfile)
  if (p.length === 128) return p.slice(0, 64); // 64-byte Go key → first 32 bytes is the seed
  throw new Error('unexpected private key length ' + p.length);
}

export interface LoadedKey {
  did: string;
  sign: (msg: string) => Promise<string>;
}

// Load an owner key from a parsed keyfile, returning a signer + the DID.
export async function loadOwnerKey(text: string): Promise<LoadedKey> {
  let kf: Keyfile;
  try {
    kf = JSON.parse(text);
  } catch {
    throw new Error('not a valid keyfile (bad JSON)');
  }
  if (!kf.public || !kf.private) throw new Error('keyfile missing public/private fields');

  const seedHex = seedHexFromKeyfile(kf);
  const pubHex = kf.public.toLowerCase();
  const jwk = {
    kty: 'OKP' as const,
    crv: 'Ed25519' as const,
    d: bytesToB64url(hexToBytes(seedHex)),
    x: bytesToB64url(hexToBytes(pubHex)),
    ext: true,
  };
  const priv = await crypto.subtle.importKey('jwk', jwk, { name: 'Ed25519' }, true, ['sign']);

  // Sanity-check: the imported key's public must match the file's public.
  const ej = await crypto.subtle.exportKey('jwk', priv);
  if (!ej.x) throw new Error('key export failed');
  if (toHex(b64urlToBytes(ej.x)) !== pubHex) throw new Error('keyfile public key does not match its private key');

  const did = kf.did || didFromPublicKey(hexToBytes(pubHex));
  return {
    did,
    sign: async (msg: string) => {
      const sig = new Uint8Array(
        await crypto.subtle.sign({ name: 'Ed25519' }, priv, new TextEncoder().encode(msg)),
      );
      return toHex(sig);
    },
  };
}

export async function hasEd25519(): Promise<boolean> {
  try {
    await crypto.subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify']);
    return true;
  } catch {
    return false;
  }
}