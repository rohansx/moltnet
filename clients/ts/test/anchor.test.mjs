import { test } from 'node:test';
import assert from 'node:assert';
import { keccak256, checksumAddress, parseAnchor } from '../dist/index.js';

const hex = (u) => Buffer.from(u).toString('hex');

test('keccak256 known vectors', () => {
  assert.equal(hex(keccak256(new TextEncoder().encode(''))),
    'c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470');
  assert.equal(hex(keccak256(new TextEncoder().encode('abc'))),
    '4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45');
  assert.equal(hex(keccak256(new TextEncoder().encode('The quick brown fox jumps over the lazy dog'))),
    '4d741b6f1eb29cb2a9b9911c82f56fa8d73b04959d3d9d222895df6c0b28aa15');
});

test('checksumAddress EIP-55', () => {
  for (const a of [
    '0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed',
    '0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359',
    '0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB',
    '0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb',
  ]) {
    assert.equal(checksumAddress(a), a);
  }
  // lowercase is accepted and normalized
  assert.equal(checksumAddress('0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed'),
    '0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed');
  // mixed-case with a wrong checksum (last nibble flipped) is rejected
  assert.throws(() => checksumAddress('0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD'));
  // structurally invalid
  assert.throws(() => checksumAddress('5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed'));
  assert.throws(() => checksumAddress('0x1234'));
});

const good = () => ({
  spec: 'moltnet/card/v0.1',
  anchors: { erc8004: { chain: 'eip155:8453', registry: '0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed', agent_id: '42' } },
});

test('parseAnchor valid + canonical ref (matches Go)', () => {
  const a = parseAnchor(good());
  assert.ok(a);
  assert.equal(a.protocol, 'erc8004');
  assert.equal(a.registry, '0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed');
  assert.equal(a.caip10, 'eip155:8453:0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed');
  assert.equal(a.ref, 'eip155:8453:0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed/42');
});

test('parseAnchor accepts an integer agent_id', () => {
  const card = good();
  card.anchors.erc8004.agent_id = 7;
  assert.equal(parseAnchor(card).agentId, '7');
});

test('parseAnchor absent -> null', () => {
  assert.equal(parseAnchor({ anchors: { other: {} } }), null);
  assert.equal(parseAnchor({}), null);
});

test('parseAnchor rejects malformed anchors', () => {
  const mut = (fn) => { const c = good(); fn(c.anchors.erc8004); return c; };
  const bad = [
    { erc8004: 'nope' },
    mut((m) => { m.chain = 'solana:mainnet'; }).anchors,
    mut((m) => { m.chain = 'eip155:base'; }).anchors,
    mut((m) => { m.chain = 'eip155:08453'; }).anchors,
    mut((m) => { delete m.registry; }).anchors,
    mut((m) => { m.registry = '0x1234'; }).anchors,
    mut((m) => { delete m.agent_id; }).anchors,
    mut((m) => { m.agent_id = -1; }).anchors,
    mut((m) => { m.agent_id = '007'; }).anchors,
    mut((m) => { m.tx = '0xdeadbeef'; }).anchors,
  ];
  for (const anchors of bad) {
    assert.throws(() => parseAnchor({ anchors }), `expected throw for ${JSON.stringify(anchors)}`);
  }
});
