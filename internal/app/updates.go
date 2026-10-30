package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/version"
)

// UpdateCheckTTL é a idade máxima do cache de verificação de atualizações.
const UpdateCheckTTL = 6 * time.Hour

// updatesCache é o conteúdo de ~/.cache/kanivet-cli/updates.json.
type updatesCache struct {
	CheckedAt time.Time         `json:"checked_at"`
	Latest    map[string]string `json:"latest"`
}

// UpdatesDisabled diz se KANIVET_NO_UPDATE_CHECK=1 desliga a verificação.
func (s *Session) UpdatesDisabled() bool { return s.Getenv["KANIVET_NO_UPDATE_CHECK"] == "1" }

func (s *Session) updatesPath() string {
	dir := s.Getenv["XDG_CACHE_HOME"]
	if dir == "" {
		dir = filepath.Join(s.P.Home, ".cache")
	}
	return filepath.Join(dir, "kanivet-cli", "updates.json")
}

func (s *Session) readUpdates() (updatesCache, bool) {
	var c updatesCache
	data, err := os.ReadFile(s.updatesPath())
	if err != nil || json.Unmarshal(data, &c) != nil || c.CheckedAt.IsZero() {
		return updatesCache{}, false
	}
	return c, true
}

// UpdatesCheckedAt devolve quando as atualizações foram verificadas pela última vez.
func (s *Session) UpdatesCheckedAt() (time.Time, bool) {
	c, ok := s.readUpdates()
	return c.CheckedAt, ok
}

// CheckUpdates marca como Outdated os itens nossos com versão nova em st.
// Usa o cache se ele tiver menos de maxAge; senão consulta os backends e regrava o cache.
// Com KANIVET_NO_UPDATE_CHECK=1 não faz nada.
func (s *Session) CheckUpdates(ctx context.Context, st map[string]detect.ItemStatus, maxAge time.Duration) {
	if s.UpdatesDisabled() {
		return
	}
	if c, ok := s.readUpdates(); ok && s.clock().Sub(c.CheckedAt) < maxAge {
		for id, latest := range c.Latest {
			x, ok := st[id]
			if !ok || x.Status != detect.StatusInstalled || latest == "" {
				continue
			}
			if x.Version == "" || version.Compare(x.Version, latest) < 0 {
				x.Status, x.Latest = detect.StatusOutdated, latest
				st[id] = x
			}
		}
		return
	}
	s.detector().CheckUpdates(ctx, s.Cat, st)
	c := updatesCache{CheckedAt: s.clock(), Latest: map[string]string{}}
	for id, x := range st {
		if x.Latest != "" {
			c.Latest[id] = x.Latest
		}
	}
	path := s.updatesPath()
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	if data, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(path, data, 0o644) // o cache é só otimização
	}
}

// clock devolve a hora atual (injetável por Now nos testes).
func (s *Session) clock() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
