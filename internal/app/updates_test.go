package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/runner"
)

// updatesFixture devolve uma sessão cujo runner conta chamadas e um status "instalado" de fzf.
func updatesFixture(t *testing.T, cacheAge time.Duration, env map[string]string) (*Session, *atomic.Int32, map[string]detect.ItemStatus) {
	t.Helper()
	f := newFixture(t)
	var calls atomic.Int32
	f.s.R = &runner.FakeRunner{Func: func(ctx context.Context, cmd runner.Cmd) (runner.Result, bool) {
		calls.Add(1)
		return runner.Result{ExitCode: 1}, true
	}}
	f.s.Getenv = env
	f.s.Now = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }
	if cacheAge > 0 {
		path := filepath.Join(f.s.P.Home, ".cache", "kanivet-cli", "updates.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]any{"checked_at": f.s.Now().Add(-cacheAge), "latest": map[string]string{"fzf": "2.0"}})
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := map[string]detect.ItemStatus{"fzf": {ID: "fzf", Status: detect.StatusInstalled, Version: "1.0"}}
	return f.s, &calls, st
}

func TestUpdatesCacheTTL(t *testing.T) {
	const ttl = 6 * time.Hour
	s, calls, st := updatesFixture(t, time.Hour, map[string]string{})
	s.CheckUpdates(context.Background(), st, ttl)
	if calls.Load() != 0 {
		t.Fatalf("cache de 1h não deve chamar o runner (%d chamadas)", calls.Load())
	}
	if st["fzf"].Status != detect.StatusOutdated || st["fzf"].Latest != "2.0" {
		t.Errorf("o cache deve marcar fzf como desatualizado: %+v", st["fzf"])
	}

	s, calls, st = updatesFixture(t, 7*time.Hour, map[string]string{})
	s.CheckUpdates(context.Background(), st, ttl)
	if calls.Load() == 0 {
		t.Fatal("cache de 7h deve chamar o runner")
	}
	if at, ok := s.UpdatesCheckedAt(); !ok || !at.Equal(s.Now()) {
		t.Errorf("a verificação deve regravar o cache com a hora atual: %v %v", at, ok)
	}

	s, calls, st = updatesFixture(t, 7*time.Hour, map[string]string{"KANIVET_NO_UPDATE_CHECK": "1"})
	s.CheckUpdates(context.Background(), st, ttl)
	if calls.Load() != 0 || st["fzf"].Status != detect.StatusInstalled {
		t.Fatalf("com KANIVET_NO_UPDATE_CHECK=1 nada roda (%d chamadas, %v)", calls.Load(), st["fzf"].Status)
	}
}

func TestUpdatesCacheRespectsXDG(t *testing.T) {
	xdg := t.TempDir()
	s, _, st := updatesFixture(t, 0, map[string]string{"XDG_CACHE_HOME": xdg})
	s.CheckUpdates(context.Background(), st, time.Hour)
	if _, err := os.Stat(filepath.Join(xdg, "kanivet-cli", "updates.json")); err != nil {
		t.Fatalf("cache deveria estar no XDG_CACHE_HOME: %v", err)
	}
}
