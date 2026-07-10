import { test } from 'node:test';
import assert from 'node:assert';
import { readFileSync } from 'node:fs';
import {
  canonicalize, canonicalizeWithout, didFromPublicKey, publicKeyFromDid,
  verifyCard, verifyAttestation, computeScore,
} from '../dist/index.js';

test('canonicalize sorts keys and strips whitespace', () => {
  assert.equal(
    canonicalize({ b: 1, a: [3, 2, 1], c: { z: true, a: null } }),
    '{"a":[3,2,1],"b":1,"c":{"a":null,"z":true}}'
  );
});

test('canonicalizeWithout drops requested keys', () => {
  assert.equal(canonicalizeWithout({ sig: 'x', id: 'did', n: 1 }, ['sig']), '{"id":"did","n":1}');
});

test('did:key round-trips a public key', () => {
  const pub = new Uint8Array(32).map((_, i) => (i * 7) & 0xff);
  const did = didFromPublicKey(pub);
  assert.ok(did.startsWith('did:key:z'));
  assert.deepEqual(publicKeyFromDid(did), pub);
});

test('computeScore: self-claims count zero', () => {
  const now = new Date();
  const base = computeScore([], null, now).score;
  const withClaims = computeScore(
    [{ type: 'self.claim', issuer: 'did:key:zA', issued_at: now.toISOString() }],
    null, now
  ).score;
  assert.equal(withClaims, base);
});

test('computeScore: diversity beats volume', () => {
  const now = new Date();
  const iso = now.toISOString();
  const one = Array.from({ length: 8 }, () => ({ type: 'task.completed', issuer: 'did:key:zA', issued_at: iso }));
  const many = Array.from({ length: 8 }, (_, i) => ({ type: 'task.completed', issuer: 'did:key:z' + i, issued_at: iso }));
  assert.ok(computeScore(many, null, now).score > computeScore(one, null, now).score);
});

// Interop: verify a doubly-signed card produced by the Go `molt` CLI.
test('verifies a Go-produced signed card (JS<->Go interop)', async () => {
  const path = process.env.MOLT_CARD;
  if (!path) { console.log('  (skipped: set MOLT_CARD to a molt-signed card.json)'); return; }
  const card = JSON.parse(readFileSync(path, 'utf8'));
  assert.equal(await verifyCard(card), true, 'Go-signed card should verify in JS');

  // Tampering must break verification.
  const tampered = { ...card, name: card.name + '-tampered' };
  assert.equal(await verifyCard(tampered), false, 'tampered card must not verify');
});

// In-process roundtrip for an attestation signed with WebCrypto.
test('signs and verifies an attestation roundtrip', async () => {
  const { subtle } = globalThis.crypto;
  const kp = await subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify']);
  const pub = new Uint8Array(await subtle.exportKey('raw', kp.publicKey));
  const did = didFromPublicKey(pub);
  const att = {
    spec: 'moltnet/attestation/v0.1', type: 'task.completed',
    subject: 'did:key:zSubject', issuer: did, issued_at: new Date().toISOString(),
    body: { outcome: 'success' },
  };
  const payload = canonicalizeWithout(att, ['sig']);
  const sig = new Uint8Array(await subtle.sign({ name: 'Ed25519' }, kp.privateKey, new TextEncoder().encode(payload)));
  att.sig = Array.from(sig).map((b) => b.toString(16).padStart(2, '0')).join('');
  assert.equal(await verifyAttestation(att), true);
  assert.equal(await verifyAttestation({ ...att, subject: 'evil' }), false);
});
