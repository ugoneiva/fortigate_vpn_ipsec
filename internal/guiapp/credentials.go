package guiapp

import (
	"encoding/json"
	"fmt"

	"github.com/zalando/go-keyring"

	"fortivpn-client/internal/profile"
)

// keyringService groups every profile's credentials under one Secret
// Service collection; each profile's ConnName is the per-entry key. Secrets
// never touch profiles.toml — only this keyring, and the daemon's 0600
// secrets.conf while a profile is applied.
const keyringService = "fortivpn-client"

func SaveCredentials(connName string, creds profile.Credentials) error {
	b, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	return keyring.Set(keyringService, connName, string(b))
}

func LoadCredentials(connName string) (profile.Credentials, error) {
	s, err := keyring.Get(keyringService, connName)
	if err != nil {
		return profile.Credentials{}, err
	}
	var creds profile.Credentials
	if err := json.Unmarshal([]byte(s), &creds); err != nil {
		return profile.Credentials{}, fmt.Errorf("decoding credentials: %w", err)
	}
	return creds, nil
}

func DeleteCredentials(connName string) error {
	err := keyring.Delete(keyringService, connName)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}
