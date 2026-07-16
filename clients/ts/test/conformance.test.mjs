import { test } from 'node:test';
import assert from 'node:assert';
import { readFileSync } from 'node:fs';
import { canonicalize, computeScore } from '../dist/index.js';

// spec/conformance/ lives three levels up from clients/ts/test/.
const dir = new URL('../../../spec/conformance/', import.meta.url);
const load = (f) => JSON.parse(readFileSync(new URL(f, dir), 'utf8'));

test('canonicalization matches the shared conformance vectors', () => {
  const vectors = load('canonical_vectors.json');
  assert.ok(vectors.length > 0);
  for (const v of vectors) assert.equal(canonicalize(v.input), v.expected);
});

test('MoltScore matches the shared conformance vectors', () => {
  const vectors = load('score_vectors.json');
  assert.ok(vectors.length > 0);
  for (const v of vectors) {
    const out = computeScore(v.attestations || [], null, null, new Date(v.now));
    assert.ok(Math.abs(out.score - v.expected.score) < 0.05,
      `score ${out.score} vs expected ${v.expected.score}`);
    assert.deepEqual(out.inputs, v.expected.inputs);
  }
});
