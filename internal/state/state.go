// Package state guarda o que o kanivet-cli instalou e configurou, e o journal das execuções.
package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type InstalledItem struct {
	Backend string    `json:"backend"`
	Package string    `json:"package"`
	Version string    `json:"version,omitempty"`
	At      time.Time `json:"at"`
}

type AppliedConfig struct {
	Item   string    `json:"item"`
	Target string    `json:"target"`
	Backup string    `json:"backup,omitempty"` // "" = o arquivo não existia
	SHA256 string    `json:"sha256"`
	Dir    bool      `json:"dir,omitempty"` // diretório movido inteiro para o backup
	At     time.Time `json:"at"`
}

type State struct {
	Version int                      `json:"version"`
	Profile string                   `json:"profile,omitempty"`
	Items   map[string]InstalledItem `json:"items"`
	Configs map[string]AppliedConfig `json:"configs"` // chave = Target absoluto
	Path    string                   `json:"-"`
}

func DefaultDir(home string) string { return filepath.Join(home, ".local", "state", "kanivet-cli") }

// Load lê o estado; um arquivo ausente devolve um estado vazio.
func Load(path string) (*State, error) {
	s := &State{Version: 1, Items: map[string]InstalledItem{}, Configs: map[string]AppliedConfig{}, Path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Items == nil {
		s.Items = map[string]InstalledItem{}
	}
	if s.Configs == nil {
		s.Configs = map[string]AppliedConfig{}
	}
	s.Path = path
	return s, nil
}

func (s *State) Save() error { return writeJSONAtomic(s.Path, s) }

func (s *State) Owned(id string) bool {
	_, ok := s.Items[id]
	return ok
}

// writeJSONAtomic grava num temporário, faz fsync e renomeia por cima do destino.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'), 0o644)
}

func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
