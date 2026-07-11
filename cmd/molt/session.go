package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// sessionFile stores the last SIWK login so CLI commands can reuse the
// session token without re-signing. Format: {registry, owner_did, token}.
type sessionFile struct {
	Registry string `json:"registry"`
	OwnerDID string `json:"owner_did"`
	Token    string `json:"token"`
}

func moltDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".moltnet")
	}
	return ".moltnet"
}

func sessionPath() string { return filepath.Join(moltDir(), "session.json") }

func loadSession() (*sessionFile, error) {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		return nil, err
	}
	var s sessionFile
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Token == "" {
		return nil, fmt.Errorf("session file has no token")
	}
	return &s, nil
}

func saveSession(s *sessionFile) error {
	if err := os.MkdirAll(moltDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), append(data, '\n'), 0o600)
}

func clearSession() { _ = os.Remove(sessionPath()) }
