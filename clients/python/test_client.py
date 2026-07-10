"""Tests for the MoltNet Python client. Run: python3 -m unittest -v

Set MOLT_CARD to a `molt`-signed card.json to exercise the Go<->Python interop
test (skipped otherwise).
"""
import json
import os
import unittest
from datetime import datetime, timezone

import moltnet_client as mc


class TestCanonical(unittest.TestCase):
    def test_sorts_keys_and_strips_whitespace(self):
        self.assertEqual(
            mc.canonicalize({"b": 1, "a": [3, 2, 1], "c": {"z": True, "a": None}}),
            '{"a":[3,2,1],"b":1,"c":{"a":null,"z":true}}',
        )

    def test_canonicalize_without(self):
        self.assertEqual(
            mc.canonicalize_without({"sig": "x", "id": "did", "n": 1}, ["sig"]),
            '{"id":"did","n":1}',
        )


class TestDidKey(unittest.TestCase):
    def test_roundtrip(self):
        pub = bytes((i * 7) & 0xFF for i in range(32))
        did = mc.did_from_public_key(pub)
        self.assertTrue(did.startswith("did:key:z"))
        self.assertEqual(mc.public_key_from_did(did), pub)


class TestScore(unittest.TestCase):
    def test_self_claims_zero(self):
        now = datetime.now(timezone.utc)
        base = mc.compute_score([], None, now)["score"]
        with_claims = mc.compute_score(
            [{"type": "self.claim", "issuer": "did:key:zA", "issued_at": now.isoformat()}],
            None, now,
        )["score"]
        self.assertEqual(base, with_claims)

    def test_diversity_beats_volume(self):
        now = datetime.now(timezone.utc)
        iso = now.isoformat()
        one = [{"type": "task.completed", "issuer": "did:key:zA", "issued_at": iso} for _ in range(8)]
        many = [{"type": "task.completed", "issuer": f"did:key:z{i}", "issued_at": iso} for i in range(8)]
        self.assertGreater(mc.compute_score(many, None, now)["score"], mc.compute_score(one, None, now)["score"])


class TestEd25519Interop(unittest.TestCase):
    def test_verifies_go_signed_card(self):
        path = os.environ.get("MOLT_CARD")
        if not path:
            self.skipTest("set MOLT_CARD to a molt-signed card.json")
        with open(path) as f:
            card = json.load(f)
        self.assertTrue(mc.verify_card(card), "Go-signed card should verify in Python")
        tampered = dict(card, name=card["name"] + "-tampered")
        self.assertFalse(mc.verify_card(tampered), "tampered card must not verify")


if __name__ == "__main__":
    unittest.main()
