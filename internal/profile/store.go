package profile

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Store is the on-disk shape of ~/.config/fortivpn-client/profiles.toml —
// unprivileged, owned by the invoking user. Credentials never live here;
// only Profile (public config) does.
type Store struct {
	Profiles []Profile `toml:"profiles"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "fortivpn-client", "profiles.toml"), nil
}

func LoadStore() (Store, error) {
	path, err := configPath()
	if err != nil {
		return Store{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Store{}, nil
	}
	if err != nil {
		return Store{}, err
	}
	var s Store
	if err := toml.Unmarshal(data, &s); err != nil {
		return Store{}, err
	}
	return s, nil
}

func SaveStore(s Store) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
