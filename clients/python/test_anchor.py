"""Tests for ERC-8004 anchor parsing (keccak256 + EIP-55 + canonical ref).
Cross-checks the canonical ref against the Go reference implementation.
"""
import unittest

import moltnet_client as mc


class TestKeccak(unittest.TestCase):
    def test_vectors(self):
        self.assertEqual(
            mc.keccak256(b"").hex(),
            "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
        )
        self.assertEqual(
            mc.keccak256(b"abc").hex(),
            "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45",
        )
        self.assertEqual(
            mc.keccak256(b"The quick brown fox jumps over the lazy dog").hex(),
            "4d741b6f1eb29cb2a9b9911c82f56fa8d73b04959d3d9d222895df6c0b28aa15",
        )


class TestEIP55(unittest.TestCase):
    def test_valid_unchanged(self):
        for a in [
            "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
            "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
            "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
            "0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
        ]:
            self.assertEqual(mc.checksum_address(a), a)

    def test_lowercase_normalized(self):
        self.assertEqual(
            mc.checksum_address("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"),
            "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
        )

    def test_bad_checksum_rejected(self):
        with self.assertRaises(ValueError):
            mc.checksum_address("0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD")

    def test_structurally_invalid(self):
        for a in ["5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", "0x1234"]:
            with self.assertRaises(ValueError):
                mc.checksum_address(a)


def good():
    return {
        "spec": "moltnet/card/v0.1",
        "anchors": {
            "erc8004": {
                "chain": "eip155:8453",
                "registry": "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
                "agent_id": "42",
            }
        },
    }


class TestParseAnchor(unittest.TestCase):
    def test_valid_ref_matches_go(self):
        a = mc.parse_anchor(good())
        self.assertIsNotNone(a)
        self.assertEqual(a["protocol"], "erc8004")
        self.assertEqual(a["registry"], "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed")
        self.assertEqual(a["caip10"], "eip155:8453:0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed")
        self.assertEqual(a["ref"], "eip155:8453:0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed/42")

    def test_integer_agent_id(self):
        card = good()
        card["anchors"]["erc8004"]["agent_id"] = 7
        self.assertEqual(mc.parse_anchor(card)["agent_id"], "7")

    def test_absent(self):
        self.assertIsNone(mc.parse_anchor({"anchors": {"other": {}}}))
        self.assertIsNone(mc.parse_anchor({}))

    def test_malformed_rejected(self):
        def mut(fn):
            c = good()
            fn(c["anchors"]["erc8004"])
            return c

        cases = [
            {"anchors": {"erc8004": "nope"}},
            mut(lambda m: m.__setitem__("chain", "solana:mainnet")),
            mut(lambda m: m.__setitem__("chain", "eip155:base")),
            mut(lambda m: m.__setitem__("chain", "eip155:08453")),
            mut(lambda m: m.pop("registry")),
            mut(lambda m: m.__setitem__("registry", "0x1234")),
            mut(lambda m: m.pop("agent_id")),
            mut(lambda m: m.__setitem__("agent_id", -1)),
            mut(lambda m: m.__setitem__("agent_id", "007")),
            mut(lambda m: m.__setitem__("tx", "0xdeadbeef")),
        ]
        for card in cases:
            with self.assertRaises(ValueError):
                mc.parse_anchor(card)


if __name__ == "__main__":
    unittest.main()
