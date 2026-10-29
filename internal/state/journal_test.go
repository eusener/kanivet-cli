package state

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 29, 15, 4, 5, 0, time.UTC)

func TestJournalResume(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir, now, []byte(`{"steps":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if j.RunID != "20261029-150405" {
		t.Fatalf("run id %q", j.RunID)
	}
	_ = j.Set("a", StepDone)
	_ = j.Set("b", StepRunning)
	got, plan, err := LatestUnfinished(dir)
	if err != nil || got == nil || got.RunID != j.RunID || string(plan) != `{"steps":[]}` || got.Steps["b"] != StepRunning {
		t.Fatalf("resume: %+v %s %v", got, plan, err)
	}
}

func TestJournalFinishedNotResumed(t *testing.T) {
	dir := t.TempDir()
	j, _ := NewJournal(dir, now, []byte(`{}`))
	_ = j.Set("a", StepPending)
	if err := j.Finish(); err != nil {
		t.Fatal(err)
	}
	got, _, err := LatestUnfinished(dir)
	if err != nil || got != nil {
		t.Fatalf("want nil, got %+v %v", got, err)
	}
}

func TestJournalRunIDCollision(t *testing.T) {
	dir := t.TempDir()
	a, _ := NewJournal(dir, now, []byte(`{}`))
	b, _ := NewJournal(dir, now, []byte(`{}`))
	if a.RunID == b.RunID || b.RunID != "20261029-150405-2" {
		t.Fatalf("ids %q %q", a.RunID, b.RunID)
	}
}

func TestJournalLogWriter(t *testing.T) {
	dir := t.TempDir()
	j, _ := NewJournal(dir, now, []byte(`{}`))
	w, err := j.LogWriter("fzf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "hello")
	_ = w.Close()
	data, _ := os.ReadFile(filepath.Join(j.Dir, "logs", "fzf.log"))
	if string(data) != "hello" {
		t.Fatalf("log %q", data)
	}
}
