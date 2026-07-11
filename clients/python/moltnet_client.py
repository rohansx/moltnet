"""moltnet_client — dependency-light verification for MoltNet Agent Cards,
attestations and MoltScore, using only the Python standard library.

Ed25519 verification is implemented in pure Python (RFC 8032) so this works
without the `cryptography` C/Rust backend. Mirrors the Go reference in `core/`
and `score/` and the TypeScript client.

Scope: verifies authenticity (Ed25519 signatures) and reproduces MoltScore v1.
Per-attestation hash-chain linkage (BLAKE3) is left to the registry / `molt
verify`.
"""
from __future__ import annotations

import hashlib
import json
import math
import urllib.request
from datetime import datetime, timezone
from typing import Any, Optional


# --------------------------------------------------------------------------- #
# Canonicalization (JCS-compatible; mirrors core/canonical.go)
# --------------------------------------------------------------------------- #

def canonicalize(v: Any) -> str:
    if isinstance(v, dict):
        return "{" + ",".join(
            json.dumps(k, ensure_ascii=False) + ":" + canonicalize(v[k])
            for k in sorted(v.keys())
        ) + "}"
    if isinstance(v, list):
        return "[" + ",".join(canonicalize(x) for x in v) + "]"
    if isinstance(v, bool):
        return "true" if v else "false"
    if v is None:
        return "null"
    if isinstance(v, int):
        return str(v)
    if isinstance(v, float):
        return repr(v)  # avoided in signed content
    if isinstance(v, str):
        return json.dumps(v, ensure_ascii=False)
    raise TypeError(f"canonicalize: unsupported type {type(v)!r}")


def canonicalize_without(v: dict, drop_keys: list[str]) -> str:
    clone = {k: val for k, val in v.items() if k not in drop_keys}
    return canonicalize(clone)


# --------------------------------------------------------------------------- #
# base58 + did:key
# --------------------------------------------------------------------------- #

_B58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"


def _b58encode(data: bytes) -> str:
    n = int.from_bytes(data, "big")
    out = ""
    while n > 0:
        n, r = divmod(n, 58)
        out = _B58[r] + out
    for b in data:
        if b == 0:
            out = "1" + out
        else:
            break
    return out


