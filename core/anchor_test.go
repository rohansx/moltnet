package core

import "testing"

func TestChecksumAddressEIP55(t *testing.T) {
	// Canonical EIP-55 vectors from the standard.
	valid := []string{
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
	}
	for _, a := range valid {
		got, err := ChecksumAddress(a)
		if err != nil {
			t.Errorf("ChecksumAddress(%s) unexpected error: %v", a, err)
			continue
		}
		if got != a {
			t.Errorf("ChecksumAddress(%s) = %s, want unchanged", a, got)
		}
	}

	// All-lowercase input is accepted and normalized to the checksummed form.
	got, err := ChecksumAddress("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
	if err != nil {
		t.Fatalf("lowercase address rejected: %v", err)
	}
	if got != "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed" {
		t.Errorf("normalized = %s, want 0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", got)
	}

	// A mixed-case address with a wrong checksum (last nibble flipped) is rejected.
	if _, err := ChecksumAddress("0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"); err == nil {
		t.Error("expected bad-checksum address to be rejected")
	}

	bad := []string{
		"5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",   // no 0x
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1Bea",   // too short
		"0xZZAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", // non-hex
	}
	for _, a := range bad {
		if _, err := ChecksumAddress(a); err == nil {
			t.Errorf("expected %s to be rejected", a)
		}
	}
}

func goodAnchor() map[string]any {
	return map[string]any{
		"erc8004": map[string]any{
			"chain":    "eip155:8453",
			"registry": "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
			"agent_id": "42",
		},
	}
}

func TestParseERC8004Valid(t *testing.T) {
	a, present, err := ParseERC8004(goodAnchor())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !present {
		t.Fatal("expected anchor to be present")
	}
	if a.Chain != "eip155:8453" {
		t.Errorf("chain = %s", a.Chain)
	}
	// Registry is normalized to EIP-55 form.
	if a.Registry != "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed" {
		t.Errorf("registry = %s, want checksummed", a.Registry)
	}
	if a.AgentID != "42" {
		t.Errorf("agent_id = %s", a.AgentID)
	}
	if want := "eip155:8453:0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed/42"; a.Ref() != want {
		t.Errorf("Ref() = %s, want %s", a.Ref(), want)
	}
}

func TestParseERC8004IntegerAgentID(t *testing.T) {
	anchors := goodAnchor()
	anchors["erc8004"].(map[string]any)["agent_id"] = float64(7) // JSON numbers decode to float64
	a, _, err := ParseERC8004(anchors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.AgentID != "7" {
		t.Errorf("agent_id = %s, want 7", a.AgentID)
	}
}

func TestParseERC8004Absent(t *testing.T) {
	a, present, err := ParseERC8004(map[string]any{"other": map[string]any{"x": "y"}})
	if err != nil || present || a != nil {
		t.Fatalf("expected clean absence, got a=%v present=%v err=%v", a, present, err)
	}
	if a, present, err := ParseERC8004(nil); err != nil || present || a != nil {
		t.Fatalf("nil anchors should be clean absence, got a=%v present=%v err=%v", a, present, err)
	}
}

func TestParseERC8004Invalid(t *testing.T) {
	mutate := func(fn func(m map[string]any)) map[string]any {
		anchors := goodAnchor()
		fn(anchors["erc8004"].(map[string]any))
		return anchors
	}
	cases := map[string]map[string]any{
		"not an object":      {"erc8004": "nope"},
		"bad chain prefix":   mutate(func(m map[string]any) { m["chain"] = "solana:mainnet" }),
		"non-numeric chain":  mutate(func(m map[string]any) { m["chain"] = "eip155:base" }),
		"leading-zero chain": mutate(func(m map[string]any) { m["chain"] = "eip155:08453" }),
		"missing registry":   mutate(func(m map[string]any) { delete(m, "registry") }),
		"bad registry":       mutate(func(m map[string]any) { m["registry"] = "0x1234" }),
		"missing agent_id":   mutate(func(m map[string]any) { delete(m, "agent_id") }),
		"negative agent_id":  mutate(func(m map[string]any) { m["agent_id"] = float64(-1) }),
		"leading-zero id":    mutate(func(m map[string]any) { m["agent_id"] = "007" }),
		"bad tx":             mutate(func(m map[string]any) { m["tx"] = "0xdeadbeef" }),
	}
	for name, anchors := range cases {
		if _, present, err := ParseERC8004(anchors); err == nil {
			t.Errorf("%s: expected error (present=%v)", name, present)
		} else if !present {
			t.Errorf("%s: expected present=true alongside error", name)
		}
	}
}
