package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissing(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil || len(s.Items) != 0 || s.Owned("x") {
		t.Fatalf("got %+v %v", s, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	s, _ := Load(path)
	s.Profile = "laravel"
	s.Items["fzf"] = InstalledItem{Backend: "brew", Package: "fzf", Version: "0.56.3", At: time.Unix(1, 0).UTC()}
	s.Configs["/h/.config/x"] = AppliedConfig{Item: "fzf", Target: "/h/.config/x", SHA256: "abc"}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Profile != "laravel" || !got.Owned("fzf") || got.Configs["/h/.config/x"].SHA256 != "abc" {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	s, _ := Load(filepath.Join(dir, "state.json"))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp*"))
	if len(matches) != 0 {
		t.Fatalf("leftover temp files: %v", matches)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultDir(t *testing.T) {
	if DefaultDir("/home/dev") != "/home/dev/.local/state/kanivet-cli" {
		t.Fatal(DefaultDir("/home/dev"))
	}
}