def _b58decode(s: str) -> bytes:
    n = 0
    for ch in s:
        n = n * 58 + _B58.index(ch)
    # count leading '1's -> leading zero bytes
    pad = 0
    for ch in s:
        if ch == "1":
            pad += 1
        else:
            break
    body = n.to_bytes((n.bit_length() + 7) // 8, "big") if n > 0 else b""
    return b"\x00" * pad + body


def did_from_public_key(pub: bytes) -> str:
    return "did:key:z" + _b58encode(b"\xed\x01" + pub)


def public_key_from_did(did: str) -> bytes:
    prefix = "did:key:z"
    if not did.startswith(prefix):
        raise ValueError(f"not a did:key with base58btc encoding: {did}")
    raw = _b58decode(did[len(prefix):])
    if len(raw) != 34 or raw[0] != 0xED or raw[1] != 0x01:
        raise ValueError(f"not an ed25519 did:key: {did}")
    return raw[2:]


# --------------------------------------------------------------------------- #
# Ed25519 verification (pure Python, RFC 8032)
# --------------------------------------------------------------------------- #

_p = 2 ** 255 - 19
_L = 2 ** 252 + 27742317777372353535851937790883648493
_d = (-121665 * pow(121666, _p - 2, _p)) % _p
_I = pow(2, (_p - 1) // 4, _p)


def _inv(x: int) -> int:
    return pow(x, _p - 2, _p)


def _xrecover(y: int) -> int:
    xx = (y * y - 1) * _inv(_d * y * y + 1) % _p
    x = pow(xx, (_p + 3) // 8, _p)
    if (x * x - xx) % _p != 0:
        x = (x * _I) % _p
    if x % 2 != 0:
        x = _p - x
    return x


_By = 4 * _inv(5) % _p
_Bx = _xrecover(_By)
_B = (_Bx % _p, _By % _p)


def _edwards(P: tuple[int, int], Q: tuple[int, int]) -> tuple[int, int]:
    x1, y1 = P
    x2, y2 = Q
    denom = _d * x1 * x2 * y1 * y2
    x3 = (x1 * y2 + x2 * y1) * _inv(1 + denom) % _p
    y3 = (y1 * y2 + x1 * x2) * _inv(1 - denom) % _p
    return (x3 % _p, y3 % _p)


def _scalarmult(P: tuple[int, int], e: int) -> tuple[int, int]:
    result = (0, 1)
    addend = P
    while e > 0:
        if e & 1:
            result = _edwards(result, addend)
        addend = _edwards(addend, addend)
        e >>= 1
    return result


def _is_on_curve(P: tuple[int, int]) -> bool:
    x, y = P
    return (-x * x + y * y - 1 - _d * x * x * y * y) % _p == 0


def _decodepoint(s: bytes) -> tuple[int, int]:
    y = int.from_bytes(s, "little") & ((1 << 255) - 1)
    x = _xrecover(y)
    if (x & 1) != (s[31] >> 7):
        x = _p - x
    P = (x, y)
    if not _is_on_curve(P):
        raise ValueError("point not on curve")
    return P


def ed25519_verify(public_key: bytes, message: bytes, signature: bytes) -> bool:
    if len(signature) != 64 or len(public_key) != 32:
        return False
    try:
        R = _decodepoint(signature[:32])
        A = _decodepoint(public_key)
    except ValueError:
        return False
    S = int.from_bytes(signature[32:], "little")
    if S >= _L:
        return False
    h = int.from_bytes(hashlib.sha512(signature[:32] + public_key + message).digest(), "little") % _L
    left = _scalarmult(_B, S)
    right = _edwards(R, _scalarmult(A, h))
    return left == right


def verify_signature(signer_did: str, message: str, sig_hex: str) -> bool:
    try:
        pub = public_key_from_did(signer_did)
        return ed25519_verify(pub, message.encode("utf-8"), bytes.fromhex(sig_hex))
    except Exception:
        return False


def verify_card(card: dict) -> bool:
    if card.get("spec") != "moltnet/card/v0.1" or not card.get("sig") or not card.get("owner_sig"):
        return False
    payload = canonicalize_without(card, ["sig", "owner_sig"])
    return (
        verify_signature(card["id"], payload, card["sig"])
        and verify_signature(card["owner"], payload, card["owner_sig"])
    )


def verify_attestation(att: dict) -> bool:
    if not att.get("sig"):
        return False
    payload = canonicalize_without(att, ["sig"])
    return verify_signature(att["issuer"], payload, att["sig"])


# --------------------------------------------------------------------------- #
# MoltScore v1 (mirrors score/score.go)
# --------------------------------------------------------------------------- #

_HALF_LIFE_POS = 180.0
_HALF_LIFE_INC = 365.0


def _decay(issued_at: str, now_sec: float, half_life_days: float) -> float:
    try:
        t = datetime.fromisoformat(issued_at.replace("Z", "+00:00")).timestamp()
    except Exception:
        return 1.0
    if t > now_sec:
        return 1.0
    days = (now_sec - t) / 86400.0
    return 0.5 ** (days / half_life_days)


def compute_score(
    atts: list[dict],
    issuer_weights: Optional[dict[str, float]] = None,
    now: Optional[datetime] = None,
) -> dict:
    if now is None:
        now = datetime.now(timezone.utc)
    now_sec = now.timestamp()

    def weight_of(issuer: str) -> float:
        if issuer_weights is None:
            return 1.0
        return issuer_weights.get(issuer, 0.25)

    wc = wd = wi = 0.0
    inputs = {"completions": 0, "disputes": 0, "incidents": 0, "endorsements": 0, "receipts": 0, "distinct_issuers": 0}
    issuers: set[str] = set()

    for a in atts:
        iw = weight_of(a.get("issuer", ""))
        t = a.get("type")
        ts = a.get("issued_at", "")
        if t == "task.completed":
            inputs["completions"] += 1
            wc += iw * _decay(ts, now_sec, _HALF_LIFE_POS)
            issuers.add(a["issuer"])
        elif t == "endorsement":
            inputs["endorsements"] += 1
            wc += 0.25 * iw * _decay(ts, now_sec, _HALF_LIFE_POS)
            issuers.add(a["issuer"])
        elif t == "payment.receipt":
            inputs["receipts"] += 1
            wc += 0.5 * iw * _decay(ts, now_sec, _HALF_LIFE_POS)
            issuers.add(a["issuer"])
        elif t == "task.disputed":
            inputs["disputes"] += 1
            wd += iw * _decay(ts, now_sec, _HALF_LIFE_POS)
        elif t == "incident":
            inputs["incidents"] += 1
            wi += iw * _decay(ts, now_sec, _HALF_LIFE_INC)
        # self.claim and key.rotation contribute nothing

    inputs["distinct_issuers"] = len(issuers)
    x = 1.0 * math.log(1 + wc) + 0.6 * math.log(1 + len(issuers)) - 1.2 * wd - 2.0 * wi - 2.0
    score = round((100.0 / (1.0 + math.exp(-x))) * 10) / 10
    return {"algorithm": "moltscore/v1", "score": score, "inputs": inputs}


# --------------------------------------------------------------------------- #
# Keccak-256 (Ethereum padding) — for EIP-55 address checksums
# A dependency-free port of core/keccak.go, stdlib only. Only used on short
# inputs (addresses), so clarity beats speed.
# --------------------------------------------------------------------------- #

_KECCAK_RC = [
    0x0000000000000001, 0x0000000000008082, 0x800000000000808A, 0x8000000080008000,
    0x000000000000808B, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
    0x000000000000008A, 0x0000000000000088, 0x0000000080008009, 0x000000008000000A,
    0x000000008000808B, 0x800000000000008B, 0x8000000000008089, 0x8000000000008003,
    0x8000000000008002, 0x8000000000000080, 0x000000000000800A, 0x800000008000000A,
    0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
]
_KECCAK_ROTC = [1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 2, 14, 27, 41, 56, 8, 25, 43, 62, 18, 39, 61, 20, 44]
_KECCAK_PILN = [10, 7, 11, 17, 18, 3, 5, 16, 8, 21, 24, 4, 15, 23, 19, 13, 12, 2, 20, 14, 22, 9, 6, 1]
_MASK64 = (1 << 64) - 1


def _rotl64(x: int, n: int) -> int:
    return ((x << n) | (x >> (64 - n))) & _MASK64


def _keccak_f(a: list[int]) -> None:
    for rnd in range(24):
        c = [a[i] ^ a[i + 5] ^ a[i + 10] ^ a[i + 15] ^ a[i + 20] for i in range(5)]
        for i in range(5):
            t = c[(i + 4) % 5] ^ _rotl64(c[(i + 1) % 5], 1)
            for j in range(0, 25, 5):
                a[j + i] ^= t
        t = a[1]
        for i in range(24):
            j = _KECCAK_PILN[i]
            bc0 = a[j]
            a[j] = _rotl64(t, _KECCAK_ROTC[i])
            t = bc0
        for j in range(0, 25, 5):
            bc = [a[j + i] for i in range(5)]
            for i in range(5):
                a[j + i] = bc[i] ^ ((~bc[(i + 1) % 5] & _MASK64) & bc[(i + 2) % 5])
        a[0] ^= _KECCAK_RC[rnd]


def keccak256(data: bytes) -> bytes:
    """Keccak-256 (as used by Ethereum), returned as 32 raw bytes."""
    rate = 136
    a = [0] * 25

    def absorb(block: bytes) -> None:
        for i in range(len(block) // 8):
            a[i] ^= int.from_bytes(block[i * 8:i * 8 + 8], "little")

    off = 0
    n = len(data)
    while n - off >= rate:
        absorb(data[off:off + rate])
        _keccak_f(a)
        off += rate

    block = bytearray(rate)
    block[:n - off] = data[off:]
    block[n - off] ^= 0x01
    block[rate - 1] ^= 0x80
    absorb(bytes(block))
    _keccak_f(a)

    out = bytearray()
    for i in range(4):
        out += (a[i] & _MASK64).to_bytes(8, "little")
    return bytes(out)


# --------------------------------------------------------------------------- #
# ERC-8004 anchor parsing (mirrors core/anchor.go)
# --------------------------------------------------------------------------- #

def _is_hex(s: str) -> bool:
    return len(s) > 0 and all(c in "0123456789abcdefABCDEF" for c in s)


def _is_decimal(s: str) -> bool:
    return len(s) > 0 and all(c in "0123456789" for c in s)


def checksum_address(addr: str) -> str:
    """Validate a 20-byte hex address and return its EIP-55 checksum form.
    All-lowercase/all-uppercase input is normalized; genuinely mixed-case input
    must already carry a correct checksum (rejects typos).
    """
    if not addr.startswith("0x"):
        raise ValueError(f'address "{addr}" must be 0x-prefixed')
    body = addr[2:]
    if len(body) != 40 or not _is_hex(body):
        raise ValueError(f'address "{addr}" must be 20 hex bytes')
    lower = body.lower()
    checksummed = _eip55(lower)
    mixed = body != lower and body != body.upper()
    if mixed and body != checksummed:
        raise ValueError(f'address "{addr}" has an invalid EIP-55 checksum')
    return "0x" + checksummed


def _eip55(lower: str) -> str:
    h = keccak256(lower.encode("ascii"))
    out = list(lower)
    for i in range(40):
        ch = out[i]
        if ch < "a" or ch > "f":  # digits are never uppercased
            continue
        nibble = (h[i // 2] >> 4) if i % 2 == 0 else (h[i // 2] & 0x0F)
        if nibble >= 8:
            out[i] = ch.upper()
    return "".join(out)


def _anchor_req_str(o: dict, k: str) -> str:
    if k not in o:
        raise ValueError(f'anchor erc8004: missing "{k}"')
    v = o[k]
    if not isinstance(v, str):
        raise ValueError(f'anchor erc8004: "{k}" must be a string')
    if v == "":
        raise ValueError(f'anchor erc8004: "{k}" must not be empty')
    return v


def _anchor_opt_str(o: dict, k: str) -> str:
    if k not in o:
        return ""
    v = o[k]
    if not isinstance(v, str):
        raise ValueError(f'anchor erc8004: "{k}" must be a string')
    return v


def _anchor_uint(o: dict, k: str) -> str:
    if k not in o:
        raise ValueError(f'anchor erc8004: missing "{k}"')
    v = o[k]
    if isinstance(v, bool):
        raise ValueError(f'anchor erc8004: "{k}" must be a decimal string or integer')
    if isinstance(v, str):
        if not _is_decimal(v):
            raise ValueError(f'anchor erc8004: "{k}" must be a decimal integer')
        if len(v) > 1 and v[0] == "0":
            raise ValueError(f'anchor erc8004: "{k}" must not have leading zeros')
        return v
    if isinstance(v, int):
        if v < 0:
            raise ValueError(f'anchor erc8004: "{k}" must be a non-negative integer')
        return str(v)
    if isinstance(v, float):
        if v < 0 or v != int(v):
            raise ValueError(f'anchor erc8004: "{k}" must be a non-negative integer')
        return str(int(v))
    raise ValueError(f'anchor erc8004: "{k}" must be a decimal string or integer')


def parse_anchor(card: dict) -> Optional[dict]:
    """Parse and validate the ERC-8004 anchor carried by a card, mirroring the
    Go reference. Returns None when the card has no erc8004 anchor; raises
    ValueError when an anchor is present but malformed. The anchor is read from
    the card's own signed ``anchors`` object, so a caller that has verified the
    card can trust the claim without trusting the registry.
    """
    anchors = card.get("anchors")
    if not isinstance(anchors, dict):
        return None
    raw = anchors.get("erc8004")
    if raw is None:
        return None
    if not isinstance(raw, dict):
        raise ValueError("anchor erc8004: must be an object")

    chain = _anchor_req_str(raw, "chain")
    rest = chain[len("eip155:"):] if chain.startswith("eip155:") else None
    if rest is None or not _is_decimal(rest):
        raise ValueError(f'anchor erc8004: chain "{chain}" must be a CAIP-2 eip155 identifier')
    if len(rest) > 1 and rest[0] == "0":
        raise ValueError(f'anchor erc8004: chain "{chain}" has a leading zero')

    registry = checksum_address(_anchor_req_str(raw, "registry"))
    agent_id = _anchor_uint(raw, "agent_id")

    tx = _anchor_opt_str(raw, "tx")
    if tx and not (tx.startswith("0x") and len(tx) == 66 and _is_hex(tx[2:])):
        raise ValueError(f'anchor erc8004: tx "{tx}" must be a 0x-prefixed 32-byte hex hash')
    card_uri = _anchor_opt_str(raw, "card_uri")

    caip10 = f"{chain}:{registry}"
    out = {
        "protocol": "erc8004",
        "chain": chain,
        "registry": registry,
        "agent_id": agent_id,
        "caip10": caip10,
        "ref": f"{caip10}/{agent_id}",
    }
    if tx:
        out["tx"] = tx
    if card_uri:
        out["card_uri"] = card_uri
    return out


# --------------------------------------------------------------------------- #
# High-level: verify before invoke
# --------------------------------------------------------------------------- #

def _get_json(url: str) -> Any:
    with urllib.request.urlopen(url, timeout=15) as resp:
        return json.load(resp)


def verify_agent(registry_url: str, did: str) -> dict:
    """Fetch an agent's card and chain from a registry, verify every signature,
    and recompute MoltScore locally — trusting the registry only for transport.
    """
    base = registry_url.rstrip("/")
    agent = _get_json(f"{base}/v1/agents/{did}")
    card = agent.get("card")
    att_resp = _get_json(f"{base}/v1/agents/{did}/attestations?limit=500")
    atts = att_resp.get("attestations") or []

    card_ok = verify_card(card) if card else False
    atts_ok = all(verify_attestation(a) for a in atts)
    out = compute_score(atts, None)
    # Parse the anchor from the *verified* card, never from the registry's
    # convenience "anchor" field — trust must stay in the card's signatures.
    anchor = None
    if card_ok:
        try:
            anchor = parse_anchor(card)
        except Exception:
            anchor = None
    return {
        "verified": card_ok and atts_ok,
        "card_ok": card_ok,
        "attestations_ok": atts_ok,
        "moltscore": out["score"],
        "inputs": out["inputs"],
        "attestation_count": len(atts),
        "anchor": anchor,
    }
