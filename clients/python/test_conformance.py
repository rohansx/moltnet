"""Validate the Python client against the shared cross-implementation
conformance vectors in spec/conformance/ (Go and TS validate against the same
files). Run: python3 -m unittest test_conformance -v
"""
import json
import unittest
from datetime import datetime
from pathlib import Path

import moltnet_client as mc

ROOT = Path(__file__).resolve().parents[2] / "spec" / "conformance"


def _load(name):
    with open(ROOT / name) as f:
        return json.load(f)


class TestConformance(unittest.TestCase):
    def test_canonicalization(self):
        vectors = _load("canonical_vectors.json")
        self.assertGreater(len(vectors), 0)
        for v in vectors:
            self.assertEqual(mc.canonicalize(v["input"]), v["expected"])

    def test_score(self):
        vectors = _load("score_vectors.json")
        self.assertGreater(len(vectors), 0)
        for v in vectors:
            now = datetime.fromisoformat(v["now"].replace("Z", "+00:00"))
            out = mc.compute_score(v.get("attestations") or [], None, now)
            self.assertAlmostEqual(out["score"], v["expected"]["score"], delta=0.05)
            self.assertEqual(out["inputs"], v["expected"]["inputs"])


if __name__ == "__main__":
    unittest.main()
