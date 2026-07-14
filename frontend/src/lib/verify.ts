// lib/verify.ts — the "verify in your browser" flow.
//
// This file deliberately contains NO cryptography and NO scoring of its own.
// Canonicalization, Ed25519 verification and MoltScore all come from
// @moltnet/client — the same library Node and browser consumers use, which is
// pinned to the Go reference implementation by spec/conformance/*.json.
//
// That matters: canonicalization decides the exact bytes that were signed. A
// second, subtly different implementation here would produce signatures that
// verify on the server and fail in the browser (or worse, the reverse). One
// implementation, one set of conformance vectors. This module only turns those
// primitives into something a human can read.

import {
  canonicalizeWithout,
  computeScore,
  verifySignature,
  type Attestation as ClientAttestation,
  type Card as ClientCard,
} from '@moltnet/client';
import type { Attestation, Card } from './api';
import { hasEd25519 } from './crypto';

export type StepState = 'ok' | 'bad' | 'pend';
export interface VerifyStep {
  state: StepState;
  text: string;
}

/**
 * The full local audit: both card signatures, every attestation signature, and
 * a locally recomputed MoltScore. Nothing here trusts the registry's numbers —
 * it is handed the raw records and re-derives everything.
 *
 * We verify signatures one at a time (rather than calling the library's
 * all-or-nothing verifyAgent) purely so the UI can show which check failed.
 */
export async function verifyAgent(card: Card, atts: Attestation[]): Promise<VerifyStep[]> {
  const steps: VerifyStep[] = [];

  // Distinguish "this browser cannot check" from "this signature is forged".
  // The library returns false for both; only a feature probe can tell them apart.
  if (!(await hasEd25519())) {
    steps.push({
      state: 'pend',
      text: 'this browser lacks WebCrypto Ed25519 — run `molt verify` for the full check',
    });
    return steps;
  }

  const c = card as unknown as ClientCard;
  const payload = canonicalizeWithout(c as unknown as Record<string, unknown>, ['sig', 'owner_sig']);
  const agentOk = await verifySignature(card.id, payload, card.sig || '');
  const ownerOk = await verifySignature(card.owner, payload, card.owner_sig || '');
  steps.push({ state: agentOk ? 'ok' : 'bad', text: `agent signature ${agentOk ? 'valid' : 'INVALID'}` });
  steps.push({ state: ownerOk ? 'ok' : 'bad', text: `owner signature ${ownerOk ? 'valid' : 'INVALID'}` });

  let ok = 0;
  for (const a of atts) {
    const p = canonicalizeWithout(a as unknown as Record<string, unknown>, ['sig']);
    if (await verifySignature(a.issuer, p, a.sig || '')) ok++;
  }
  if (atts.length) {
    const bad = atts.length - ok;
    steps.push({
      state: bad ? 'bad' : 'ok',
      text: `${ok}/${atts.length} attestation signatures valid${bad ? `, ${bad} INVALID` : ''}`,
    });
  }

  // null weights = every issuer counts 1.0: the correct trustless default when
  // you have only this agent's chain and cannot know how trusted its issuers are.
  const out = computeScore(atts as unknown as ClientAttestation[], null);
  steps.push({
    state: 'ok',
    text: `MoltScore recomputed locally: ${out.score.toFixed(1)} — completions ${out.inputs.completions}, distinct issuers ${out.inputs.distinct_issuers}`,
  });
  return steps;
}

/**
 * Why the number above can differ from the one the registry publishes.
 *
 * A standalone verifier holding one agent's chain cannot know how trustworthy
 * that agent's issuers are, so it weights them all at 1.0. The registry also
 * weights each issuer by that issuer's OWN score (its anti-sybil defence), so
 * its published figure is normally lower. Both are correct; they answer
 * different questions. `molt verify` uses this same uniform basis. Saying so
 * keeps an honest gap from looking like a bug.
 */
export const UNIFORM_WEIGHT_NOTE =
  'Recomputed with uniform issuer weights — the trustless default, and exactly what `molt verify` does. ' +
  'The registry also weights each issuer by their own score (anti-sybil), so its published figure is usually lower. ' +
  'The signatures above are the part that must be exact.';
