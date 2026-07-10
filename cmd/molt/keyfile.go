package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/moltnet/moltnet/core"
)

// Keyfile is the on-disk representation of a MoltNet identity. The private key
// never leaves the holder; signing happens locally in the CLI.
type Keyfile struct {
	DID     string `json:"did"`
	Kind    string `json:"kind"` // "owner" or "agent"
	Public  string `json:"public"`
	Private string `json:"private"`
}

func writeKeyfile(path string, kp *core.KeyPair, kind string) error {
	kf := Keyfile{
		DID:     kp.DID,
		Kind:    kind,
		Public:  core.PublicKeyHex(kp.Public),
		Private: core.PrivateKeyHex(kp.Private),
	}
	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func loadKeyfile(path string) (*core.KeyPair, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var kf Keyfile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	kp, err := core.KeyPairFromHex(kf.Private)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return kp, nil
}
